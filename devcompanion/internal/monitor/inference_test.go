package monitor

import (
	"strings"
	"testing"
	"time"
)

// --- 単一シグナルのスコアリングテスト ---

func TestAddLine_GoTest_ScoresRunTests(t *testing.T) {
	// Given: "go test" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("Running: go test ./...")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: RunTests が選ばれる（スコア +3 が他を上回る）
	if task != TaskRunTests {
		t.Errorf("want %s, got %s", TaskRunTests, task)
	}
}

func TestAddLine_FAIL_ScoresFixFailing(t *testing.T) {
	// Given: "FAIL" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("FAIL\texample.com/mymodule\t0.123s")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: FixFailingTests が選ばれる
	if task != TaskFixFailingTests {
		t.Errorf("want %s, got %s", TaskFixFailingTests, task)
	}
}

func TestAddLine_panic_ScoresDebug(t *testing.T) {
	// Given: "panic" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("panic: runtime error: index out of range [0] with length 0")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: Debug が選ばれる（スコア +4 が最大）
	if task != TaskDebug {
		t.Errorf("want %s, got %s", TaskDebug, task)
	}
}

func TestAddLine_lint_ScoresLintFormat(t *testing.T) {
	// Given: "lint" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("golangci-lint run ./...")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: LintFormat が選ばれる（スコア +2）
	if task != TaskLintFormat {
		t.Errorf("want %s, got %s", TaskLintFormat, task)
	}
}

func TestAddLine_fmt_ScoresLintFormat(t *testing.T) {
	// Given: "fmt" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("gofmt -w .")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: LintFormat が選ばれる（スコア +2）
	if task != TaskLintFormat {
		t.Errorf("want %s, got %s", TaskLintFormat, task)
	}
}

func TestAddLine_generate_ScoresGenerateCode(t *testing.T) {
	// Given: "generate" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("Running: generate handler for users")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: GenerateCode が選ばれる（スコア +2）
	if task != TaskGenerateCode {
		t.Errorf("want %s, got %s", TaskGenerateCode, task)
	}
}

func TestAddLine_JapaneseWrite_ScoresGenerateCode(t *testing.T) {
	// Given: "写" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("コードを写して")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: GenerateCode が選ばれる（スコア +2）
	if task != TaskGenerateCode {
		t.Errorf("want %s, got %s", TaskGenerateCode, task)
	}
}

func TestAddLine_JapaneseImplement_ScoresGenerateCode(t *testing.T) {
	// Given: "実装" を含む行をバッファに追加
	ti := NewTaskInferrer()
	ti.AddLine("ユーザー認証を実装する")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: GenerateCode が選ばれる（スコア +2）
	if task != TaskGenerateCode {
		t.Errorf("want %s, got %s", TaskGenerateCode, task)
	}
}

// --- 無音シグナルのテスト ---

func TestInfer_Silence5s_ScoresPlan(t *testing.T) {
	// Given: シグナルなし・5秒以上の無音
	ti := NewTaskInferrer()

	// When: 5秒の無音で推論
	task := ti.Infer(5 * time.Second)

	// Then: Plan が選ばれる（無音 +2、他のシグナルなし）
	if task != TaskPlan {
		t.Errorf("want %s, got %s", TaskPlan, task)
	}
}

func TestInfer_SilenceBelowThreshold_NotPlan(t *testing.T) {
	// Given: "panic" シグナルあり・無音 4 秒（閾値未満）
	ti := NewTaskInferrer()
	ti.AddLine("panic: error")

	// When: 4秒の無音で推論（Plan +0, Debug +4）
	task := ti.Infer(4 * time.Second)

	// Then: Debug が選ばれる（無音がPlanに加算されないため）
	if task != TaskDebug {
		t.Errorf("want %s (silence < 5s should not add Plan score), got %s", TaskDebug, task)
	}
}

func TestInfer_NoSignals_NoSilence_DefaultsPlan(t *testing.T) {
	// Given: シグナルなし・無音なし
	ti := NewTaskInferrer()

	// When: 無音 0 で推論
	task := ti.Infer(0)

	// Then: Plan がデフォルトとして返る（全スコアが 0 のとき）
	if task != TaskPlan {
		t.Errorf("want %s as default, got %s", TaskPlan, task)
	}
}

// --- 複合シグナルのテスト ---

func TestInfer_MultiSignal_MaxScoreWins(t *testing.T) {
	// Given: "panic"（Debug +4）と "go test"（RunTests +3）を両方追加
	ti := NewTaskInferrer()
	ti.AddLine("go test ./...")  // RunTests +3
	ti.AddLine("panic: runtime") // Debug +4

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: Debug が勝つ（4 > 3）
	if task != TaskDebug {
		t.Errorf("want %s (panic score 4 > go test score 3), got %s", TaskDebug, task)
	}
}

func TestInfer_GoTestWinsOverSilence(t *testing.T) {
	// Given: "go test" シグナルあり・5秒以上の無音
	ti := NewTaskInferrer()
	ti.AddLine("go test ./...") // RunTests +3

	// When: 6秒の無音で推論（Plan +2）
	task := ti.Infer(6 * time.Second)

	// Then: RunTests が勝つ（3 > 2）
	if task != TaskRunTests {
		t.Errorf("want %s (go test +3 > silence +2), got %s", TaskRunTests, task)
	}
}

func TestInfer_SameScoredSignals_GoTestAndGenerate(t *testing.T) {
	// Given: "FAIL"（RunTests +2）と "generate"（GenerateCode +2）を追加
	ti := NewTaskInferrer()
	ti.AddLine("FAIL")
	ti.AddLine("generate code")

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: どちらかのタスクが返る（同スコア時の決定論的な選択はあること）
	if task != TaskRunTests && task != TaskGenerateCode {
		t.Errorf("want %s or %s (tie), got %s", TaskRunTests, TaskGenerateCode, task)
	}
}

// --- リングバッファのテスト ---

func TestInfer_RingBuffer_OldSignalEvicted(t *testing.T) {
	// Given: バッファを "go test"（RunTests +3 each）で20行満たし、
	//        21行目に "panic"（Debug +4）を追加
	ti := NewTaskInferrer()

	for i := 0; i < 20; i++ {
		ti.AddLine("go test ./...") // RunTests +3 × 20 = 60
	}
	ti.AddLine("panic: error") // 先頭の "go test" が退出、Debug +4

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: RunTests が勝つ（19×3=57 > Debug の 4）
	if task != TaskRunTests {
		t.Errorf("want %s (19 go-test lines outweigh 1 panic), got %s", TaskRunTests, task)
	}
}

func TestInfer_RingBuffer_CompletelyOverwritten(t *testing.T) {
	// Given: バッファを "panic" で20行満たし、続けて "go test" で20行上書き
	ti := NewTaskInferrer()

	for i := 0; i < 20; i++ {
		ti.AddLine("panic: error") // Debug +4 × 20 = 80
	}
	for i := 0; i < 20; i++ {
		ti.AddLine("go test ./...") // RunTests +3 × 20 = 60
	}

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: RunTests が勝つ（panic がすべてバッファから退出）
	if task != TaskRunTests {
		t.Errorf("want %s (all panic evicted), got %s", TaskRunTests, task)
	}
}

func TestInfer_RingBuffer_MaxCapacity(t *testing.T) {
	// Given: バッファ容量（20行）を確認するためのテスト
	//        容量を超えた21行目以降が古い行を正しく上書きするか
	ti := NewTaskInferrer()

	// "lint" で21行追加（20行分がバッファに残る）
	for i := 0; i < 21; i++ {
		ti.AddLine("lint run") // LintFormat +2 each
	}

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: LintFormat が選ばれる（バッファが満杯でも正常動作）
	if task != TaskLintFormat {
		t.Errorf("want %s, got %s", TaskLintFormat, task)
	}
}

// --- ヘルパー: contains がバッファ内の複数シグナルを合算するか ---

func TestInfer_MultipleMatchesInBuffer_AccumulateScore(t *testing.T) {
	// Given: "go test" を3行追加（RunTests +9）、"panic" を1行追加（Debug +4）
	ti := NewTaskInferrer()

	for i := 0; i < 3; i++ {
		ti.AddLine("go test ./...") // RunTests +3 per line
	}
	ti.AddLine("panic: runtime error") // Debug +4

	// When: 無音なしで推論
	task := ti.Infer(0)

	// Then: RunTests が勝つ（累積 9 > 4）
	if task != TaskRunTests {
		t.Errorf("want %s (accumulated score 9 > 4), got %s", TaskRunTests, task)
	}
}

func TestInfer_ExitCodeLine_ScoresDebug(t *testing.T) {
	// Given: 非ゼロ exit code を含むログ行
	ti := NewTaskInferrer()
	ti.AddLine("process exited with code 2")

	// When: 推論
	task := ti.Infer(0)

	// Then: Debug が選ばれる（exit code != 0 のバイアス）
	if task != TaskDebug {
		t.Errorf("want %s for exit code line, got %s", TaskDebug, task)
	}
}

func TestInfer_FailureSignal_BiasesFixFailingTests(t *testing.T) {
	// Given: FAIL ログが最新バッファに入っている
	ti := NewTaskInferrer()
	ti.AddLine("FAIL\tsakura-kodama/internal/monitor 0.34s")

	// When: 推論
	task := ti.Infer(0)

	// Then: FixFailingTests を優先
	if task != TaskFixFailingTests {
		t.Errorf("want %s when FAIL is observed, got %s", TaskFixFailingTests, task)
	}
}

func TestInfer_RecentSignalsOutweighOldOnes(t *testing.T) {
	// Given: 古い panic シグナル多数と直近の go test シグナル
	ti := NewTaskInferrer()
	for i := 0; i < 10; i++ {
		ti.AddLine("panic: runtime error")
	}
	ti.AddLine("go test ./...")

	// When: 推論
	task := ti.Infer(0)

	// Then: 直近シグナル（go test）が優先される
	if task != TaskRunTests {
		t.Errorf("want %s favored by recency decay, got %s", TaskRunTests, task)
	}
}

// --- ヘルパー: AddLine が空文字列を安全に処理するか ---

func TestAddLine_EmptyString_NoEffect(t *testing.T) {
	// Given: 空文字列を追加後 "go test" を追加
	ti := NewTaskInferrer()
	ti.AddLine("")
	ti.AddLine("go test ./...")

	// When: 推論
	task := ti.Infer(0)

	// Then: RunTests が選ばれる（空行が干渉しない）
	if task != TaskRunTests {
		t.Errorf("want %s, got %s", TaskRunTests, task)
	}
}

// --- ヘルパー: strings パッケージが test 内でインポートされることを確認 ---

func TestAddLine_CaseInsensitive_NotApplied(t *testing.T) {
	// シグナルマッチは大文字小文字を区別することを確認
	// "GO TEST" は "go test" とマッチしない
	ti := NewTaskInferrer()
	ti.AddLine(strings.ToUpper("go test ./...")) // "GO TEST ./..."

	// When: 推論（"GO TEST" はシグナルにマッチしないはず）
	task := ti.Infer(0)

	// Then: Plan がデフォルトとして返る（大文字はマッチしない）
	if task != TaskPlan {
		t.Errorf("want %s (uppercase should not match signal), got %s", TaskPlan, task)
	}
}
