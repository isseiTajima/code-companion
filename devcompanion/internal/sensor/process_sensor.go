package sensor

import (
	"context"
	"log"
	"strings"
	"time"

	"sakura-kodama/internal/types"
	"github.com/shirou/gopsutil/v3/process"
)

type ProcessSensor struct {
	processes []string
	interval  time.Duration
}

func NewProcessSensor(processes []string, interval time.Duration) *ProcessSensor {
	return &ProcessSensor{
		processes: processes,
		interval:  interval,
	}
}

func (s *ProcessSensor) Name() string {
	return "ProcessSensor"
}

func (s *ProcessSensor) Run(ctx context.Context, signals chan<- types.Signal) error {
	log.Printf("[SENSOR] Starting ProcessSensor with interval %v on processes: %v", s.interval, s.processes)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	runningStates := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			procs, err := process.Processes()
			if err != nil {
				continue
			}

			currentRunning := make(map[string]bool)
			for _, p := range procs {
				name, err := p.Name()
				if err != nil {
					continue
				}
				name = strings.ToLower(name)
				for _, target := range s.processes {
					if strings.Contains(name, strings.ToLower(target)) {
						currentRunning[target] = true
					}
				}
			}

			for _, target := range s.processes {
				if currentRunning[target] && !runningStates[target] {
					signals <- types.Signal{
						Type:      types.SigProcessStarted,
						Source:    types.SourceProcess,
						Value:     target,
						Timestamp: types.TimeToStr(time.Now()),
					}
				} else if !currentRunning[target] && runningStates[target] {
					signals <- types.Signal{
						Type:      types.SigProcessStopped,
						Source:    types.SourceProcess,
						Value:     target,
						Timestamp: types.TimeToStr(time.Now()),
					}
				}
				runningStates[target] = currentRunning[target]
			}
		}
	}
}
