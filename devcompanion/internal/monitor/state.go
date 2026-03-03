package monitor

import "time"

// StateType はアプリケーションの状態を表す。
type StateType string

const (
	StateIdle     StateType = "Idle"
	StateThinking StateType = "Thinking"
	StateEditing  StateType = "Editing"
	StateRunning  StateType = "Running"
	StateSuccess  StateType = "Success"
	StateFail     StateType = "Fail"
)

// TransitionInput は状態遷移の入力条件をまとめた構造体。
type TransitionInput struct {
	ProcessRunning   bool
	ProcessExited    bool
	ExitCode         int
	FileChanged      bool
	SilenceDuration  time.Duration
	SilenceThreshold time.Duration
}

// Transition は現在のStateと入力条件から次のStateを返す純粋関数。
// 優先順位（上位優先）:
//  1. プロセス未検出 → Idle
//  2. プロセス終了 exitCode=0 → Success
//  3. プロセス終了 exitCode≠0 → Fail
//  4. ファイル変更 → Editing
//  5. 無音≥閾値 → Thinking
//  6. プロセス実行中 → Running
func Transition(current StateType, in TransitionInput) StateType {
	if in.ProcessExited {
		if in.ExitCode == 0 {
			return StateSuccess
		}
		return StateFail
	}

	if !in.ProcessRunning {
		return StateIdle
	}

	if in.FileChanged {
		return StateEditing
	}

	if in.SilenceDuration >= in.SilenceThreshold {
		return StateThinking
	}

	return StateRunning
}
