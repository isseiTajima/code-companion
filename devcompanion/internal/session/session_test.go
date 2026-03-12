package session

import (
	"testing"
	"time"

	"sakura-kodama/internal/types"
)

func TestTracker_Update(t *testing.T) {
	tr := NewTracker()
	now := time.Now()

	// 1. Productive Flow
	st := tr.Update(types.Behavior{Type: types.BehaviorCoding}, now)
	if st.Mode != types.ModeProductiveFlow {
		t.Errorf("expected ModeProductiveFlow, got %v", st.Mode)
	}

	// 2. Deep Focus (after repeated activity)
	for i := 0; i < 10; i++ {
		st = tr.Update(types.Behavior{Type: types.BehaviorCoding}, now.Add(time.Duration(i)*time.Minute))
	}
	if st.Mode != types.ModeDeepFocus {
		t.Errorf("expected ModeDeepFocus, got %v", st.Mode)
	}

	// 3. Struggling
	st = tr.Update(types.Behavior{Type: types.BehaviorDebugging}, now.Add(11*time.Minute))
	if st.Mode != types.ModeStruggling {
		t.Errorf("expected ModeStruggling, got %v", st.Mode)
	}
}
