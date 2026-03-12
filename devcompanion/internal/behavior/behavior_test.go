package behavior

import (
	"testing"
	"time"

	"sakura-kodama/internal/types"
)

func TestInferrer_Infer(t *testing.T) {
	inf := NewInferrer(5 * time.Minute)
	now := time.Now()
	nowStr := types.TimeToStr(now)
	nowStr1s := types.TimeToStr(now.Add(1 * time.Second))

	// 1. AI Pairing
	inf.AddSignal(types.Signal{Source: types.SourceAgent, Timestamp: nowStr})
	inf.AddSignal(types.Signal{Source: types.SourceFS, Timestamp: nowStr1s})

	b := inf.Infer()
	if b.Type != types.BehaviorAIPairing {
		t.Errorf("expected BehaviorAIPairing, got %v", b.Type)
	}

	// 2. Debugging
	inf = NewInferrer(5 * time.Minute)
	inf.AddSignal(types.Signal{Source: types.SourceFS, Message: "FAIL", Timestamp: nowStr})

	b = inf.Infer()
	if b.Type != types.BehaviorDebugging {
		t.Errorf("expected BehaviorDebugging, got %v", b.Type)
	}

	// 3. Coding
	inf = NewInferrer(5 * time.Minute)
	inf.AddSignal(types.Signal{Source: types.SourceFS, Timestamp: nowStr})
	
	b = inf.Infer()
	if b.Type != types.BehaviorCoding {
		t.Errorf("expected BehaviorCoding, got %v", b.Type)
	}
}
