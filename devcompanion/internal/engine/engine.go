package engine

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"sakura-kodama/internal/config"
	contextengine "sakura-kodama/internal/context"
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/observer"
	"sakura-kodama/internal/persona"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

// Notifier はエンジンからのイベントを外部（Wails/WebSocket）に通知するインターフェース。
type Notifier interface {
	Notify(event types.Event)
}

// Engine はモニタリング、状況推定、人格生成、通知を統合するコアエンジン。
//
// 不変条件:
//   - monitor, speech, profile, notifier は nil であってはならない
//   - cfg は起動後に nil になってはならない
//   - history は最大 10 件を保持する
type Engine struct {
	monitor   *monitor.Monitor
	context   *contextengine.Estimator
	persona   *persona.PersonaEngine
	speech    llm.Speaker
	profile   *profile.ProfileStore
	observer  *observer.DevObserver
	notifier  Notifier
	cfg       *config.Config
	logger    SpeechLogger
	learning  *LearningEngine
	situation *SituationEngine
	proactive *ProactiveEngine

	lastEvent monitor.MonitorEvent
	history   []string
}

// New は新しい Engine を作成する。
//
// 事前条件:
//   - m, s, prof, n は nil であってはならない
//   - c は nil であってはならない
func New(m *monitor.Monitor, ctxEng *contextengine.Estimator, p *persona.PersonaEngine, s llm.Speaker, prof *profile.ProfileStore, obs *observer.DevObserver, c *config.Config, n Notifier) *Engine {
	if m == nil {
		panic("engine: New: monitor must not be nil")
	}
	if s == nil {
		panic("engine: New: speech must not be nil")
	}
	if prof == nil {
		panic("engine: New: profile must not be nil")
	}
	if n == nil {
		panic("engine: New: notifier must not be nil")
	}
	if c == nil {
		panic("engine: New: config must not be nil")
	}

	logDir := resolveLogDir(c)
	e := &Engine{
		monitor:  m,
		context:  ctxEng,
		persona:  p,
		speech:   s,
		profile:  prof,
		observer: obs,
		notifier: n,
		cfg:      c,
		logger:   NewFileSpeechLogger(logDir),
		history:  make([]string, 0, 10),
	}
	e.situation = NewSituationEngine()
	e.learning = NewLearningEngine(prof, e, c)
	e.proactive = NewProactiveEngine(prof, e, c)
	return e
}

// resolveLogDir は設定ファイルパスからログディレクトリを導出する。
// エラー時は空文字列を返す（ログをスキップ）。
func resolveLogDir(c *config.Config) string {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		fmt.Printf("[WARN] could not resolve config path for logging: %v\n", err)
		return ""
	}
	return filepath.Dir(cfgPath)
}

// Run はメインのパイプライン処理を開始する。
func (e *Engine) Run(ctx context.Context) {
	events := e.monitor.Events()
	var observations <-chan observer.DevObservation
	if e.observer != nil {
		observations = e.observer.Observations()
	}

	// 積極的介入エンジンの定期実行
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.proactive.Tick(e.lastEvent)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("[ENGINE] Pipeline stopped")
			return

		case ev := <-events:
			reason := e.reasonFromEvent(ev)
			eventObj := types.Event{
				Type: "monitor_event",
				Payload: map[string]interface{}{
					"state": string(ev.State),
					"task":  string(ev.Task),
				},
			}

			// Monitor固有のプロファイル更新
			e.profile.RecordActivity(time.Now())
			if ev.State == types.StateSuccess {
				e.profile.RecordBuildSuccess()
				e.profile.RecordMoment(types.ProjectMoment{
					Type:      "success",
					Message:   "ビルド成功！",
					Timestamp: types.TimeToStr(time.Now()),
				})
			} else if ev.State == types.StateFail {
				e.profile.RecordBuildFail()
			}
			if e.observer != nil {
				e.observer.OnMonitorEvent(ev, time.Now())
			}
			e.lastEvent = ev

			e.handleAndNotify("monitor_event", eventObj, ev, reason, e.context.LastDecision.Confidence)

		case obs := <-observations:
			reason := e.reasonFromObservation(obs)
			eventObj := types.Event{
				Type: "observation_event",
				Payload: map[string]interface{}{
					"type": string(obs.Type),
				},
			}

			// Observation固有のプロファイル更新
			if obs.Type == observer.ObsGitCommit {
				e.profile.RecordCommit()
				e.profile.RecordMoment(types.ProjectMoment{
					Type:      "milestone",
					Message:   "コミット完了",
					Timestamp: types.TimeToStr(time.Now()),
				})
			}

			e.handleAndNotify("observation_event", eventObj, e.lastEvent, reason, e.context.LastDecision.Confidence)
		}
	}
}

// handleAndNotify はセリフ生成・状況更新・通知の共通パイプラインを実行する。
// monitor_event と observation_event の両方で使用され、重複を排除する。
func (e *Engine) handleAndNotify(eventType string, eventObj types.Event, ev monitor.MonitorEvent, reason llm.Reason, confidence float64) {
	world, emotion := e.situation.ProcessEvent(eventObj)
	e.learning.ProcessEvent(eventObj)

	prof := e.profile.Get()
	text, prompt, backend := e.speech.Generate(ev, e.cfg, reason, prof, "")
	if text == "" {
		return
	}

	e.addHistory(text)
	e.logger.LogSpeech(string(reason), text, prompt, backend, confidence, emotion, world.IsDeepWork, ev, e.history)

	e.notifier.Notify(types.Event{
		Type: eventType,
		Payload: map[string]interface{}{
			"state":          string(ev.State),
			"task":           string(ev.Task),
			"mood":           string(ev.Mood),
			"emotion":        string(emotion),
			"is_deep_work":   world.IsDeepWork,
			"speech":         text,
			"timestamp":      time.Now(),
			"using_fallback": e.speech.IsUsingFallback(),
			"profile":        map[string]string{"name": e.cfg.Name, "tone": e.cfg.Tone},
		},
	})
}

func (e *Engine) addHistory(msg string) {
	e.history = append(e.history, msg)
	if len(e.history) > 10 {
		e.history = e.history[1:]
	}
}

// StartupGreeting は起動時の挨拶を行う。
func (e *Engine) StartupGreeting(ctx context.Context) {
	time.Sleep(5 * time.Second)
	// Ollamaの準備を待つ（ベストエフォート）
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://localhost:11434/api/tags")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(2 * time.Second)
		if ctx.Err() != nil {
			return
		}
	}

	reason := llm.ReasonInitSetup
	if e.cfg.SetupCompleted {
		reason = llm.ReasonGreeting
	}
	e.DispatchSpeech("greeting_event", e.lastEvent, reason, "")
}

// OnUserClick はユーザークリック時のセリフを生成する。
func (e *Engine) OnUserClick() {
	e.DispatchSpeech("click_event", e.lastEvent, llm.ReasonUserClick, "")
}

// OnUserQuestion はユーザーからの直接の質問に回答する。
// 事前条件: question は空文字列であってはならない。
func (e *Engine) OnUserQuestion(question string) {
	if question == "" {
		return
	}
	e.DispatchSpeech("question_reply_event", e.lastEvent, llm.ReasonUserQuestion, question)
}

// UpdateConfig はエンジンの設定を更新する。
// 事前条件: cfg は nil であってはならない。
func (e *Engine) UpdateConfig(cfg *config.Config) {
	if cfg == nil {
		panic("engine: UpdateConfig: cfg must not be nil")
	}
	e.cfg = cfg
	e.speech.UpdateLLMConfig(cfg)
	e.learning.UpdateConfig(cfg)
	e.proactive.UpdateConfig(cfg)
}

// HandleQuestionAnswer はユーザーの回答を処理する。
// 事前条件: traitID は空文字列であってはならない。
func (e *Engine) HandleQuestionAnswer(traitID string, optionIndex int, text string) {
	if traitID == "" {
		panic("engine: HandleQuestionAnswer: traitID must not be empty")
	}
	if e.learning != nil {
		e.learning.HandleAnswer(types.TraitID(traitID), optionIndex, text)
	}
}

// TriggerQuestion は指定した特性に関する質問を強制的に発生させる。
func (e *Engine) TriggerQuestion(traitID string) {
	if e.learning != nil {
		e.learning.TriggerQuestion(traitID)
	}
}

// --- SpeechDispatcher 実装 ---

// DispatchSpeech はセリフを生成し、指定タイプのイベントとして通知する（SpeechDispatcher実装）。
// 事前条件: reason は空文字列であってはならない。
func (e *Engine) DispatchSpeech(eventType string, ev monitor.MonitorEvent, reason llm.Reason, question string) {
	if reason == "" {
		panic("engine: DispatchSpeech: reason must not be empty")
	}

	prof := e.profile.Get()
	text, prompt, backend := e.speech.Generate(ev, e.cfg, reason, prof, question)
	if text == "" {
		return
	}

	world, emotion := e.situation.GetState()
	e.addHistory(text)
	e.logger.LogSpeech(string(reason), text, prompt, backend, 1.0, emotion, world.IsDeepWork, ev, e.history)

	e.notifier.Notify(types.Event{
		Type: eventType,
		Payload: map[string]interface{}{
			"state":          string(ev.State),
			"task":           string(ev.Task),
			"mood":           string(ev.Mood),
			"emotion":        string(emotion),
			"is_deep_work":   world.IsDeepWork,
			"speech":         text,
			"timestamp":      time.Now(),
			"using_fallback": e.speech.IsUsingFallback(),
			"profile":        map[string]string{"name": e.cfg.Name, "tone": e.cfg.Tone},
		},
	})
}

// DispatchEvent はセリフ生成なしにイベントを直接通知する（SpeechDispatcher実装）。
// 事前条件: event.Type は空文字列であってはならない。
func (e *Engine) DispatchEvent(event types.Event) {
	if event.Type == "" {
		panic("engine: DispatchEvent: event.Type must not be empty")
	}
	e.notifier.Notify(event)
}

// GenerateQuestion は性格学習用の質問を生成する（SpeechDispatcher実装）。
func (e *Engine) GenerateQuestion(userName string, trait types.TraitID, progress types.TraitProgress, behavior, lang string) (types.Question, error) {
	return e.speech.GenerateQuestion(userName, trait, progress, behavior, lang)
}

// LastEvent は最後に処理したモニタリングイベントを返す（SpeechDispatcher実装）。
func (e *Engine) LastEvent() monitor.MonitorEvent {
	return e.lastEvent
}

// WorldState は現在の世界モデルと感情状態を返す（SpeechDispatcher実装）。
func (e *Engine) WorldState() (types.WorldModel, types.EmotionState) {
	return e.situation.GetState()
}

// --- Engine が SpeechDispatcher を実装していることをコンパイル時に検証 ---
var _ SpeechDispatcher = (*Engine)(nil)

// --- イベント → Reason マッピング ---

func (e *Engine) reasonFromEvent(ev monitor.MonitorEvent) llm.Reason {
	if ev.Behavior.Type == types.BehaviorResearching {
		return llm.ReasonActiveEdit
	}

	switch ev.Event {
	case types.EventAISessionStarted:
		return llm.ReasonAISessionStarted
	case types.EventAISessionActive:
		return llm.ReasonAISessionActive
	case types.EventDevSessionStarted:
		return llm.ReasonDevSessionStarted
	case types.EventDevEditing:
		return llm.ReasonActiveEdit
	case types.EventGitActivity:
		return llm.ReasonGitCommit
	case types.EventProductiveToolActivity:
		return llm.ReasonProductiveToolActivity
	case types.EventDocWriting:
		return llm.ReasonDocWriting
	case types.EventLongInactivity:
		return llm.ReasonLongInactivity
	case types.EventWebBrowsing:
		return llm.ReasonWebBrowsing
	}

	switch ev.State {
	case types.StateSuccess:
		return llm.ReasonSuccess
	case types.StateFail:
		return llm.ReasonFail
	case types.StateCoding:
		return llm.ReasonActiveEdit
	case types.StateIdle:
		return llm.ReasonIdle
	}
	return llm.ReasonThinkingTick
}

func (e *Engine) reasonFromObservation(obs observer.DevObservation) llm.Reason {
	switch obs.Type {
	case observer.ObsGitCommit:
		return llm.ReasonGitCommit
	case observer.ObsGitPush:
		return llm.ReasonGitPush
	case observer.ObsGitAdd:
		return llm.ReasonGitAdd
	case observer.ObsIdleStart:
		return llm.ReasonIdle
	case observer.ObsNightWork:
		return llm.ReasonNightWork
	case observer.ObsActiveEditing:
		return llm.ReasonActiveEdit
	}
	return llm.ReasonThinkingTick
}
