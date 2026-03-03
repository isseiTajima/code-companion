package monitor

import (
	"testing"
)

// --- 結合テスト: Fail→Editing の特別ルール ---
//
// 特別ルール: 直前の State が Fail で次の State が Editing に遷移したとき、
// Task を FixFailingTests に強制設定する。
// このルールは monitor.go の Run() ループが管理するが、
// テスタビリティのために applySpecialTaskRule() として純粋関数に抽出する。

func TestApplySpecialTaskRule_FailToEditing_SetsFixFailingTests(t *testing.T) {
	// Given: 直前が Fail・次が Editing・現在のタスクが GenerateCode
	prevState := StateFail
	nextState := StateEditing
	currentTask := TaskGenerateCode

	// When: 特別ルールを適用
	result := applySpecialTaskRule(prevState, nextState, currentTask)

	// Then: FixFailingTests に強制変更される
	if result != TaskFixFailingTests {
		t.Errorf("want %s, got %s", TaskFixFailingTests, result)
	}
}

func TestApplySpecialTaskRule_FailToEditing_AnyTask_SetsFixFailingTests(t *testing.T) {
	// Given: 直前が Fail・次が Editing・様々なタスク
	prevState := StateFail
	nextState := StateEditing

	for _, task := range []TaskType{TaskPlan, TaskRunTests, TaskDebug, TaskLintFormat, TaskGenerateCode} {
		// When: 特別ルールを適用
		result := applySpecialTaskRule(prevState, nextState, task)

		// Then: 常に FixFailingTests
		if result != TaskFixFailingTests {
			t.Errorf("task=%s: want %s, got %s", task, TaskFixFailingTests, result)
		}
	}
}

func TestApplySpecialTaskRule_NonFail_PrevState_PreservesTask(t *testing.T) {
	// Given: 直前が Fail 以外・次が Editing
	nextState := StateEditing
	task := TaskRunTests

	for _, prevState := range []StateType{StateIdle, StateRunning, StateThinking, StateEditing, StateSuccess} {
		// When: 特別ルールを適用
		result := applySpecialTaskRule(prevState, nextState, task)

		// Then: タスクが変更されない（Fail→Editing の場合のみ特別ルールが適用される）
		if result != task {
			t.Errorf("prevState=%s: want %s (task unchanged), got %s", prevState, task, result)
		}
	}
}

func TestApplySpecialTaskRule_FailToDifferentState_PreservesTask(t *testing.T) {
	// Given: 直前が Fail・次が Editing 以外
	prevState := StateFail
	task := TaskRunTests

	for _, nextState := range []StateType{StateIdle, StateRunning, StateThinking, StateSuccess, StateFail} {
		// When: 特別ルールを適用
		result := applySpecialTaskRule(prevState, nextState, task)

		// Then: タスクが変更されない（次が Editing でないため）
		if result != task {
			t.Errorf("nextState=%s: want %s (task unchanged), got %s", nextState, task, result)
		}
	}
}

func TestApplySpecialTaskRule_FixFailingTests_AlreadySet_StaysFixed(t *testing.T) {
	// Given: 直前が Fail・次が Editing・タスクが既に FixFailingTests
	prevState := StateFail
	nextState := StateEditing
	task := TaskFixFailingTests

	// When: 特別ルールを適用
	result := applySpecialTaskRule(prevState, nextState, task)

	// Then: FixFailingTests のまま
	if result != TaskFixFailingTests {
		t.Errorf("want %s, got %s", TaskFixFailingTests, result)
	}
}
