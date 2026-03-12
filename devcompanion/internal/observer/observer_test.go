package observer

import (
	"testing"
	"time"

	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/types"
)

func TestObserver_OnMonitorEvent(t *testing.T) {
	o, _ := NewDevObserver(".")
	o.UpdateFrequency(3) // お喋り

	now := time.Now()

	// 1. Idle 検知テスト
	o.OnMonitorEvent(monitor.MonitorEvent{State: types.StateIdle}, now)
	o.OnMonitorEvent(monitor.MonitorEvent{State: types.StateIdle}, now.Add(2*time.Minute))

	select {
	case obs := <-o.Observations():
		if obs.Type != ObsIdleStart {
			t.Errorf("expected ObsIdleStart, got %v", obs.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected observation, got none")
	}

	// 2. Active Editing 検知テスト（activeEditThreshold=10に合わせて送信）
	for i := 0; i < 10; i++ {
		o.OnMonitorEvent(monitor.MonitorEvent{State: types.StateCoding}, now.Add(time.Duration(i)*time.Second))
	}

	select {
	case obs := <-o.Observations():
		if obs.Type != ObsActiveEditing {
			t.Errorf("expected ObsActiveEditing, got %v", obs.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected observation, got none")
	}
}
