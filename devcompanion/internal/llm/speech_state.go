package llm

import (
	"strings"
	"sync"
	"time"
)

// SpeechState は発言状態と履歴を管理する。
type SpeechState struct {
	mu           sync.RWMutex
	recentLines  []string     // 最大10件の発言履歴
	recentEvents []SpeechEvent // 最大10件のイベント履歴
}

type SpeechEvent struct {
	Type string
	Time time.Time
}

// NewSpeechState は SpeechState を初期化する。
func NewSpeechState() *SpeechState {
	return &SpeechState{
		recentLines:  make([]string, 0, 10),
		recentEvents: make([]SpeechEvent, 0, 10),
	}
}

// AddLine は発言を履歴に追加する。
func (ss *SpeechState) AddLine(text string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.recentLines = append(ss.recentLines, text)
	if len(ss.recentLines) > 10 {
		ss.recentLines = ss.recentLines[1:]
	}
}

// normalize は重複判定のためにテキストを正規化する。
// 句読点の除去に加えて、語尾の揺らぎ（〜な気がします/する, 〜ました/た 等）を吸収する。
func normalizeForDedup(s string) string {
	// 句読点・記号除去
	for _, r := range []string{"。", "、", "．", "，", ".", ",", "!", "！", "?", "？", "…", "〜", "～", "ー", " ", "　"} {
		s = strings.ReplaceAll(s, r, "")
	}
	s = strings.TrimSpace(s)

	// 語尾の揺らぎを正規化（長い順に置換して誤置換を防ぐ）
	endings := []struct{ from, to string }{
		{"な気がします", "気"},
		{"な気がする", "気"},
		{"気がします", "気"},
		{"気がする", "気"},
		{"見てました", "見た"},
		{"見てます", "見た"},
		{"見てた", "見た"},
		{"ですよね", ""},
		{"ですよ", ""},
		{"ですね", ""},
		{"だよね", ""},
		{"だよ", ""},
		{"ですか", ""},
		{"ですかね", ""},
		{"なのはわたしだけですかね", ""},
		{"なのはわたしだけ", ""},
		{"じゃないですか", ""},
		{"じゃないですかね", ""},
		{"んですけど", ""},
		{"んですが", ""},
		{"ましたね", ""},
		{"ました", ""},
		{"してます", ""},
		{"してる", ""},
		{"します", ""},
		{"する", ""},
	}
	for _, e := range endings {
		if strings.HasSuffix(s, e.from) {
			s = s[:len(s)-len(e.from)] + e.to
		}
	}

	return s
}

// IsDuplicate は直近の発言と重複・類似しているかを判定する。
func (ss *SpeechState) IsDuplicate(text string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	normTarget := normalizeForDedup(text)
	if normTarget == "" {
		return true // 空文字（または記号のみ）は重複扱い
	}

	// 直近5件を確認（プール利用時はより広い範囲をチェック）
	start := len(ss.recentLines) - 5
	if start < 0 {
		start = 0
	}

	for i := start; i < len(ss.recentLines); i++ {
		if normalizeForDedup(ss.recentLines[i]) == normTarget {
			return true
		}
	}
	return false
}

// GetRecentLines は直近の発言履歴を返す。
func (ss *SpeechState) GetRecentLines(limit int) []string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if limit > len(ss.recentLines) {
		limit = len(ss.recentLines)
	}
	return append([]string{}, ss.recentLines[len(ss.recentLines)-limit:]...)
}

// GetLastEventTime は最後のイベント時刻を返す。
func (ss *SpeechState) GetLastEventTime() time.Time {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if len(ss.recentEvents) == 0 {
		return time.Time{}
	}
	return ss.recentEvents[len(ss.recentEvents)-1].Time
}

// HasDeepFocus は3分以上イベントがないかを判定する。
func (ss *SpeechState) HasDeepFocus(now time.Time) bool {
	lastEventTime := ss.GetLastEventTime()
	if lastEventTime.IsZero() {
		return false
	}
	return now.Sub(lastEventTime) >= 3*time.Minute
}
