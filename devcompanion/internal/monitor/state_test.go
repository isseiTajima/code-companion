package monitor

import (
	"testing"
	"time"
)

const defaultSilenceThreshold = 5 * time.Second

// --- プロセス未検出: Idle ---

func TestTransition_NoProcess_IsIdle(t *testing.T) {
	// Given: プロセスが実行されていない
	input := TransitionInput{
		ProcessRunning:   false,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: どの State からでも遷移
	for _, current := range []StateType{StateIdle, StateRunning, StateThinking, StateEditing, StateSuccess, StateFail} {
		got := Transition(current, input)

		// Then: 常に Idle に遷移する（最高優先順位）
		if got != StateIdle {
			t.Errorf("current=%s: want %s, got %s", current, StateIdle, got)
		}
	}
}

// --- プロセス終了: exitCode による Success / Fail ---

func TestTransition_ProcessExited_ExitCodeZero_IsSuccess(t *testing.T) {
	// Given: プロセスが正常終了（exitCode=0）
	input := TransitionInput{
		ProcessRunning:   false,
		ProcessExited:    true,
		ExitCode:         0,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Success
	if got != StateSuccess {
		t.Errorf("want %s, got %s", StateSuccess, got)
	}
}

func TestTransition_ProcessExited_ExitCodeNonZero_IsFail(t *testing.T) {
	// Given: プロセスが異常終了（exitCode!=0）
	input := TransitionInput{
		ProcessRunning:   false,
		ProcessExited:    true,
		ExitCode:         1,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Fail
	if got != StateFail {
		t.Errorf("want %s, got %s", StateFail, got)
	}
}

func TestTransition_ProcessExited_ExitCode2_IsFail(t *testing.T) {
	// Given: exitCode が 1 以外の非ゼロ値
	input := TransitionInput{
		ProcessRunning:   false,
		ProcessExited:    true,
		ExitCode:         2,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Fail（exitCode != 0 は全て Fail）
	if got != StateFail {
		t.Errorf("want %s for exitCode=2, got %s", StateFail, got)
	}
}

// --- ファイル変更: Editing ---

func TestTransition_FileChanged_IsEditing(t *testing.T) {
	// Given: プロセス実行中・ファイル変更あり
	input := TransitionInput{
		ProcessRunning:   true,
		FileChanged:      true,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Editing
	if got != StateEditing {
		t.Errorf("want %s, got %s", StateEditing, got)
	}
}

// --- 無音: Thinking ---

func TestTransition_Silence5s_IsThinking(t *testing.T) {
	// Given: プロセス実行中・無音時間が閾値ちょうど
	input := TransitionInput{
		ProcessRunning:   true,
		SilenceDuration:  5 * time.Second,
		SilenceThreshold: 5 * time.Second,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Thinking（SilenceDuration >= SilenceThreshold）
	if got != StateThinking {
		t.Errorf("want %s (silence=threshold), got %s", StateThinking, got)
	}
}

func TestTransition_Silence10s_IsThinking(t *testing.T) {
	// Given: 無音時間が閾値を大きく超えた場合
	input := TransitionInput{
		ProcessRunning:   true,
		SilenceDuration:  10 * time.Second,
		SilenceThreshold: 5 * time.Second,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Thinking
	if got != StateThinking {
		t.Errorf("want %s, got %s", StateThinking, got)
	}
}

func TestTransition_Silence4s_IsRunning(t *testing.T) {
	// Given: 無音時間が閾値未満（4秒 < 5秒）
	input := TransitionInput{
		ProcessRunning:   true,
		SilenceDuration:  4 * time.Second,
		SilenceThreshold: 5 * time.Second,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Running（閾値未満なので Thinking にならない）
	if got != StateRunning {
		t.Errorf("want %s (silence < threshold), got %s", StateRunning, got)
	}
}

// --- プロセス実行中・変化なし: Running ---

func TestTransition_ProcessRunning_NoChange_IsRunning(t *testing.T) {
	// Given: プロセス実行中・ファイル変更なし・無音なし
	input := TransitionInput{
		ProcessRunning:   true,
		FileChanged:      false,
		SilenceDuration:  0,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Running を維持
	if got != StateRunning {
		t.Errorf("want %s, got %s", StateRunning, got)
	}
}

// --- 優先順位テスト ---

// ProcessExited(ExitCode!=0) と FileChanged が同時: Fail が優先
func TestTransition_Priority_ExitCode_BeatsFileChanged(t *testing.T) {
	// Given: exitCode=1 かつ FileChanged=true（同時発生）
	input := TransitionInput{
		ProcessRunning:   false,
		ProcessExited:    true,
		ExitCode:         1,
		FileChanged:      true,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Fail が優先（exitCode チェックが FileChanged より先）
	if got != StateFail {
		t.Errorf("want %s (exitCode beats fileChanged), got %s", StateFail, got)
	}
}

// FileChanged と Silence が同時: FileChanged が優先
func TestTransition_Priority_FileChanged_BeatsSilence(t *testing.T) {
	// Given: FileChanged=true かつ SilenceDuration >= threshold
	input := TransitionInput{
		ProcessRunning:   true,
		FileChanged:      true,
		SilenceDuration:  10 * time.Second,
		SilenceThreshold: 5 * time.Second,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Editing が優先（FileChanged が Thinking より先）
	if got != StateEditing {
		t.Errorf("want %s (fileChanged beats silence), got %s", StateEditing, got)
	}
}

// !ProcessRunning が ExitCode より優先
func TestTransition_Priority_NoProcess_BeatsExitCode(t *testing.T) {
	// Given: ProcessRunning=false かつ ExitCode=0（ProcessExited=false）
	// ProcessRunning=false の場合は Idle に遷移（ExitCode は無視）
	input := TransitionInput{
		ProcessRunning:   false,
		ProcessExited:    false,
		ExitCode:         0,
		SilenceThreshold: defaultSilenceThreshold,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Idle（ProcessRunning=false が最優先）
	if got != StateIdle {
		t.Errorf("want %s (!ProcessRunning is highest priority), got %s", StateIdle, got)
	}
}

// --- カスタム閾値のテスト ---

func TestTransition_CustomThreshold_3s(t *testing.T) {
	// Given: カスタム閾値 3 秒・無音 3 秒
	input := TransitionInput{
		ProcessRunning:   true,
		SilenceDuration:  3 * time.Second,
		SilenceThreshold: 3 * time.Second,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Thinking（カスタム閾値が機能する）
	if got != StateThinking {
		t.Errorf("want %s with custom threshold 3s, got %s", StateThinking, got)
	}
}

func TestTransition_CustomThreshold_3s_Below(t *testing.T) {
	// Given: カスタム閾値 3 秒・無音 2 秒（閾値未満）
	input := TransitionInput{
		ProcessRunning:   true,
		SilenceDuration:  2 * time.Second,
		SilenceThreshold: 3 * time.Second,
	}

	// When: 遷移
	got := Transition(StateRunning, input)

	// Then: Running（閾値未満なので Thinking にならない）
	if got != StateRunning {
		t.Errorf("want %s (silence < custom threshold), got %s", StateRunning, got)
	}
}
