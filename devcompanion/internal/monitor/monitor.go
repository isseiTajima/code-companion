package monitor

import (
	"context"
	"time"

	"devcompanion/internal/config"
)

const defaultSilenceThresholdDuration = 5 * time.Second

// MonitorEvent はモニタリング結果を表す。
type MonitorEvent struct {
	State StateType
	Task  TaskType
	Mood  MoodType
}

// Monitor はプロセス検知・タスク推論・State遷移を束ねる。
type Monitor struct {
	detector *Detector
	inferrer *TaskInferrer
	events   chan MonitorEvent
	cfg      *config.Config
}

// New は Monitor を作成する。watchDir はfsnotifyの監視対象ディレクトリ。
func New(cfg *config.Config, watchDir string) (*Monitor, error) {
	detector, err := NewDetector(watchDir)
	if err != nil {
		return nil, err
	}
	return &Monitor{
		detector: detector,
		inferrer: NewTaskInferrer(),
		events:   make(chan MonitorEvent, 16),
		cfg:      cfg,
	}, nil
}

// Run はメインループを開始する（goroutine内で呼ぶ）。
func (m *Monitor) Run(ctx context.Context) {
	go m.detector.Run(ctx)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	currentState := StateIdle
	currentTask := TaskPlan
	processRunning := false

	for {
		select {
		case <-ctx.Done():
			return

		case line := <-m.detector.Signals():
			m.inferrer.AddLine(line)

		case pe := <-m.detector.ProcessEvents():
			prevState := currentState
			input := buildTransitionInput(processRunning, pe, m.detector.SilenceDuration(), defaultSilenceThresholdDuration)
			if pe.Type == ProcessStarted {
				processRunning = true
			} else {
				processRunning = false
			}
			newState := Transition(currentState, input)
			newTask := applySpecialTaskRule(prevState, newState, currentTask)
			newMood := InferMood(newState, newTask)
			if newState != currentState || newTask != currentTask {
				m.events <- MonitorEvent{newState, newTask, newMood}
				currentState = newState
				currentTask = newTask
			}

		case <-ticker.C:
			input := TransitionInput{
				ProcessRunning:   processRunning,
				SilenceDuration:  m.detector.SilenceDuration(),
				SilenceThreshold: defaultSilenceThresholdDuration,
			}
			prevState := currentState
			newState := Transition(currentState, input)
			newTask := m.inferrer.Infer(m.detector.SilenceDuration())
			newTask = applySpecialTaskRule(prevState, newState, newTask)
			newMood := InferMood(newState, newTask)
			if newState != currentState || newTask != currentTask {
				m.events <- MonitorEvent{newState, newTask, newMood}
				currentState = newState
				currentTask = newTask
			}
		}
	}
}

// Events はMonitorEventチャネルを返す。
func (m *Monitor) Events() <-chan MonitorEvent {
	return m.events
}

// applySpecialTaskRule は Fail→Editing 遷移時にTaskをFixFailingTestsに強制する純粋関数。
func applySpecialTaskRule(prev, next StateType, task TaskType) TaskType {
	if prev == StateFail && next == StateEditing {
		return TaskFixFailingTests
	}
	return task
}

// buildTransitionInput は ProcessEvent からTransitionInputを構築する。
func buildTransitionInput(wasRunning bool, pe ProcessEvent, silence, threshold time.Duration) TransitionInput {
	if pe.Type == ProcessStarted {
		return TransitionInput{
			ProcessRunning:   true,
			SilenceDuration:  silence,
			SilenceThreshold: threshold,
		}
	}
	return TransitionInput{
		ProcessRunning:   false,
		ProcessExited:    true,
		ExitCode:         pe.ExitCode,
		SilenceThreshold: threshold,
	}
}
