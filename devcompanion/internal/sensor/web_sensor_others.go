//go:build !darwin
package sensor

import (
	"context"
	"time"

	"sakura-kodama/internal/types"
)

type WebSensor struct{}

func NewWebSensor(interval time.Duration) *WebSensor {
	return &WebSensor{}
}

func (s *WebSensor) Name() string {
	return "WebSensor"
}

func (s *WebSensor) Run(ctx context.Context, signals chan<- types.Signal) error {
	return nil
}
