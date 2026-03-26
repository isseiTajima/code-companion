package monitor

import (
	"context"
	"log"
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
	IsAISession bool                  `json:"is_ai_session"` // AIエージェント実行中（バイブコーディング）
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
					// sig.Message is "browsing: {title}" — extract the page title
					details = strings.TrimPrefix(sig.Message, "browsing: ")
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

func (m *Monitor) classifySignal(sig types.Signal) types.HighLevelEvent {
	switch sig.Type {
	case types.SigProcessStarted:
		if sig.Source == types.SourceAgent {
			if !m.aiSessionActive {
				m.aiSessionActive = true
				return types.EventAISessionStarted
			}
			return types.EventAISessionActive
		}
	case types.SigFileModified, types.SigGitCommit:
		if !m.devSessionActive {
			m.devSessionActive = true
			return types.EventDevSessionStarted
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
