package observer

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/types"
)

// ObservationType は観察イベントの種類を表す。
type ObservationType string

const (
	ObsGitCommit    ObservationType = "git_commit"
	ObsGitPush      ObservationType = "git_push"
	ObsGitAdd       ObservationType = "git_add"
	ObsIdleStart    ObservationType = "idle_start"
	ObsNightWork    ObservationType = "night_work"
	ObsActiveEditing ObservationType = "active_editing"
)

const (
	idleThreshold       = 2 * time.Minute
	idleCooldown        = 3 * time.Minute
	nightWorkCooldown   = 30 * time.Minute
	activeEditWindow    = 2 * time.Minute
	activeEditThreshold = 10 // 3から10に引き上げ
	activeEditCooldown  = 10 * time.Minute // クールダウンも長く
	gitCooldown         = 5 * time.Minute
	observationBufSize  = 16
)

// DevObservation は観察されたイベントを表す。
type DevObservation struct {
	Type ObservationType
}

// DevObserver は開発者の行動を観察して DevObservation を発行する。
type DevObserver struct {
	mu           sync.Mutex
	observations chan DevObservation

	// thresholds
	idleThreshold time.Duration
	idleCooldown  time.Duration

	// idle tracking
	idleStart    time.Time
	lastIdleEmit time.Time

	// night work tracking
	lastNightWorkEmit time.Time

	// active editing tracking
	editTimes          []time.Time
	lastActiveEditEmit time.Time

	// git event tracking
	lastGitCommitEmit time.Time
	lastGitPushEmit   time.Time
	lastGitAddEmit    time.Time

	watcher *fsnotify.Watcher
	done    chan struct{}
}

func (o *DevObserver) UpdateFrequency(freq int) {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch freq {
	case 1: // 控えめ
		o.idleThreshold = 10 * time.Minute
		o.idleCooldown = 15 * time.Minute
	case 3: // お喋り
		o.idleThreshold = 1 * time.Minute
		o.idleCooldown = 2 * time.Minute
	default: // ふつう
		o.idleThreshold = 3 * time.Minute
		o.idleCooldown = 5 * time.Minute
	}
}

// NewDevObserver は DevObserver を初期化する。
// dir に .git/ が存在しない場合は watcher 登録をスキップし、エラーを返さない。
func NewDevObserver(dir string) (*DevObserver, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	o := &DevObserver{
		observations: make(chan DevObservation, observationBufSize),
		watcher:      watcher,
		done:         make(chan struct{}),
	}
	o.UpdateFrequency(2) // デフォルト「ふつう」で初期化

	// .git/ が存在する場合のみ watcher に登録する（best-effort）
	_ = watcher.Add(filepath.Join(dir, ".git", "COMMIT_EDITMSG"))
	_ = watcher.Add(filepath.Join(dir, ".git", "index"))
	_ = watcher.Add(filepath.Join(dir, ".git", "refs", "remotes"))
	_ = watcher.Add(filepath.Join(dir, ".git", "packed-refs"))

	go o.watchGit()

	return o, nil
}

// Observations は観察イベントの受信専用チャネルを返す。
func (o *DevObserver) Observations() <-chan DevObservation {
	return o.observations
}

// OnMonitorEvent は MonitorEvent を受け取り、idle / 深夜 / 高頻度編集を判断して発行する。
func (o *DevObserver) OnMonitorEvent(e monitor.MonitorEvent, now time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.checkNightWork(e, now)
	o.checkActiveEditing(e, now)
	o.checkIdle(e, now)
}

// checkIdle は idle 状態が 5 分以上継続した場合に ObsIdleStart を発行する。
func (o *DevObserver) checkIdle(e monitor.MonitorEvent, now time.Time) {
	if e.State != types.StateIdle {
		o.idleStart = time.Time{}
		return
	}

	if o.idleStart.IsZero() {
		o.idleStart = now
		return
	}

	if now.Sub(o.idleStart) < o.idleThreshold {
		return
	}
	if !o.lastIdleEmit.IsZero() && now.Sub(o.lastIdleEmit) < o.idleCooldown {
		return
	}

	o.send(DevObservation{Type: ObsIdleStart})
	o.lastIdleEmit = now
}

// checkNightWork は深夜帯（23:00〜04:59）にイベントを受信した場合に ObsNightWork を発行する。
func (o *DevObserver) checkNightWork(e monitor.MonitorEvent, now time.Time) {
	_ = e
	if !isNightHour(now.Hour()) {
		return
	}
	if !o.lastNightWorkEmit.IsZero() && now.Sub(o.lastNightWorkEmit) < nightWorkCooldown {
		return
	}

	o.send(DevObservation{Type: ObsNightWork})
	o.lastNightWorkEmit = now
}

// checkActiveEditing は 2 分ウィンドウ内に StateCoding が 3 回以上あった場合に ObsActiveEditing を発行する。
func (o *DevObserver) checkActiveEditing(e monitor.MonitorEvent, now time.Time) {
	if e.State != types.StateCoding {
		return
	}

	o.editTimes = append(o.editTimes, now)

	// 2 分ウィンドウ外のイベントを除去
	cutoff := now.Add(-activeEditWindow)
	valid := o.editTimes[:0]
	for _, t := range o.editTimes {
		if !t.Before(cutoff) {
			valid = append(valid, t)
		}
	}
	o.editTimes = valid

	if len(o.editTimes) < activeEditThreshold {
		return
	}
	if !o.lastActiveEditEmit.IsZero() && now.Sub(o.lastActiveEditEmit) < activeEditCooldown {
		return
	}

	o.send(DevObservation{Type: ObsActiveEditing})
	o.lastActiveEditEmit = now
	o.editTimes = nil
}

// watchGit は fsnotify イベントを受信して git commit / push を検知する。
func (o *DevObserver) watchGit() {
	for {
		select {
		case event, ok := <-o.watcher.Events:
			if !ok {
				return
			}
			o.handleGitEvent(event)
		case <-o.watcher.Errors:
			// watcher エラーは無視して継続
		case <-o.done:
			_ = o.watcher.Close()
			return
		}
	}
}

// handleGitEvent は fsnotify イベントから git commit / push を判別して発行する。
func (o *DevObserver) handleGitEvent(event fsnotify.Event) {
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()

	// COMMIT_EDITMSG の書き込みは git commit を示す
	if isCommitMsg(event.Name) && event.Has(fsnotify.Write) {
		if o.lastGitCommitEmit.IsZero() || now.Sub(o.lastGitCommitEmit) >= gitCooldown {
			o.send(DevObservation{Type: ObsGitCommit})
			o.lastGitCommitEmit = now
		}
		return
	}

	// index の変更は git add を示す
	if isGitIndex(event.Name) && event.Has(fsnotify.Write) {
		if o.lastGitAddEmit.IsZero() || now.Sub(o.lastGitAddEmit) >= 2*time.Minute {
			o.send(DevObservation{Type: ObsGitAdd})
			o.lastGitAddEmit = now
		}
		return
	}

	// refs/remotes/ または packed-refs の変更は push を示す（best-effort）
	if isPushIndicator(event.Name) {
		if o.lastGitPushEmit.IsZero() || now.Sub(o.lastGitPushEmit) >= gitCooldown {
			o.send(DevObservation{Type: ObsGitPush})
			o.lastGitPushEmit = now
		}
	}
}

// send は observations チャネルにノンブロッキングで送信する。
func (o *DevObserver) send(obs DevObservation) {
	select {
	case o.observations <- obs:
	default:
	}
}

// isNightHour は時刻が深夜帯（23:00〜04:59）かを返す。
func isNightHour(hour int) bool {
	return hour >= 23 || hour < 5
}

func isCommitMsg(name string) bool {
	return strings.HasSuffix(name, "COMMIT_EDITMSG")
}

func isGitIndex(name string) bool {
	return strings.HasSuffix(name, "index") && strings.Contains(name, ".git")
}

func isPushIndicator(name string) bool {
	return contains(name, "refs/remotes") || contains(name, "packed-refs")
}

func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
