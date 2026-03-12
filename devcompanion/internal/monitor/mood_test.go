package monitor

import (
	"sakura-kodama/internal/types"
	"testing"
)

func TestInferMood_SuccessReturnsHappy(t *testing.T) {
	ev := MonitorEvent{State: types.StateSuccess}
	mood := InferMood(ev)
	if mood != MoodHappy {
		t.Errorf("want %s for success state, got %s", MoodHappy, mood)
	}
}

func TestInferMood_FailReturnsConcerned(t *testing.T) {
	ev := MonitorEvent{State: types.StateFail}
	mood := InferMood(ev)
	if mood != MoodConcerned {
		t.Errorf("want %s for fail state, got %s", MoodConcerned, mood)
	}
}

func TestInferMood_CodingGenerateCodeIsFocus(t *testing.T) {
	ev := MonitorEvent{State: types.StateCoding, Task: TaskGenerateCode}
	mood := InferMood(ev)
	if mood != MoodFocus {
		t.Errorf("want %s when coding & generating code, got %s", MoodFocus, mood)
	}
}

func TestInferMood_CodingDebugIsFocus(t *testing.T) {
	ev := MonitorEvent{State: types.StateCoding, Task: TaskDebug}
	mood := InferMood(ev)
	if mood != MoodFocus {
		t.Errorf("want %s for coding+debug, got %s", MoodFocus, mood)
	}
}

func TestInferMood_ThinkingReturnsFocus(t *testing.T) {
	ev := MonitorEvent{State: types.StateThinking}
	mood := InferMood(ev)
	if mood != MoodFocus {
		t.Errorf("want %s for thinking, got %s", MoodFocus, mood)
	}
}
