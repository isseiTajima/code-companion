package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sakura-kodama/internal/types"
)

// Recorder はシグナルをJSONL形式で記録する。
type Recorder struct {
	mu      sync.Mutex
	file    *os.File
	encoder *json.Encoder
	enabled bool
}

// New は指定されたパスでレコーダーを初期化する。
func New(enabled bool) (*Recorder, error) {
	if !enabled {
		return &Recorder{enabled: false}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(home, ".sakura-kodama", "signals")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	filename := "signals_" + time.Now().Format("20060102_150405") + ".jsonl"
	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return &Recorder{
		file:    f,
		encoder: json.NewEncoder(f),
		enabled: true,
	}, nil
}

// Record はシグナルを書き込む。
func (r *Recorder) Record(sig types.Signal) {
	if !r.enabled || r.file == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// エラーはログに出すか無視（メイン処理を止めないため）
	_ = r.encoder.Encode(sig)
}

// Close はファイルを閉じる。
func (r *Recorder) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}
