package monitor

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"sakura-kodama/internal/types"

	"github.com/fsnotify/fsnotify"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	pollInterval       = 2 * time.Second
	logPollInterval    = 500 * time.Millisecond
	fileChangeDebounce = 500 * time.Millisecond
	claudeProcess      = "claude"
)

var exitCodeRegex = regexp.MustCompile(`(?i)exit code[^0-9]*(-?\d+)`)

type dirWatcher interface {
	Add(name string) error
}

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

// Detector は AI Agent, Dev, System の 3 層でシグナルを検知する。
type Detector struct {
	signals        chan types.Signal
	proc           chan ProcessEvent
	fileChanges    chan struct{}
	watcher        *fsnotify.Watcher
	mu             sync.Mutex
	lastSignal     time.Time
	logPaths       []string
	logOffsets     map[string]int64
	debounceMu     sync.Mutex
	debounceActive bool
	exitCodeMu     sync.Mutex
	pendingExit    int
	
	// レイヤーごとのパス管理
	pathLayers map[string]types.SignalSource
}

// NewDetector は指定されたディレクトリを 3 層モデルで監視する Detector を作成する。
func NewDetector(logPaths []string, watchDir string) (*Detector, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	d := &Detector{
		signals:     make(chan types.Signal, 64),
		proc:        make(chan ProcessEvent, 8),
		fileChanges: make(chan struct{}, 1),
		watcher:     watcher,
		lastSignal:  time.Now(),
		logPaths:    logPaths,
		logOffsets:  make(map[string]int64),
		pathLayers:  make(map[string]types.SignalSource),
	}

	d.setupLayers(watchDir)

	// 起動時は各ログファイルの先頭から開始（デバッグ用）
	for _, lp := range logPaths {
		if lp != "" {
			d.logOffsets[lp] = 0
		}
	}

	return d, nil
}

func (d *Detector) setupLayers(watchDir string) {
	home, _ := os.UserHomeDir()

	// Layer 1: AI Agent Activity
	aiDirs := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".codex"),
		filepath.Join(home, ".aider"),
		filepath.Join(home, ".cursor"),
		filepath.Join(home, ".config", "gemini"),
	}
	for _, dir := range aiDirs {
		if exists(dir) {
			_ = d.addWatchRecursive(dir, types.SourceAgent)
		}
	}

	// Layer 2: Development Activity
	if watchDir != "" && exists(watchDir) {
		_ = d.addWatchRecursive(watchDir, types.SourceFS)
		
		// Editor configs explicitly
		_ = d.addWatchRecursive(filepath.Join(watchDir, ".vscode"), types.SourceFS)
		_ = d.addWatchRecursive(filepath.Join(watchDir, ".idea"), types.SourceFS)
	}

	// Layer 3: System Activity (Non-recursive to stay lightweight)
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	xdgData := os.Getenv("XDG_DATA_HOME")
	if xdgData == "" {
		xdgData = filepath.Join(home, ".local", "share")
	}
	xdgState := os.Getenv("XDG_STATE_HOME")
	if xdgState == "" {
		xdgState = filepath.Join(home, ".local", "state")
	}
	xdgCache := os.Getenv("XDG_CACHE_HOME")
	if xdgCache == "" {
		xdgCache = filepath.Join(home, ".cache")
	}

	systemDirs := []string{xdgConfig, xdgData, xdgState, xdgCache}
	for _, dir := range systemDirs {
		if exists(dir) {
			_ = d.watcher.Add(dir)
			d.pathLayers[dir] = types.SourceSystem
		}
	}
}

func (d *Detector) addWatchRecursive(root string, layer types.SignalSource) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if strings.Contains(path, "node_modules") || strings.Contains(path, ".git/objects") {
				return filepath.SkipDir
			}
			err = d.watcher.Add(path)
			if err == nil {
				d.pathLayers[path] = layer
			}
		}
		return nil
	})
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Run はプロセス検知とファイル監視を開始する（goroutine内で呼ぶ）。
func (d *Detector) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var claudeRunning bool

	go d.tailLog(ctx)

	for {
		select {
		case <-ctx.Done():
			d.watcher.Close()
			return

		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}
			
			// 新しいディレクトリが作成されたら監視対象に追加（Layer 2 のみ再帰的に追加）
			if event.Op&fsnotify.Create == fsnotify.Create {
				if layer, ok := d.pathLayers[filepath.Dir(event.Name)]; ok {
					if layer == types.SourceFS {
						_ = d.addWatchRecursive(event.Name, types.SourceFS)
					} else {
						// Layer 1, 3 は直下のみ
						info, err := os.Stat(event.Name)
						if err == nil && info.IsDir() {
							_ = d.watcher.Add(event.Name)
							d.pathLayers[event.Name] = layer
						}
					}
				}
			}

			d.recordSignal(event.Name)
			d.triggerFileChange()

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
				d.proc <- ProcessEvent{Type: ProcessExited, ExitCode: d.popExitCode()}
			}
		}
	}
}

// recordSignal はファイルパスからシグナルを生成してチャネルへ送る。
func (d *Detector) recordSignal(name string) {
	d.touchSignal()

	source := types.SourceFS // Default
	for p, l := range d.pathLayers {
		if strings.HasPrefix(name, p) {
			source = l
			break
		}
	}

	base := filepath.Base(name)
	msg := ""
	switch {
	case strings.HasSuffix(base, "_test.go"):
		msg = "go test"
	case strings.HasSuffix(base, ".go"):
		msg = "generate"
	case base == "Makefile" || strings.HasSuffix(base, ".sh"):
		msg = "lint"
	}

	select {
	case d.signals <- types.Signal{
		Type:      types.SigFileModified,
		Source:    source,
		Value:     name,
		Message:   msg,
		Timestamp: types.TimeToStr(time.Now()),
	}:
	default:
	}
}

func (d *Detector) touchSignal() {
	d.mu.Lock()
	d.lastSignal = time.Now()
	d.mu.Unlock()
}

func (d *Detector) triggerFileChange() {
	d.debounceMu.Lock()
	if d.debounceActive {
		d.debounceMu.Unlock()
		return
	}
	d.debounceActive = true
	d.debounceMu.Unlock()

	go func() {
		time.Sleep(fileChangeDebounce)
		select {
		case d.fileChanges <- struct{}{}:
		default:
		}
		d.debounceMu.Lock()
		d.debounceActive = false
		d.debounceMu.Unlock()
	}()
}

// Signals はシグナルチャネルを返す。
func (d *Detector) Signals() <-chan types.Signal {
	return d.signals
}

// FileChanges はデバウンス済みのファイル変更イベントを返す。
func (d *Detector) FileChanges() <-chan struct{} {
	return d.fileChanges
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

func (d *Detector) tailLog(ctx context.Context) {
	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.readLog()
		}
	}
}

func (d *Detector) readLog() {
	for _, lp := range d.logPaths {
		if lp == "" {
			continue
		}
		d.readFile(lp)
	}
}

func (d *Detector) readFile(lp string) {
	f, err := os.Open(lp)
	if err != nil {
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return
	}
	size := info.Size()
	
	offset := d.logOffsets[lp]

	if size < offset {
		offset = 0
	}
	
	if size == offset {
		return
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}
	
	buf, err := io.ReadAll(f)
	if err != nil {
		return
	}
	
	d.logOffsets[lp] = size
	if len(buf) == 0 {
		return
	}

	lines := strings.Split(string(buf), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		d.touchSignal()
		select {
		case d.signals <- types.Signal{
			Type:      types.SigLogHint,
			Source:    types.SourceAgent,
			Value:     lp,
			Message:   line,
			Timestamp: types.TimeToStr(time.Now()),
		}:
		default:
		}
		d.inspectLogLine(line)
	}
}

func (d *Detector) inspectLogLine(line string) {
	if matches := exitCodeRegex.FindStringSubmatch(line); len(matches) > 1 {
		code, err := strconv.Atoi(matches[1])
		if err == nil {
			d.setPendingExitCode(code)
		}
	}
}

func (d *Detector) setPendingExitCode(code int) {
	d.exitCodeMu.Lock()
	defer d.exitCodeMu.Unlock()
	d.pendingExit = code
}

func (d *Detector) popExitCode() int {
	d.exitCodeMu.Lock()
	defer d.exitCodeMu.Unlock()
	code := d.pendingExit
	d.pendingExit = 0
	return code
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
