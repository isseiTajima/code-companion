package main

import (
	"context"
	"embed"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"sakura-kodama/internal/config"
	contextengine "sakura-kodama/internal/context"
	"sakura-kodama/internal/engine"
	"sakura-kodama/internal/i18n"
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/observer"
	"sakura-kodama/internal/persona"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/transport"
	wails_transport "sakura-kodama/internal/transport/wails"
	ws_transport "sakura-kodama/internal/transport/websocket"
	"sakura-kodama/internal/types"
	"sakura-kodama/internal/ws"
)

// App はWailsバインディングを公開するアプリケーション構造体。
type App struct {
	ctx           context.Context
	speech        *llm.SpeechGenerator
	ws            *ws.Server
	cfg           *config.Config
	assets        embed.FS
	icon          []byte
	mu            sync.RWMutex
	lastEvent     monitor.MonitorEvent
	profile       *profile.ProfileStore
	observer      *observer.DevObserver
	monitor       *monitor.Monitor
	engine        *engine.Engine
	installCancel context.CancelFunc

	ollamaMgr *llm.OllamaManager
	configMgr *config.Manager
}

// NewApp は App を初期化する。
func NewApp(cfg *config.Config, speech *llm.SpeechGenerator, wsServer *ws.Server, ps *profile.ProfileStore, assets embed.FS, icon []byte, obs *observer.DevObserver) *App {
	return &App{
		speech:    speech,
		ws:        wsServer,
		cfg:       cfg,
		assets:    assets,
		icon:      icon,
		lastEvent: monitor.MonitorEvent{State: types.StateIdle, Task: monitor.TaskPlan, Mood: monitor.MoodNeutral},
		profile:   ps,
		observer:  obs,
	}
}

// SetInteractiveMode はフロントエンドから設定/オンボ画面などの全体インタラクティブモードを切り替える（Wailsバインディング）。
func (a *App) SetInteractiveMode(required bool) {
	a.setInteractiveShapeNative(nil, required)
}

// UpdateInteractiveRegions はフロントエンドから操作可能な領域のCSS座標リストを受け取り、
// ObjC側のNSEventモニタに渡す（Wailsバインディング）。
// rects: [x1, y1, w1, h1, x2, y2, w2, h2, ...] CSS pixels, top-left origin
func (a *App) UpdateInteractiveRegions(rects []float64) {
	a.setInteractiveShapeNative(rects, false)
}

func (a *App) SetMonitor(m *monitor.Monitor) {
	a.monitor = m
}

func (a *App) GetContext() context.Context {
	return a.ctx
}

// startup はWailsランタイムからアプリ起動時に呼ばれる。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	appendStatusLog("Application startup initiated")

	// dev モード検出
	if cwd, err := os.Getwd(); err == nil {
		devLocalePath := filepath.Join(cwd, "internal", "i18n", "locales")
		if _, err := os.Stat(devLocalePath); err == nil {
			i18n.SetOverrideDir(devLocalePath)
		}
	}

	// マネージャーの初期化
	a.ollamaMgr = llm.NewOllamaManager(a.cfg.OllamaEndpoint)
	cm, _ := config.NewManager()
	a.configMgr = cm

	if a.cfg != nil {
		a.applyWindowPreferences(*a.cfg)
		if a.configMgr != nil {
			_ = a.configMgr.UpdateAutoStart(a.cfg.AutoStart)
		}
	}

	a.setupNativeTray()

	wn := wails_transport.NewWailsNotifier(a.ctx)
	wsn := ws_transport.NewWebSocketNotifier(a.ws)
	mn := transport.NewMultiNotifier(wn, wsn)

	// エンジンの初期化
	ce := contextengine.NewEstimator()
	pe := persona.NewPersonaEngine(types.StyleSoft)
	a.engine = engine.New(a.monitor, ce, pe, a.speech, a.profile, a.observer, a.cfg, mn)

	// WebSocketサーバーのコマンドハンドラ
	if a.ws != nil {
		a.ws.SetCommandHandler(func(e ws.Event) {
			if e.Type == "trigger_question" {
				traitID, _ := e.Payload["trait_id"].(string)
				a.TriggerTestQuestion(traitID)
			} else if e.Type == "trigger_mood" {
				mood, _ := e.Payload["mood"].(string)
				runtime.EventsEmit(a.ctx, "force_mood", mood)
			}
		})
	}

	// 監視エンジンの起動
	if a.monitor != nil {
		go a.monitor.Run(a.ctx)
		go a.engine.Run(a.ctx)
	}

	go a.engine.StartupGreeting(a.ctx)
}

// mouseMonitorLoop は 150ms ごとにマウス座標を確認し、
// フロントエンドから受け取った実際の CSS 座標(interactiveRects) に基づいて

// LoadConfig は現在の設定を返す（Wailsバインディング）。
func (a *App) LoadConfig() config.Config {
	return a.currentConfig()
}

type SetupStatus struct {
	IsFirstRun       bool     `json:"is_first_run"`
	DetectedBackends []string `json:"detected_backends"`
	HasClaudeKey     bool     `json:"has_claude_key"`
}

func (a *App) DetectSetupStatus() SetupStatus {
	backends := []string{}
	if a.speech.IsAvailable("ollama") {
		backends = append(backends, "ollama")
	}
	if a.speech.IsAvailable("claude") {
		backends = append(backends, "claude")
	}
	if a.speech.IsAvailable("gemini") {
		backends = append(backends, "gemini")
	}
	return SetupStatus{
		IsFirstRun:       !a.cfg.SetupCompleted,
		DetectedBackends: backends,
		HasClaudeKey:     a.speech.IsAvailable("claude"),
	}
}

func (a *App) InstallOllama() {
	runtime.BrowserOpenURL(a.ctx, "https://ollama.com/download")
}

func (a *App) PullModel(modelName string) error {
	return a.ollamaMgr.PullModel(modelName, func(line map[string]interface{}) {
		runtime.EventsEmit(a.ctx, "ollama-pull-progress", line)
	})
}

func (a *App) CreateSakuraModel(baseModel string) (string, error) {
	return a.ollamaMgr.CreateSakuraModel(baseModel)
}

func (a *App) DeleteModel(modelName string) error {
	return a.ollamaMgr.DeleteModel(modelName)
}

func (a *App) ListOllamaModels() []string {
	names, _ := a.ollamaMgr.ListModels()
	return names
}

func (a *App) CancelInstall() {}

func (a *App) CompleteSetup() {
	a.mu.Lock()
	a.cfg.SetupCompleted = true
	cfg := *a.cfg
	a.mu.Unlock()
	if a.engine != nil {
		a.engine.UpdateConfig(&cfg)
	}
	_ = a.SaveConfig(cfg)
}

func (a *App) ExpandForOnboarding() {
	if a.ctx != nil {
		runtime.WindowSetSize(a.ctx, 500, 450)
		a.SetClickThrough(false)
	}
}

func (a *App) ExpandForReview() {
	if a.ctx == nil {
		return
	}
	const w, h = 400, 440
	runtime.WindowSetSize(a.ctx, w, h)
	// 拡張後にウィンドウが画面右端からはみ出さないよう位置を調整する
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err == nil && len(screens) > 0 {
		screen := screens[0]
		for _, s := range screens {
			if s.IsCurrent {
				screen = s
				break
			}
		}
		x := screen.Size.Width - w - 10
		if x < 0 {
			x = 0
		}
		y := 40
		runtime.WindowSetPosition(a.ctx, x, y)
	}
	a.SetClickThrough(false)
}

func (a *App) CollapseFromReview() {
	if a.ctx != nil {
		a.applyWindowPreferences(a.currentConfig())
	}
}

func (a *App) currentConfig() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return config.Config{}
	}
	return *a.cfg
}

func (a *App) swapConfig(next config.Config) config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg == nil {
		a.cfg = &config.Config{}
	}
	*a.cfg = next
	return *a.cfg
}

func (a *App) snapshot() (monitor.MonitorEvent, config.Config) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var cfgCopy config.Config
	if a.cfg != nil {
		cfgCopy = *a.cfg
	}
	return a.lastEvent, cfgCopy
}

func (a *App) SaveConfig(cfg config.Config) error {
	current := a.swapConfig(cfg)
	a.speech.UpdateLLMConfig(&current)
	if a.engine != nil {
		a.engine.UpdateConfig(&current)
	}
	if a.configMgr != nil {
		if err := a.configMgr.Save(&current); err != nil {
			return err
		}
	}
	a.ollamaMgr.UpdateEndpoint(current.OllamaEndpoint)
	a.applyWindowPreferences(current)
	i18n.Reload("ja", "en")
	if a.observer != nil {
		a.observer.UpdateFrequency(current.SpeechFrequency)
	}
	return nil
}

func (a *App) SetClickThrough(enabled bool) {
	if a.ctx != nil {
		script := fmt.Sprintf("document.body.dataset.ghostMode = '%t';", enabled)
		runtime.WindowExecJS(a.ctx, script)
	}
}

func (a *App) LogGeminiActivity(message string) {
	a.mu.RLock()
	logPaths := a.cfg.LogPaths
	a.mu.RUnlock()
	if len(logPaths) == 0 || logPaths[0] == "" {
		return
	}
	f, err := os.OpenFile(logPaths[0], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		entry := fmt.Sprintf("\nGemini is working: %s\n", message)
		_, _ = f.WriteString(entry)
		f.Close()
	}
}

func (a *App) SetLastEvent(e monitor.MonitorEvent) {
	a.mu.Lock()
	a.lastEvent = e
	a.mu.Unlock()
}

func (a *App) OnCharaClick() {
	if a.engine != nil {
		go a.engine.OnUserClick()
	}
}

func (a *App) AnswerQuestion(question string) {
	if a.engine != nil {
		a.engine.OnUserQuestion(question)
	}
}

func (a *App) HandleQuestionAnswer(traitID string, optionIndex int, answerText string) {
	if a.engine != nil {
		a.engine.HandleQuestionAnswer(traitID, optionIndex, answerText)
	}
}

func (a *App) TriggerTestQuestion(traitID string) {
	if a.engine != nil {
		a.engine.TriggerQuestion(traitID)
	}
}

func (a *App) RecordNewsInterest(newsContext string, interested bool, tags []string) {
	if newsContext == "" || a.profile == nil {
		return
	}
	a.profile.RecordNewsInterest(newsContext, tags, interested)
}

// GetUnratedSpeeches は直近2時間分のauditログから未評価セリフ一覧を返す。
func (a *App) GetUnratedSpeeches() []llm.SpeechReviewItem {
	since := time.Now().Add(-2 * time.Hour)
	items, err := llm.LoadUnratedSpeeches(1, since)
	if err != nil {
		return nil
	}
	return items
}

// RateSpeech は1件のセリフに評価（1-10）とコメントを付けてratings.jsonlに記録する。
func (a *App) RateSpeech(speech, personality, category, lang string, rating int, comment string) error {
	return llm.SaveRating(llm.SpeechReviewItem{
		Speech:      speech,
		Personality: personality,
		Category:    category,
		Lang:        lang,
	}, rating, comment)
}

func (a *App) AppendSpeechHistory(reason, text string) {
	if text == "" {
		return
	}
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return
	}
	dir := filepath.Dir(cfgPath)
	historyPath := filepath.Join(dir, "SPEECH_HISTORY.txt")
	f, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	prefix := "[さくら]"
	if reason != "" {
		prefix = fmt.Sprintf("[さくら (%s)]", reason)
	}
	entry := fmt.Sprintf("[%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), prefix, text)
	_, _ = f.WriteString(entry)
}

func appendStatusLog(message string) {
	cfgPath, err := config.DefaultConfigPath()
	if err != nil {
		return
	}
	dir := filepath.Dir(cfgPath)
	_ = os.MkdirAll(dir, 0755)
	statusPath := filepath.Join(dir, "STATUS.md")
	entry := fmt.Sprintf("- %s %s\n", time.Now().Format(time.RFC3339), message)
	f, err := os.OpenFile(statusPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}

const (
	defaultWindowWidth  = 350
	defaultWindowHeight = 400
	minScale            = 0.8
	maxScale            = 2.0
)

func (a *App) applyWindowPreferences(cfg config.Config) {
	if a.ctx == nil {
		return
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, cfg.AlwaysOnTop)
	width, height := ScaledDimensions(cfg.Scale)
	runtime.WindowSetSize(a.ctx, width, height)
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err == nil && len(screens) > 0 {
		screen := screens[0]
		for _, s := range screens {
			if s.IsCurrent { screen = s; break }
		}
		var x, y int
		switch cfg.WindowPosition {
		case "bottom-right":
			x = screen.Size.Width - width - 5
			y = screen.Size.Height - height - 5
		default:
			x = screen.Size.Width - width - 5
			y = 30
		}
		runtime.WindowSetPosition(a.ctx, x, y)
	}
}

func ScaledDimensions(scale float64) (int, int) {
	clamped := ClampScale(scale)
	return int(math.Round(float64(defaultWindowWidth) * clamped)), int(math.Round(float64(defaultWindowHeight) * clamped))
}

func ClampScale(scale float64) float64 {
	s := scale
	if s == 0 { s = 1 }
	if s < minScale { s = minScale }
	if s > maxScale { s = maxScale }
	return s
}

func clampOpacity(value float64) float64 {
	if value == 0 { return 1 }
	if value < 0.05 { return 0.05 }
	if value > 1 { return 1 }
	return value
}

func pointerEventScript(clickThrough bool, opacity float64) string {
	return fmt.Sprintf(`(function(){
		const apply = function(){
			if (!document || !document.body) return;
			document.body.style.opacity = "%0.2f";
			document.body.dataset.ghostMode = "%t";
		};
		if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', apply, { once: true });
		else apply();
	})();`, opacity, clickThrough)
}
