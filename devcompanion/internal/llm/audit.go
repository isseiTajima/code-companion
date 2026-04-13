package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RecordType はauditレコードの種別。
type RecordType string

const (
	RecordTypeSpeech        RecordType = "speech"         // 実際に表示されたセリフ
	RecordTypeRejected      RecordType = "rejected"        // direct生成でvalidatorに弾かれた
	RecordTypeBatchRejected RecordType = "batch_rejected"  // バッチ生成でvalidatorに弾かれた
)

// SpeechRecord は1件の発話ログ。
type SpeechRecord struct {
	Timestamp   string     `json:"ts"`
	Type        RecordType `json:"type"`
	Speech      string     `json:"speech,omitempty"`
	Chars       int        `json:"chars,omitempty"`
	Source      string     `json:"source,omitempty"`      // "pool"|"direct"|"fallback"
	Reason      string     `json:"reason,omitempty"`
	Personality string     `json:"personality,omitempty"`
	Language    string     `json:"lang,omitempty"`
	Category    string     `json:"category,omitempty"`
	Original    string     `json:"original,omitempty"`    // rejected時: 弾かれる前のテキスト
	RejectedBy  string     `json:"rejected_by,omitempty"` // rejected時: 弾いたvalidator名
}

// AuditStats はインメモリの集計カウンタ。Stats()で取得できる。
type AuditStats struct {
	TotalShown    int64
	TotalRejected int64
	Fallbacks     int64
	BySource      map[string]int64
	ByRejector    map[string]int64
	CharSum       int64 // 表示セリフの文字数合計（平均算出用）
}

// AuditLogger はセリフの生成・リジェクトをJSONLで記録する。
// パッケージレベルのシングルトンとして使用する。
type AuditLogger struct {
	mu      sync.Mutex
	file    *os.File
	encoder *json.Encoder
	day     string // 現在書き込み中の日付 (YYYYMMDD)

	// アトミックカウンタ
	totalShown    atomic.Int64
	totalRejected atomic.Int64
	fallbacks     atomic.Int64
	charSum       atomic.Int64

	// mapは別muで守る
	countMu    sync.Mutex
	bySource   map[string]int64
	byRejector map[string]int64
}

// globalAudit はパッケージ全体で共有されるAuditLogger。
// NewSpeechGenerator() で初期化される。
var globalAudit = &AuditLogger{
	bySource:   make(map[string]int64),
	byRejector: make(map[string]int64),
}

// ensureFile は日付が変わったらファイルをローテートする。
// mu.Lock() 済みの状態で呼ぶこと。
func (al *AuditLogger) ensureFile() error {
	today := time.Now().Format("20060102")
	if al.file != nil && al.day == today {
		return nil
	}

	if al.file != nil {
		_ = al.file.Close()
		al.file = nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".sakura-kodama", "audit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, fmt.Sprintf("speech_%s.jsonl", today))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	al.file = f
	al.encoder = json.NewEncoder(f)
	al.day = today
	return nil
}

func (al *AuditLogger) write(rec SpeechRecord) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if err := al.ensureFile(); err != nil {
		return
	}
	_ = al.encoder.Encode(rec)
}

// LogSpeech は実際に表示されたセリフを記録する。
func (al *AuditLogger) LogSpeech(speech, reason, personality, category, lang, source string) {
	chars := len([]rune(speech))
	al.write(SpeechRecord{
		Timestamp:   time.Now().Format(time.RFC3339),
		Type:        RecordTypeSpeech,
		Speech:      speech,
		Chars:       chars,
		Source:      source,
		Reason:      reason,
		Personality: personality,
		Category:    category,
		Language:    lang,
	})
	al.totalShown.Add(1)
	al.charSum.Add(int64(chars))
	if source == "fallback" {
		al.fallbacks.Add(1)
	}
	al.countMu.Lock()
	al.bySource[source]++
	al.countMu.Unlock()
}

// LogRejected はdirect生成でvalidatorに弾かれたテキストを記録する。
func (al *AuditLogger) LogRejected(original, reason, rejectedBy string) {
	al.write(SpeechRecord{
		Timestamp:  time.Now().Format(time.RFC3339),
		Type:       RecordTypeRejected,
		Original:   original,
		Reason:     reason,
		RejectedBy: rejectedBy,
	})
	al.totalRejected.Add(1)
	al.countMu.Lock()
	al.byRejector[rejectedBy]++
	al.countMu.Unlock()
}

// LogBatchRejected はバッチ生成でvalidatorに弾かれたテキストを記録する。
func (al *AuditLogger) LogBatchRejected(original, key, rejectedBy string) {
	// key例: "cute:working:ja:day" → category/personalityを分解
	parts := strings.SplitN(key, ":", 4)
	personality, category := "", ""
	if len(parts) >= 2 {
		personality = parts[0]
		category = parts[1]
	}
	lang := ""
	if len(parts) >= 3 {
		lang = parts[2]
	}
	al.write(SpeechRecord{
		Timestamp:   time.Now().Format(time.RFC3339),
		Type:        RecordTypeBatchRejected,
		Original:    original,
		Personality: personality,
		Category:    category,
		Language:    lang,
		RejectedBy:  rejectedBy,
	})
	al.totalRejected.Add(1)
	al.countMu.Lock()
	al.byRejector["batch:"+rejectedBy]++
	al.countMu.Unlock()
}

// Stats は現在のインメモリ集計を返す。
func (al *AuditLogger) Stats() AuditStats {
	al.countMu.Lock()
	src := make(map[string]int64, len(al.bySource))
	for k, v := range al.bySource {
		src[k] = v
	}
	rej := make(map[string]int64, len(al.byRejector))
	for k, v := range al.byRejector {
		rej[k] = v
	}
	al.countMu.Unlock()

	return AuditStats{
		TotalShown:    al.totalShown.Load(),
		TotalRejected: al.totalRejected.Load(),
		Fallbacks:     al.fallbacks.Load(),
		CharSum:       al.charSum.Load(),
		BySource:      src,
		ByRejector:    rej,
	}
}

// SummaryString は現在のセッション統計を1行で返す（デバッグ・ログ用）。
func (al *AuditLogger) SummaryString() string {
	s := al.Stats()
	total := s.TotalShown + s.TotalRejected
	rejRate := 0.0
	if total > 0 {
		rejRate = float64(s.TotalRejected) / float64(total) * 100
	}
	avgChars := 0.0
	if s.TotalShown > 0 {
		avgChars = float64(s.CharSum) / float64(s.TotalShown)
	}
	return fmt.Sprintf(
		"shown=%d rejected=%d(%.0f%%) fallback=%d avgChars=%.1f",
		s.TotalShown, s.TotalRejected, rejRate, s.Fallbacks, avgChars,
	)
}

// sourceFromBackend はbackend文字列からsource種別を返す。
func sourceFromBackend(backend string) string {
	upper := strings.ToUpper(backend)
	switch {
	case strings.Contains(upper, "FALLBACK"):
		return "fallback"
	case strings.Contains(upper, "POOL"):
		return "pool"
	default:
		return "direct"
	}
}
