package monitor

import (
	"testing"

	"sakura-kodama/internal/types"
)

func TestDetector_recordSignal(t *testing.T) {
	d := &Detector{
		signals:    make(chan types.Signal, 16),
		pathLayers: make(map[string]types.SignalSource),
	}

	d.recordSignal("test.go")

	select {
	case sig := <-d.signals:
		if sig.Type != types.SigFileModified {
			t.Errorf("expected SigFileModified, got %v", sig.Type)
		}
		if sig.Message != "generate" {
			t.Errorf("expected generate, got %v", sig.Message)
		}
	default:
		t.Error("expected signal, got none")
	}
}
