package llm

import (
	"fmt"
	"sync"
	"time"
)

const (
	poolRefillThreshold = 2
	poolBatchSize       = 5
	poolRefillCooldown  = 5 * time.Minute // 全破棄時のリトライ待機時間
	maxDiscardedPerKey  = 20              // キーごとの動的Avoidリスト上限
)

// SpeechPool はカテゴリ×パーソナリティ別にセリフをバッファとして保持する。
type SpeechPool struct {
	mu        sync.Mutex
	pool      map[string][]string
	refilling map[string]bool
	cooldown  map[string]time.Time // 全破棄後のクールダウン終了時刻
	discarded map[string][]string  // キーごとの破棄済みセリフ（動的Avoidリスト）
}

func NewSpeechPool() *SpeechPool {
	return &SpeechPool{
		pool:      make(map[string][]string),
		refilling: make(map[string]bool),
		cooldown:  make(map[string]time.Time),
		discarded: make(map[string][]string),
	}
}

// currentTimeSlot は現在時刻を day / night の2スロットに分類する。
func currentTimeSlot() string {
	if h := time.Now().Hour(); h >= 22 || h < 6 {
		return "night"
	}
	return "day"
}

func poolKey(personality, category, language string) string {
	return fmt.Sprintf("%s:%s:%s:%s", personality, category, language, currentTimeSlot())
}

// Pop はプールからセリフを1つ取り出す。
func (sp *SpeechPool) Pop(key string) (string, bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if len(sp.pool[key]) == 0 {
		return "", false
	}
	speech := sp.pool[key][0]
	sp.pool[key] = sp.pool[key][1:]
	return speech, true
}

// Push はプールにセリフを追加する。
func (sp *SpeechPool) Push(key string, speeches []string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.pool[key] = append(sp.pool[key], speeches...)
}

// Len はプールの残量を返す。
func (sp *SpeechPool) Len(key string) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return len(sp.pool[key])
}

// NeedsRefill は補充が必要かどうかを返す（補充中でなく残量が閾値以下かつクールダウン外）。
func (sp *SpeechPool) NeedsRefill(key string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.refilling[key] {
		return false
	}
	if cd, ok := sp.cooldown[key]; ok && time.Now().Before(cd) {
		return false
	}
	return len(sp.pool[key]) <= poolRefillThreshold
}

// SetCooldown は全破棄時にリトライを一定時間抑制する。
func (sp *SpeechPool) SetCooldown(key string, d time.Duration) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.cooldown[key] = time.Now().Add(d)
}

// IsRefilling は補充中かどうかを返す。
func (sp *SpeechPool) IsRefilling(key string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.refilling[key]
}

// SetRefilling は補充中フラグを設定する。
func (sp *SpeechPool) SetRefilling(key string, v bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.refilling[key] = v
}

// AddDiscarded はバリデーションで弾かれたセリフを動的Avoidリストに追加する。
func (sp *SpeechPool) AddDiscarded(key, speech string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	existing := sp.discarded[key]
	for _, s := range existing {
		if s == speech {
			return // 重複追加しない
		}
	}
	existing = append(existing, speech)
	if len(existing) > maxDiscardedPerKey {
		existing = existing[len(existing)-maxDiscardedPerKey:]
	}
	sp.discarded[key] = existing
}

// GetDiscarded は動的Avoidリストのコピーを返す。
func (sp *SpeechPool) GetDiscarded(key string) []string {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	d := sp.discarded[key]
	if len(d) == 0 {
		return nil
	}
	result := make([]string, len(d))
	copy(result, d)
	return result
}
