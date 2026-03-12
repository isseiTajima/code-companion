package monitor

import (
	"log"
	"sakura-kodama/internal/types"
)

// SignalLogger は受信したシグナルを標準出力に記録する。
func SignalLogger(signals <-chan types.Signal) {
	for sig := range signals {
		log.Printf("[DEBUG-SIGNAL] Source: %s, Type: %s, Value: %s, Message: %s", 
			sig.Source, sig.Type, sig.Value, sig.Message)
	}
}

// ContextViewer は現在のコンテキスト状態を表示する。
func ContextViewer(events <-chan MonitorEvent) {
	for ev := range events {
		log.Printf("[DEBUG-CONTEXT] State: %s, Behavior: %s, Mode: %s, Confidence: %.2f",
			ev.State, ev.Behavior.Type, ev.Session.Mode, ev.Context.Confidence)
	}
}
