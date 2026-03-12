package contextengine

import (
	"testing"
	"time"

	"sakura-kodama/internal/types"
)

func TestEstimator_ProcessSignal(t *testing.T) {
	est := NewEstimator()
	now := time.Now()
	nowStr := types.TimeToStr(now)

	// 1. AI Pairing
	info := est.ProcessSignal(types.Signal{Type: types.SigProcessStarted, Source: types.SourceAgent, Timestamp: nowStr})
	if info.State != types.StateAIPairing {
		// Scored 0.5, threshold is 0.6, so might not trigger yet
	}

	// Add more signals to exceed threshold
	info = est.ProcessSignal(types.Signal{Type: types.SigProcessStarted, Source: types.SourceAgent, Timestamp: nowStr})
	if info.State != types.StateAIPairing {
		t.Errorf("expected StateAIPairing, got %v (score: %f)", info.State, info.Confidence)
	}

	// 2. Coding
	est = NewEstimator()
	info = est.ProcessSignal(types.Signal{Type: types.SigGitCommit, Source: types.SourceGit, Timestamp: nowStr})
	info = est.ProcessSignal(types.Signal{Type: types.SigGitCommit, Source: types.SourceGit, Timestamp: nowStr})
	if info.State != types.StateCoding {
		t.Errorf("expected StateCoding, got %v", info.State)
	}
}
