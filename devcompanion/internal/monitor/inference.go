package monitor

import (
	"strings"
	"time"
)

// TaskType はClaudeが現在行っている作業の種類を表す。
type TaskType string

const (
	TaskPlan            TaskType = "Plan"
	TaskGenerateCode    TaskType = "GenerateCode"
	TaskRunTests        TaskType = "RunTests"
	TaskLintFormat      TaskType = "LintFormat"
	TaskDebug           TaskType = "Debug"
	TaskFixFailingTests TaskType = "FixFailingTests"
)

const ringBufferCap = 20

// TaskInferrer はシグナル行バッファを保持し、最もスコアの高いTaskを推論する。
type TaskInferrer struct {
	buffer []string
}

// NewTaskInferrer は TaskInferrer を初期化する。
func NewTaskInferrer() *TaskInferrer {
	return &TaskInferrer{
		buffer: make([]string, 0, ringBufferCap),
	}
}

// AddLine はシグナル行をリングバッファに追加する。
// バッファが満杯の場合は最古の行を削除する。
func (ti *TaskInferrer) AddLine(line string) {
	if len(ti.buffer) >= ringBufferCap {
		ti.buffer = ti.buffer[1:]
	}
	ti.buffer = append(ti.buffer, line)
}

// Infer は現在のバッファと無音時間からTaskを推論する。
// 全スコアが0の場合は TaskPlan をデフォルトとして返す。
func (ti *TaskInferrer) Infer(silenceDuration time.Duration) TaskType {
	scores := map[TaskType]int{}

	for _, line := range ti.buffer {
		if strings.Contains(line, "go test") {
			scores[TaskRunTests] += 3
		}
		if strings.Contains(line, "FAIL") {
			scores[TaskRunTests] += 2
		}
		if strings.Contains(line, "panic") {
			scores[TaskDebug] += 4
		}
		if strings.Contains(line, "lint") || strings.Contains(line, "fmt") {
			scores[TaskLintFormat] += 2
		}
		if strings.Contains(line, "generate") || strings.Contains(line, "写") || strings.Contains(line, "実装") {
			scores[TaskGenerateCode] += 2
		}
	}

	if silenceDuration >= 5*time.Second {
		scores[TaskPlan] += 2
	}

	return argmax(scores)
}

// argmax はスコアマップの最大値を持つTaskを返す。
// 全スコアが0またはマップが空の場合は TaskPlan を返す。
func argmax(scores map[TaskType]int) TaskType {
	best := TaskPlan
	bestScore := 0
	for task, score := range scores {
		if score > bestScore {
			bestScore = score
			best = task
		}
	}
	return best
}
