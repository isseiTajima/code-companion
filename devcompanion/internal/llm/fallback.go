package llm

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sakura-kodama/internal/i18n"
)

var (
	rnd   = rand.New(rand.NewSource(time.Now().UnixNano()))
	rndMu sync.Mutex

	lastUsed   = make(map[string][]string) // Reason毎の履歴管理
	lastUsedMu sync.Mutex
)

// SetSeed は乱数シードを固定する（テスト用）。
func SetSeed(seed int64) {
	rndMu.Lock()
	defer rndMu.Unlock()
	rnd = rand.New(rand.NewSource(seed))
}

// Reason はセリフ生成の理由を表す。
type Reason string

const (
	ReasonSuccess      Reason = "success"
	ReasonFail         Reason = "fail"
	ReasonThinkingTick Reason = "thinking_tick"
	ReasonUserClick    Reason = "user_click"

	// 開発観察コメントの Reason
	ReasonGitCommit  Reason = "git_commit"
	ReasonGitPush    Reason = "git_push"
	ReasonGitAdd     Reason = "git_add"
	ReasonIdle       Reason = "idle"
	ReasonNightWork  Reason = "night_work"
	ReasonActiveEdit Reason = "active_edit"
	ReasonInitSetup  Reason = "init_setup"
	ReasonGreeting   Reason = "greeting"

	// 新しい高レベルイベントの Reason
	ReasonAISessionStarted      Reason = "ai_session_started"
	ReasonAISessionActive       Reason = "ai_session_active"
	ReasonDevSessionStarted     Reason = "dev_session_started"
	ReasonProductiveToolActivity Reason = "productive_tool_activity"
	ReasonDocWriting            Reason = "doc_writing"
	ReasonLongInactivity        Reason = "long_inactivity"
	ReasonUserQuestion          Reason = "user_question"
	ReasonQuestionAnswered      Reason = "question_answered"

	// Proactive Initiatives
	ReasonInitObservation Reason = "initiative_observation"
	ReasonInitSupport     Reason = "initiative_support"
	ReasonInitCuriosity   Reason = "initiative_curiosity"
	ReasonInitMemory      Reason = "initiative_memory"
	ReasonInitWeather     Reason = "initiative_weather"

	// Web browsing
	ReasonWebBrowsing Reason = "web_browsing"
)

// FallbackSpeech はLLM呼び出し失敗時のテンプレートセリフを返す。
func FallbackSpeech(r Reason, lang string) string {
	if lang == "" {
		lang = "ja"
	}

	if r == ReasonGreeting {
		h := time.Now().Hour()
		switch {
		case h >= 5 && h < 10:
			return i18n.T(lang, "speech.greeting.morning")
		case h >= 10 && h < 17:
			return i18n.T(lang, "speech.greeting.noon")
		case h >= 17 && h < 20:
			return i18n.T(lang, "speech.greeting.afternoon")
		case h >= 20 && h < 23:
			return i18n.T(lang, "speech.greeting.evening")
		default:
			return i18n.T(lang, "speech.greeting.night")
		}
	}

	key := fmt.Sprintf("speech.fallback.%s", string(r))
	texts := i18n.TVariant(lang, key)
	if len(texts) == 0 || (len(texts) == 1 && texts[0] == key) {
		return "…"
	}
	
	lastUsedMu.Lock()
	history := lastUsed[key]
	
	// 直近の履歴を除外した候補を作成
	candidates := []string{}
	historyMap := make(map[string]bool)
	for _, s := range history {
		historyMap[s] = true
	}
	for _, t := range texts {
		if !historyMap[t] {
			candidates = append(candidates, t)
		}
	}

	// 全て使用済みの場合は履歴をリセット
	if len(candidates) == 0 {
		candidates = texts
		history = []string{}
	}

	rndMu.Lock()
	idx := rnd.Intn(len(candidates))
	selected := candidates[idx]
	rndMu.Unlock()

	// 履歴を更新（最大5件または候補数の半分まで保持）
	history = append(history, selected)
	maxHist := len(texts) / 2
	if maxHist < 1 { maxHist = 1 }
	if maxHist > 5 { maxHist = 5 }
	
	if len(history) > maxHist {
		history = history[1:]
	}
	lastUsed[key] = history
	lastUsedMu.Unlock()

	return selected
}

// connWarnSpeeches はLLM接続障害時の警告セリフ（性格を問わず共通）。
var connWarnSpeeches = map[string][]string{
	"ja": {
		"あの…AIとの接続がうまくいってないみたいです。設定を確認してもらえますか？",
		"なんか、AIとつながれてないかも…設定、見てもらえますか？",
		"AIとの接続がうまくいってないっぽくて…確認してもらえると助かります。",
	},
	"en": {
		"Hmm, the AI connection doesn't seem to be working… could you check the settings?",
		"I think there might be a connection issue with the AI… worth checking the config.",
	},
}

// connWarnSpeech はLLM接続障害検知時に表示する警告セリフを返す。
func connWarnSpeech(lang string) string {
	if lang == "" {
		lang = "ja"
	}
	speeches, ok := connWarnSpeeches[lang]
	if !ok {
		speeches = connWarnSpeeches["ja"]
	}
	rndMu.Lock()
	s := speeches[rnd.Intn(len(speeches))]
	rndMu.Unlock()
	return s
}

// isTooManyRequestsError は error が HTTP 429 (Too Many Requests) を示すかチェックする。
func isTooManyRequestsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "status 429")
}

// callGeminiCLI は ~/.bin/ai コマンドを呼び出して Gemini からセリフを生成する。
func callGeminiCLI(in OllamaInput) (string, error) {
	prompt, err := renderPrompt(in)
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	aiPath := filepath.Join(homeDir, ".bin", "ai")
	cmd := exec.Command(aiPath, "-p", prompt)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gemini cli: %w", err)
	}

	return strings.TrimSpace(out.String()), nil
}
