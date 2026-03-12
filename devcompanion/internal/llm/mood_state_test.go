package llm

import (
	"testing"
	"time"
)

func TestInferMoodState(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		lastEventTime time.Time
		successStreak int
		lastReason    Reason
		want          MoodState
	}{
		{
			name:          "ReasonFail should return MoodStateFail",
			lastEventTime: now,
			successStreak: 0,
			lastReason:    ReasonFail,
			want:          MoodStateFail,
		},
		{
			name:          "SuccessStreak >= 3 should return MoodStateExcited",
			lastEventTime: now,
			successStreak: 3,
			lastReason:    ReasonSuccess,
			want:          MoodStateExcited,
		},
		{
			name:          "SuccessStreak < 3 should return MoodStateHappy",
			lastEventTime: now,
			successStreak: 2,
			lastReason:    ReasonSuccess,
			want:          MoodStateHappy,
		},
		{
			name:          "01:00 to 05:00 should return MoodStateSleepy",
			lastEventTime: time.Date(2025, 1, 1, 2, 0, 0, 0, time.Local),
			successStreak: 0,
			lastReason:    ReasonActiveEdit,
			want:          MoodStateSleepy,
		},
		{
			name:          "Standard time should return MoodStateHappy",
			lastEventTime: time.Date(2025, 1, 1, 14, 0, 0, 0, time.Local),
			successStreak: 0,
			lastReason:    ReasonActiveEdit,
			want:          MoodStateHappy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferMoodState(tt.lastEventTime, tt.successStreak, tt.lastReason)
			if got != tt.want {
				t.Errorf("InferMoodState() = %v, want %v", got, tt.want)
			}
		})
	}
}
