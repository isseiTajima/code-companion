package behavior

import (
	"strings"
	"time"

	"sakura-kodama/internal/types"
)

// Inferrer はシグナルの履歴から行動を推論する。
type Inferrer struct {
	history []types.Signal
	window  time.Duration
}

// NewInferrer は Inferrer を初期化する。
func NewInferrer(window time.Duration) *Inferrer {
	return &Inferrer{
		history: make([]types.Signal, 0),
		window:  window,
	}
}

// AddSignal は新しいシグナルを履歴に追加し、古いものを捨てる。
func (i *Inferrer) AddSignal(sig types.Signal) {
	i.history = append(i.history, sig)
	i.cleanup(types.StrToTime(sig.Timestamp))
}

func (i *Inferrer) cleanup(now time.Time) {
	cutoff := now.Add(-i.window)
	start := 0
	for j, sig := range i.history {
		if types.StrToTime(sig.Timestamp).After(cutoff) {
			start = j
			break
		}
	}
	i.history = i.history[start:]
}

// Infer は現在の履歴から行動を推論する。
func (i *Inferrer) Infer() types.Behavior {
	if len(i.history) == 0 {
		return types.Behavior{Type: types.BehaviorBreak, Score: 1.0}
	}

	sourceCounts := map[types.SignalSource]int{}
	messages := ""
	lastWebURL := ""
	for _, sig := range i.history {
		sourceCounts[sig.Source]++
		messages += " " + strings.ToLower(sig.Message)
		if sig.Source == types.SourceWeb {
			lastWebURL = strings.ToLower(sig.Value)
		}
	}

	// 1. AI Pairing 判定
	if sourceCounts[types.SourceAgent] > 0 && sourceCounts[types.SourceFS] > 0 {
		return types.Behavior{Type: types.BehaviorAIPairing, Score: 0.8}
	}

	// 2. Debugging 判定
	if strings.Contains(messages, "fail") || strings.Contains(messages, "panic") || strings.Contains(messages, "test") {
		return types.Behavior{Type: types.BehaviorDebugging, Score: 0.9}
	}

	// 3. Web活動の判定（リサーチ vs 寄り道）
	if sourceCounts[types.SourceWeb] > 0 {
		isDevRelated := false
		devDomains := []string{
			"github.com", "stackoverflow.com", "google.com", "bing.com", "chatgpt.com",
			"claude.ai", "gemini.google.com", "docs.microsoft.com", "developer.apple.com",
			"pkg.go.dev", "npmjs.com", "rust-lang.org", "wails.io", "localhost",
		}
		for _, d := range devDomains {
			if strings.Contains(lastWebURL, d) {
				isDevRelated = true
				break
			}
		}

		if isDevRelated {
			return types.Behavior{Type: types.BehaviorResearching, Score: 0.8}
		} else {
			// 開発に関係なさそうなサイト（SNS, YouTube, ニュース等）
			return types.Behavior{Type: types.BehaviorProcrastinating, Score: 0.7}
		}
	}

	// 4. Researching 判定（システム活動ベース）
	if sourceCounts[types.SourceSystem] > sourceCounts[types.SourceFS] {
		return types.Behavior{Type: types.BehaviorResearching, Score: 0.7}
	}

	// 5. Coding 判定 (Default for FS activity)
	if sourceCounts[types.SourceFS] > 0 || sourceCounts[types.SourceGit] > 0 {
		return types.Behavior{Type: types.BehaviorCoding, Score: 0.9}
	}

	return types.Behavior{Type: types.BehaviorUnknown, Score: 0.5}
}
