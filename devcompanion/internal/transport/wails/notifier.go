package wails

import (
	"context"
	"sakura-kodama/internal/types"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WailsNotifier は Wails のイベントシステムを使用して通知を行う。
type WailsNotifier struct {
	ctx context.Context
}

// NewWailsNotifier は新しい WailsNotifier を作成する。
func NewWailsNotifier(ctx context.Context) *WailsNotifier {
	return &WailsNotifier{ctx: ctx}
}

// Notify はイベントをフロントエンドに送信する。
func (n *WailsNotifier) Notify(event types.Event) {
	if n.ctx == nil {
		return
	}
	runtime.EventsEmit(n.ctx, event.Type, event.Payload)
}
