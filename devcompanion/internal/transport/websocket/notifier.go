package websocket

import (
	"encoding/json"
	"sakura-kodama/internal/types"
	"sakura-kodama/internal/ws"
)

// WebSocketNotifier は WebSocket を介して通知を行う。
type WebSocketNotifier struct {
	server *ws.Server
}

// NewWebSocketNotifier は新しい WebSocketNotifier を作成する。
func NewWebSocketNotifier(s *ws.Server) *WebSocketNotifier {
	return &WebSocketNotifier{server: s}
}

// Notify はイベントを WebSocket クライアントに配信する。
func (n *WebSocketNotifier) Notify(event types.Event) {
	if n.server == nil {
		return
	}
	
	// types.Event.Payload (map[string]interface{}) から ws.Event への変換
	// Engine 内で Payload は map[string]interface{} として構築されているため、
	// JSON 経由で構造体にマッピングします。
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	
	var wsEv ws.Event
	if err := json.Unmarshal(payloadBytes, &wsEv); err != nil {
		return
	}
	
	n.server.Broadcast(wsEv)
}
