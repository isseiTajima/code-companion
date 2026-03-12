package session

import (
	"time"

	"sakura-kodama/internal/types"
)

// Tracker は行動の履歴からセッション状態を管理する。
type Tracker struct {
	state types.SessionState
}

// NewTracker は Tracker を初期化する。
func NewTracker() *Tracker {
	return &Tracker{
		state: types.SessionState{
			Mode:      types.ModeIdle,
			StartTime: types.TimeToStr(time.Now()),
		},
	}
}

// Update は現在の行動に基づいてセッション状態を更新する。
func (t *Tracker) Update(b types.Behavior, now time.Time) types.SessionState {
	t.state.LastActivity = types.TimeToStr(now)

	// モード推論ロジック
	switch b.Type {
	case types.BehaviorCoding, types.BehaviorRefactoring:
		if t.state.Mode == types.ModeProductiveFlow || t.state.Mode == types.ModeDeepFocus {
			// 継続的な活動で Deep Focus へ
			t.state.FocusLevel += 0.1
			if t.state.FocusLevel >= 0.8 {
				t.state.Mode = types.ModeDeepFocus
			}
		} else {
			t.state.Mode = types.ModeProductiveFlow
			t.state.FocusLevel = 0.5
		}
	case types.BehaviorDebugging:
		t.state.Mode = types.ModeStruggling
		t.state.FocusLevel = 0.9 // デバッグ中は集中力は高い
	case types.BehaviorResearching:
		t.state.Mode = types.ModeCasualWork
		t.state.FocusLevel = 0.6
	case types.BehaviorBreak, types.BehaviorProcrastinating:
		t.state.Mode = types.ModeOnBreak
		t.state.FocusLevel = 0.1
	default:
		t.state.Mode = types.ModeCasualWork
	}

	if t.state.FocusLevel > 1.0 {
		t.state.FocusLevel = 1.0
	}
	if t.state.FocusLevel < 0.0 {
		t.state.FocusLevel = 0.0
	}

	return t.state
}

// GetState は現在のセッション状態を返す。
func (t *Tracker) GetState() types.SessionState {
	return t.state
}
