package monitor

import (
	"context"
	"log"
	"net/url"
	"strings"
	"time"

	"sakura-kodama/internal/agent"
	"sakura-kodama/internal/behavior"
	"sakura-kodama/internal/config"
	"sakura-kodama/internal/context"
	"sakura-kodama/internal/debug/recorder"
	"sakura-kodama/internal/metrics"
	"sakura-kodama/internal/pipeline"
	"sakura-kodama/internal/plugin"
	"sakura-kodama/internal/sensor"
	"sakura-kodama/internal/session"
	"sakura-kodama/internal/types"
)

// MonitorEvent はパイプラインの最終出力を表す。
type MonitorEvent struct {
	State       types.ContextState    `json:"state"`
	Task        TaskType              `json:"task"`
	Mood        MoodType              `json:"mood"`
	Event       types.HighLevelEvent  `json:"event"`
	Behavior    types.Behavior        `json:"behavior"`
	Session     types.SessionState    `json:"session"`
	Context     types.ContextInfo     `json:"context"`
	Decision    types.ContextDecision `json:"decision"`
	Details     string                `json:"details"`
	IsAISession    bool     `json:"is_ai_session"`   // AIエージェント実行中（バイブコーディング）
	NewsContext    string   `json:"news_context"`    // ニュース見出し（InitCuriosity用）
	NewsTags       []string `json:"news_tags"`       // フィードカテゴリ（フィードバック学習用）
	WeatherContext string   `json:"weather_context"` // 天気情報（InitWeather用）
}

// Monitor はパイプライン（Sensors -> Signals -> Context）を管理する。
type Monitor struct {
	cfg           *config.AppConfig
	agentRegistry *agent.Registry
	contextEngine *contextengine.Estimator
	sensors       []sensor.Sensor
	recorder      *recorder.Recorder
	pluginRegistry *plugin.Registry
	
	behaviorInferrer *behavior.Inferrer
	sessionTracker   *session.Tracker

	aiSessionActive  bool
	devSessionActive bool

	signals chan types.Signal
	events  chan MonitorEvent
}

func New(cfg *config.AppConfig, watchDir string) (*Monitor, error) {
	rec, err := recorder.New(true)
	if err != nil {
		rec, _ = recorder.New(false)
	}

	m := &Monitor{
		cfg:              cfg,
		agentRegistry:    agent.NewRegistry(),
		contextEngine:    contextengine.NewEstimator(),
		behaviorInferrer: behavior.NewInferrer(5 * time.Minute),
		sessionTracker:   session.NewTracker(),
		recorder:         rec,
		pluginRegistry:   plugin.NewRegistry(),
		signals:          make(chan types.Signal, 64),
		events:           make(chan MonitorEvent, 16),
	}

	// センサーの登録
	m.sensors = append(m.sensors, sensor.NewFSSensor(watchDir))
	// AIエージェントをプロセス名・コマンドライン両方から動的検知
	m.sensors = append(m.sensors, sensor.NewAIAgentSensor(3*time.Second))
	m.sensors = append(m.sensors, sensor.NewWebSensor(5*time.Second))

	if cfg != nil {
		m.contextEngine.SetWeights(cfg.SignalWeights)
	}

	return m, nil
}

func (m *Monitor) Run(ctx context.Context) {
	defer m.recorder.Close()

	// 起動時の初期シグナルを注入
	go func() {
		time.Sleep(100 * time.Millisecond)
		m.InjectSignal(types.Signal{
			Type:      types.SigIdleStart,
			Source:    types.SourceSystem,
			Timestamp: types.TimeToStr(time.Now()),
		})
	}()

	for _, s := range m.sensors {
		go func(sn sensor.Sensor) {
			_ = sn.Run(ctx, m.signals)
		}(s)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()


	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 定期的な状態更新（ひとり言用）
			pipeline.SafeExecute("HeartbeatLoop", func() {
				now := time.Now()
				// シグナルがない状態での現在の推定結果を取得
				decision := m.contextEngine.LastDecision
				if decision.State == "" {
					decision.State = types.StateIdle
				}
				
				currentBehavior := m.behaviorInferrer.Infer()
				currentSession := m.sessionTracker.Update(currentBehavior, now)

				m.events <- MonitorEvent{
					State:    decision.State,
					Behavior: currentBehavior,
					Session:  currentSession,
					Context: types.ContextInfo{
						State:      decision.State,
						Confidence: decision.Confidence,
						LastSignal: types.TimeToStr(now),
					},
					Decision: decision,
					Mood:     InferMood(MonitorEvent{State: decision.State, Behavior: currentBehavior, Session: currentSession}),
				}
			})

		case sig := <-m.signals:
			log.Printf("[DEBUG] Monitor received signal: %+v", sig)
			pipeline.SafeExecute("MonitorLoop", func() {
				// parse sig.Timestamp if needed, but for now we just use it as string or current time
				now := time.Now() 

				metrics.IncrementSignalsReceived()
				m.recorder.Record(sig)
				m.pluginRegistry.NotifySignal(sig)
				
				highLevelEvent := m.classifySignal(sig)

				prevState := m.contextEngine.LastDecision.State
				
				// Context Engine で状態推定
				decision := m.contextEngine.ProcessSignal(sig)
				
				if decision.State != prevState {
					metrics.IncrementContextSwitch()
				}

				m.behaviorInferrer.AddSignal(sig)
				currentBehavior := m.behaviorInferrer.Infer()
				currentSession := m.sessionTracker.Update(currentBehavior, now)

				details := sig.Value
				if sig.Type == types.SigWebNavigated {
					// sig.Value = URL, sig.Message = "browsing: {title}"
					title := strings.TrimPrefix(sig.Message, "browsing: ")
					domain := extractDomain(sig.Value)
					if domain != "" && domain != title {
						details = title + "（" + domain + "）"
					} else {
						details = title
					}
				}

				ev := MonitorEvent{
					State:    decision.State,
					Event:    highLevelEvent,
					Behavior: currentBehavior,
					Session:  currentSession,
					Context: types.ContextInfo{
						State:      decision.State,
						Confidence: decision.Confidence,
						LastSignal: types.TimeToStr(now),
					},
					Decision: decision,
					Details:  details,
				}
				ev.Mood = InferMood(ev)
				m.events <- ev
			})
		}
	}
}

func (m *Monitor) Events() <-chan MonitorEvent {
	return m.events
}

func (m *Monitor) InjectSignal(sig types.Signal) {
	select {
	case m.signals <- sig:
	default:
	}
}

// startOnce はフラグが false のときに true にして startEvent を返す。
// 既に true なら activeEvent を返す。セッション開始の重複発火防止に使う。
func startOnce(active *bool, startEvent, activeEvent types.HighLevelEvent) types.HighLevelEvent {
	if !*active {
		*active = true
		return startEvent
	}
	return activeEvent
}

// extractDomain はURLからホスト名（www.なし）を返す。パース失敗時は空文字。
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(u.Host, "www.")
	return host
}

func (m *Monitor) classifySignal(sig types.Signal) types.HighLevelEvent {
	switch sig.Type {
	case types.SigProcessStarted:
		if sig.Source == types.SourceAgent {
			return startOnce(&m.aiSessionActive, types.EventAISessionStarted, types.EventAISessionActive)
		}
	case types.SigFileModified, types.SigGitCommit:
		if ev := startOnce(&m.devSessionActive, types.EventDevSessionStarted, ""); ev != "" {
			return ev
		}
		if sig.Source == types.SourceGit {
			return types.EventGitActivity
		}
		return types.EventDevEditing
	case types.SigWebNavigated:
		return types.EventWebBrowsing
	}
	return ""
}
