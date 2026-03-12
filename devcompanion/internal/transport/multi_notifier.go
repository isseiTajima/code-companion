package transport

import (
	"sakura-kodama/internal/engine"
	"sakura-kodama/internal/types"
)

// MultiNotifier は複数の Notifier にイベントを転送する。
type MultiNotifier struct {
	notifiers []engine.Notifier
}

// NewMultiNotifier は新しい MultiNotifier を作成する。
func NewMultiNotifier(ns ...engine.Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: ns}
}

// Notify は登録されたすべての Notifier にイベントを送信する。
func (m *MultiNotifier) Notify(event types.Event) {
	for _, n := range m.notifiers {
		if n != nil {
			n.Notify(event)
		}
	}
}
