package monitor

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	pollInterval  = 2 * time.Second
	claudeProcess = "claude"
)

// ProcessEventType はプロセスイベントの種別を表す。
type ProcessEventType int

const (
	ProcessStarted ProcessEventType = iota
	ProcessExited
)

// ProcessEvent はプロセスの起動・終了イベント。
type ProcessEvent struct {
	Type     ProcessEventType
	ExitCode int
}

// Detector はClaudeプロセスとファイル変更を監視する。
type Detector struct {
	signals     chan string
	proc        chan ProcessEvent
	watcher     *fsnotify.Watcher
	mu          sync.Mutex
	lastSignal  time.Time
}

// NewDetector は watchDir 以下のファイル変更を監視する Detector を作成する。
func NewDetector(watchDir string) (*Detector, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := addDirsRecursive(watcher, watchDir); err != nil {
		watcher.Close()
		return nil, err
	}

	d := &Detector{
		signals:    make(chan string, 64),
		proc:       make(chan ProcessEvent, 8),
		watcher:    watcher,
		lastSignal: time.Now(),
	}
	return d, nil
}

// addDirsRecursive は watchDir 以下の全ディレクトリを watcher に登録する。
func addDirsRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // アクセスできないディレクトリはスキップ
		}
		if d.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}

// Run はプロセス検知とファイル監視を開始する（goroutine内で呼ぶ）。
func (d *Detector) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var claudeRunning bool

	for {
		select {
		case <-ctx.Done():
			d.watcher.Close()
			return

		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}
			d.recordSignal(event.Name)

		case err, ok := <-d.watcher.Errors:
			if !ok {
				return
			}
			_ = err // ファイル監視エラーはサイレントに無視

		case <-ticker.C:
			running := isClaudeRunning()
			if running && !claudeRunning {
				claudeRunning = true
				d.proc <- ProcessEvent{Type: ProcessStarted}
			} else if !running && claudeRunning {
				claudeRunning = false
				d.proc <- ProcessEvent{Type: ProcessExited, ExitCode: 0}
			}
		}
	}
}

// recordSignal はファイルパスからシグナル文字列を生成してチャネルへ送る。
func (d *Detector) recordSignal(name string) {
	d.mu.Lock()
	d.lastSignal = time.Now()
	d.mu.Unlock()

	base := filepath.Base(name)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		d.send("go test")
	case strings.HasSuffix(base, ".go"):
		d.send("generate")
	case base == "Makefile" || strings.HasSuffix(base, ".sh"):
		d.send("lint")
	}
}

func (d *Detector) send(sig string) {
	select {
	case d.signals <- sig:
	default: // バッファ満杯時は破棄
	}
}

// Signals はシグナル行チャネルを返す。
func (d *Detector) Signals() <-chan string {
	return d.signals
}

// ProcessEvents はプロセスイベントチャネルを返す。
func (d *Detector) ProcessEvents() <-chan ProcessEvent {
	return d.proc
}

// SilenceDuration は最後のシグナルからの経過時間を返す。
func (d *Detector) SilenceDuration() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Since(d.lastSignal)
}

// isClaudeRunning は "claude" プロセスが実行中かを返す。
func isClaudeRunning() bool {
	procs, err := process.Processes()
	if err != nil {
		return false
	}
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(name), claudeProcess) {
			return true
		}
	}
	return false
}
