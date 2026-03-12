package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/types"
)

// SpeechLogger はセリフをログに記録するインターフェース。
// テスト時は NoopSpeechLogger などモック実装に差し替え可能。
//
// 事前条件（各実装が保証すること）:
//   - text が空の場合は何もしない（呼び出し元で事前チェック不要）
type SpeechLogger interface {
	// LogSpeech はセリフとそのコンテキストをログに記録する。
	LogSpeech(reason, text, prompt, backend string, confidence float64, emotion types.EmotionState, isDeepWork bool, ev monitor.MonitorEvent, history []string)
}

// FileSpeechLogger はファイルシステムにセリフを記録する実装。
// SPEECH_HISTORY.txt（人間可読テキスト）と DEVELOPER_LOG.jsonl（JSON詳細）の
// 2つのファイルに書き込む。
type FileSpeechLogger struct {
	dir string // ログファイルを置くディレクトリ（空の場合は記録をスキップ）
}

// NewFileSpeechLogger は FileSpeechLogger を作成する。
// dir が空の場合、LogSpeech 呼び出しは何もしない（エラーなし）。
func NewFileSpeechLogger(dir string) *FileSpeechLogger {
	return &FileSpeechLogger{dir: dir}
}

// LogSpeech はセリフを SPEECH_HISTORY.txt と DEVELOPER_LOG.jsonl に追記する。
// text が空の場合、または dir が空の場合は何もしない。
func (l *FileSpeechLogger) LogSpeech(reason, text, prompt, backend string, confidence float64, emotion types.EmotionState, isDeepWork bool, ev monitor.MonitorEvent, history []string) {
	if text == "" || l.dir == "" {
		return
	}

	if err := os.MkdirAll(l.dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] failed to create log directory %s: %v\n", l.dir, err)
		return
	}

	l.writeTextLog(reason, text, backend, emotion)
	l.writeJSONLog(reason, text, prompt, backend, confidence, emotion, isDeepWork, ev, history)
}

func (l *FileSpeechLogger) writeTextLog(reason, text, backend string, emotion types.EmotionState) {
	path := filepath.Join(l.dir, "SPEECH_HISTORY.txt")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] failed to open history log %s: %v\n", path, err)
		return
	}
	defer f.Close()

	prefix := "[さくら]"
	if reason != "" {
		if backend != "" {
			prefix = fmt.Sprintf("[さくら (%s:%s:%s)]", reason, backend, emotion)
		} else {
			prefix = fmt.Sprintf("[さくら (%s)]", reason)
		}
	}
	entry := fmt.Sprintf("[%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), prefix, text)
	_, _ = f.WriteString(entry)
}

func (l *FileSpeechLogger) writeJSONLog(reason, text, prompt, backend string, confidence float64, emotion types.EmotionState, isDeepWork bool, ev monitor.MonitorEvent, history []string) {
	path := filepath.Join(l.dir, "DEVELOPER_LOG.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] failed to open developer log %s: %v\n", path, err)
		return
	}
	defer f.Close()

	logEntry := map[string]interface{}{
		"timestamp":    time.Now().Format(time.RFC3339),
		"reason":       reason,
		"speech":       text,
		"prompt":       prompt,
		"backend":      backend,
		"confidence":   confidence,
		"emotion":      emotion,
		"is_deep_work": isDeepWork,
		"history":      history,
		"context": map[string]interface{}{
			"state": ev.State,
			"task":  ev.Task,
			"mood":  ev.Mood,
		},
	}
	data, _ := json.Marshal(logEntry)
	_, _ = f.WriteString(string(data) + "\n")
}

// NoopSpeechLogger はログ記録をすべて無視する（テスト用）。
type NoopSpeechLogger struct{}

func (n *NoopSpeechLogger) LogSpeech(_, _, _, _ string, _ float64, _ types.EmotionState, _ bool, _ monitor.MonitorEvent, _ []string) {
}

// FileSpeechLogger が SpeechLogger を実装していることをコンパイル時に検証する。
var _ SpeechLogger = (*FileSpeechLogger)(nil)
var _ SpeechLogger = (*NoopSpeechLogger)(nil)
