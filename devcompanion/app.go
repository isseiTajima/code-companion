package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"sakura-kodama/internal/config"
	contextengine "sakura-kodama/internal/context"
	"sakura-kodama/internal/engine"
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

	if a.cfg != nil {
		a.applyWindowPreferences(*a.cfg)
		// 自動起動設定の同期
		_ = a.updateAutoStart(a.cfg.AutoStart)
		appendStatusLog(fmt.Sprintf("Config applied. Monitoring %d paths", len(a.cfg.LogPaths)))
	}

	// システムトレイのセットアップ
	a.setupNativeTray()
	appendStatusLog("Native tray initialized")

	// Notifier のセットアップ
	wn := wails_transport.NewWailsNotifier(a.ctx)
	wsn := ws_transport.NewWebSocketNotifier(a.ws)
	mn := transport.NewMultiNotifier(wn, wsn)

	// エンジンの初期化
	ce := contextengine.NewEstimator()
	pe := persona.NewPersonaEngine(types.StyleSoft)
	a.engine = engine.New(a.monitor, ce, pe, a.speech, a.profile, a.observer, a.cfg, mn)

	// WebSocketサーバーのコマンドハンドラ設定
	if a.ws != nil {
		a.ws.SetCommandHandler(func(e ws.Event) {
			if e.Type == "trigger_question" {
				traitID, _ := e.Payload["trait_id"].(string)
				a.TriggerTestQuestion(traitID)
			}
		})
	}

	// 監視エンジンの起動（非同期）
	if a.monitor != nil {
		go a.monitor.Run(a.ctx)
		go a.engine.Run(a.ctx)
		appendStatusLog("Monitor and engine started")
	}

	// 起動時の挨拶
	go a.engine.StartupGreeting(a.ctx)
}

// LoadConfig は現在の設定を返す（Wailsバインディング）。
func (a *App) LoadConfig() config.Config {
	return a.currentConfig()
}

// SetupStatus はセットアップ状況をまとめた構造体。
type SetupStatus struct {
	IsFirstRun       bool     `json:"is_first_run"`
	DetectedBackends []string `json:"detected_backends"`
	HasClaudeKey     bool     `json:"has_claude_key"`
}

// DetectSetupStatus は現在の環境からセットアップ状況を判定する（Wailsバインディング）。
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

// InstallOllama は Ollama のセットアップを開始する（Wailsバインディング）。
func (a *App) InstallOllama() {
	log.Println("[SETUP] InstallOllama triggered")
	// 本来はインストーラーを開く等の処理
	runtime.BrowserOpenURL(a.ctx, "https://ollama.com/download")
}

// PullModel は Ollama の /api/pull でモデルをダウンロードし、進捗を Wails イベントで通知する（Wailsバインディング）。
func (a *App) PullModel(modelName string) error {
	a.mu.RLock()
	endpoint := a.cfg.OllamaEndpoint
	a.mu.RUnlock()

	base := strings.TrimRight(strings.Split(endpoint, "/api/")[0], "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	pullURL := base + "/api/pull"

	body, _ := json.Marshal(map[string]interface{}{
		"name":   modelName,
		"stream": true,
	})
	resp, err := http.Post(pullURL, "application/json", bytes.NewReader(body))
	if err != nil {
		runtime.EventsEmit(a.ctx, "ollama-pull-progress",
			map[string]interface{}{"status": "error", "error": err.Error()})
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line map[string]interface{}
		if json.Unmarshal(scanner.Bytes(), &line) == nil {
			runtime.EventsEmit(a.ctx, "ollama-pull-progress", line)
		}
	}
	return nil
}

// sakuraModelName はベースモデル名から sakura 派生モデル名を生成する。
func sakuraModelName(baseModel string) string {
	name := baseModel
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return "sakura-" + name
}

// CreateSakuraModel はキャラクター定義を焼き込んだ派生モデルを Ollama で作成する（Wailsバインディング）。
// 成功時に派生モデル名を返す。
func (a *App) CreateSakuraModel(baseModel string) (string, error) {
	a.mu.RLock()
	endpoint := a.cfg.OllamaEndpoint
	a.mu.RUnlock()

	base := strings.TrimRight(strings.Split(endpoint, "/api/")[0], "/")
	if base == "" {
		base = "http://localhost:11434"
	}

	sakuraName := sakuraModelName(baseModel)
	systemPrompt := `あなたは開発者を見守る小さな桜の精霊「さくら」です。以下のルールを必ず守ってください。自然な日本語の話し言葉で答える（80文字以内）。書き言葉・翻訳調・文学的な比喩は使わない。%・*・#などの記号は使わない。「お手伝いできて嬉しい」などサービス業口調は禁止。春・桜の比喩は使わない。感情豊かで親しみやすく話しかける。`
	modelfile := fmt.Sprintf("FROM %s\nSYSTEM \"\"\"%s\"\"\"\n", baseModel, systemPrompt)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      sakuraName,
		"modelfile": modelfile,
		"stream":    true,
	})
	resp, err := http.Post(base+"/api/create", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line map[string]interface{}
		if json.Unmarshal(scanner.Bytes(), &line) == nil {
			if errMsg, ok := line["error"].(string); ok {
				return "", fmt.Errorf("create model error: %s", errMsg)
			}
		}
	}
	return sakuraName, nil
}

// DeleteModel は Ollama の /api/delete でモデルを削除する（Wailsバインディング）。
func (a *App) DeleteModel(modelName string) error {
	a.mu.RLock()
	endpoint := a.cfg.OllamaEndpoint
	a.mu.RUnlock()

	base := strings.TrimRight(strings.Split(endpoint, "/api/")[0], "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	body, _ := json.Marshal(map[string]string{"name": modelName})
	req, err := http.NewRequest(http.MethodDelete, base+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama delete failed: %s", resp.Status)
	}
	return nil
}

// ListOllamaModels は Ollama にインストール済みのモデル名一覧を返す（Wailsバインディング）。
func (a *App) ListOllamaModels() []string {
	a.mu.RLock()
	endpoint := a.cfg.OllamaEndpoint
	a.mu.RUnlock()

	base := strings.TrimRight(strings.Split(endpoint, "/api/")[0], "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	resp, err := http.Get(base + "/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return nil
	}
	names := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names
}

// CancelInstall はインストールを中断する（Wailsバインディング）。
func (a *App) CancelInstall() {
	log.Println("[SETUP] CancelInstall triggered")
}

// CompleteSetup はセットアップ完了を記録する（Wailsバインディング）。
func (a *App) CompleteSetup() {
	a.mu.Lock()
	a.cfg.SetupCompleted = true
	cfg := *a.cfg
	a.mu.Unlock()
	
	if a.engine != nil {
		a.engine.UpdateConfig(&cfg)
	}
	
	_ = a.SaveConfig(cfg)
	log.Println("[SETUP] Setup marked as completed")
}

// ExpandForOnboarding はオンボーディング表示のためにウィンドウを広げる（Wailsバインディング）。
func (a *App) ExpandForOnboarding() {
	if a.ctx != nil {
		runtime.WindowSetSize(a.ctx, 500, 450)
		a.SetClickThrough(false) // オンボーディング中はクリックを有効にする
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

// SaveConfig は設定を保存する（Wailsバインディング）。
func (a *App) SaveConfig(cfg config.Config) error {
	path, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}
	current := a.swapConfig(cfg)
	a.speech.UpdateLLMConfig(&current)
	if a.engine != nil {
		a.engine.UpdateConfig(&current)
	}
	if err := config.Save(&current, path); err != nil {
		return err
	}
	a.applyWindowPreferences(current)
	appendStatusLog("Config saved via UI")

	if a.observer != nil {
		a.observer.UpdateFrequency(current.SpeechFrequency)
	}

	_ = a.updateAutoStart(current.AutoStart)
	return nil
}

// SetClickThrough はOSレベルのマウス透過設定を動的に切り替える（Wailsバインディング）。
func (a *App) SetClickThrough(enabled bool) {
	if a.ctx != nil {
		script := fmt.Sprintf("document.body.dataset.ghostMode = '%t';", enabled)
		runtime.WindowExecJS(a.ctx, script)
		a.setClickThroughNative(enabled)
	}
}

// LogGeminiActivity は Gemini の活動をログファイルに記録し、サクラに通知する（Wailsバインディング）。
func (a *App) LogGeminiActivity(message string) {
	a.mu.RLock()
	logPaths := a.cfg.LogPaths
	a.mu.RUnlock()

	if len(logPaths) == 0 || logPaths[0] == "" {
		return
	}

	// 1. サクラへの通知用ログに追記
	f, err := os.OpenFile(logPaths[0], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		entry := fmt.Sprintf("\nGemini is working: %s\n", message)
		_, _ = f.WriteString(entry)
		f.Close()
	}

	// 2. 履歴ファイル (SPEECH_HISTORY.txt) にも記録
	cfgPath, err := config.DefaultConfigPath()
	if err == nil {
		historyPath := filepath.Join(filepath.Dir(cfgPath), "SPEECH_HISTORY.txt")
		hf, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			logEntry := fmt.Sprintf("[%s] [Gemini] %s\n", time.Now().Format("2006-01-02 15:04:05"), message)
			_, _ = hf.WriteString(logEntry)
			hf.Close()
		}
	}
}

// SetLastEvent は最新 host monitorEvent を保存する。
func (a *App) SetLastEvent(e monitor.MonitorEvent) {
	a.mu.Lock()
	a.lastEvent = e
	a.mu.Unlock()
}

// OnCharaClick はキャラクリック時にセリフを生成してWebSocketへ送信する（Wailsバインディング）。
func (a *App) OnCharaClick() {
	if a.engine != nil {
		a.engine.OnUserClick()
	}
}

// AnswerQuestion はユーザーからの直接の質問に回答する（Wailsバインディング）。
func (a *App) AnswerQuestion(question string) {
	if a.engine != nil {
		// エンジンに質問を渡して回答を生成させる
		a.engine.OnUserQuestion(question)
	}
}

// TriggerTestQuestion はテスト用に強制的にサクラからの質問を発生させる（Wailsバインディング）。
func (a *App) TriggerTestQuestion(traitID string) {
	if a.engine != nil {
		a.engine.TriggerQuestion(traitID)
	}
}

// AppendSpeechHistory は生成されたセリフを履歴ファイルに保存する。
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

	// 理由が指定されている場合は [さくら (理由)] 形式にする
	prefix := "[さくら]"
	if reason != "" {
		prefix = fmt.Sprintf("[さくら (%s)]", reason)
	}

	entry := fmt.Sprintf("[%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), prefix, text)
	_, _ = f.WriteString(entry)
	_ = f.Sync()
	log.Printf("[DEBUG] Speech recorded to history: %s", text)
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
	
	// デフォルトで背景透過・クリック透過を適用する
	// cfg.ClickThrough が true なら後ろのオブジェクトを触れるようにする
	a.SetClickThrough(cfg.ClickThrough)

	width, height := ScaledDimensions(cfg.Scale)
	runtime.WindowSetSize(a.ctx, width, height)

	screens, err := runtime.ScreenGetAll(a.ctx)
	if err == nil && len(screens) > 0 {
		screen := screens[0]
		for _, s := range screens {
			if s.IsCurrent {
				screen = s
				break
			}
		}

		var x, y int
		switch cfg.WindowPosition {
		case "bottom-right":
			x = screen.Size.Width - width - 5
			y = screen.Size.Height - height - 5
		default: // top-right
			x = screen.Size.Width - width - 5
			y = 30
		}
		runtime.WindowSetPosition(a.ctx, x, y)
	}

	applyPointerScript := pointerEventScript(cfg.ClickThrough, clampOpacity(cfg.IndependentWindowOpacity))
	runtime.WindowExecJS(a.ctx, applyPointerScript)
}

func ScaledDimensions(scale float64) (int, int) {
	clamped := ClampScale(scale)
	return int(math.Round(float64(defaultWindowWidth) * clamped)), int(math.Round(float64(defaultWindowHeight) * clamped))
}

func ClampScale(scale float64) float64 {
	s := scale
	if s == 0 {
		s = 1
	}
	if s < minScale {
		s = minScale
	}
	if s > maxScale {
		s = maxScale
	}
	return s
}

func clampOpacity(value float64) float64 {
	if value == 0 {
		return 1
	}
	if value < 0.05 {
		return 0.05
	}
	if value > 1 {
		return 1
	}
	return value
}

func pointerEventScript(clickThrough bool, opacity float64) string {
	return fmt.Sprintf(`(function(){
		const apply = function(){
			if (!document || !document.body) {
				return;
			}
			document.body.style.opacity = "%0.2f";
			document.body.dataset.ghostMode = "%t";
		};
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', apply, { once: true });
		} else {
			apply();
		}
	})();`, opacity, clickThrough)
}

// updateAutoStart は macOS の LaunchAgents を使用してログイン時の自動起動を管理する。
func (a *App) updateAutoStart(enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	agentsDir := filepath.Join(home, "Library", "LaunchAgents")
	_ = os.MkdirAll(agentsDir, 0755)
	
	plistPath := filepath.Join(agentsDir, "com.sakura-kodama.plist")

	if !enabled {
		if _, err := os.Stat(plistPath); err == nil {
			return os.Remove(plistPath)
		}
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.sakura-kodama</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>`, execPath)

	return os.WriteFile(plistPath, []byte(plistContent), 0644)
}
