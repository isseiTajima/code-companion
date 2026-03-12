package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const WSPort = 34567
const broadcastTimeout = 1 * time.Second

// Server はWebSocketサーバーを管理する。
type Server struct {
	clients        map[*websocket.Conn]struct{}
	mu             sync.RWMutex
	upgrader       websocket.Upgrader
	commandHandler func(e Event)
}

// NewServer は Server を初期化する。
func NewServer() *Server {
	return &Server{
		clients: make(map[*websocket.Conn]struct{}),
		upgrader: websocket.Upgrader{
			// Wails webviewのオリジン制限を回避
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// SetCommandHandler は外部からのコマンドを受け取るハンドラを設定する。
func (s *Server) SetCommandHandler(handler func(e Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commandHandler = handler
}

// Start はWebSocketサーバーをポートWSPortで起動する。
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	mux.HandleFunc("/trigger", s.handleTrigger) // HTTPエンドポイント追加
	
	addr := fmt.Sprintf("127.0.0.1:%d", WSPort)
	log.Printf("[WS] Server starting on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// handleTrigger はHTTP経由での強制発火を受け付ける。
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	traitID := r.URL.Query().Get("trait_id")
	
	s.mu.RLock()
	handler := s.commandHandler
	s.mu.RUnlock()

	if handler != nil {
		handler(Event{
			Type: "trigger_question",
			Payload: map[string]interface{}{
				"trait_id": traitID,
			},
		})
		fmt.Fprintf(w, "Triggered question for: %s\n", traitID)
	} else {
		http.Error(w, "Handler not initialized", http.StatusInternalServerError)
	}
}

// Broadcast はすべての接続クライアントにイベントを送信する。
// 送信失敗したクライアントは即座に切断・削除する。
func (s *Server) Broadcast(e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var failed []*websocket.Conn
	for conn := range s.clients {
		conn.SetWriteDeadline(time.Now().Add(broadcastTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			failed = append(failed, conn)
		}
	}

	for _, conn := range failed {
		conn.Close()
		delete(s.clients, conn)
	}
}

// handleWS はWebSocketアップグレードと接続管理を行う。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.clients[conn] = struct{}{}
	s.mu.Unlock()

	// クライアントからのメッセージを読み取って処理
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var ev Event
		if err := json.Unmarshal(message, &ev); err == nil {
			s.mu.RLock()
			handler := s.commandHandler
			s.mu.RUnlock()
			if handler != nil {
				handler(ev)
			}
		}
	}

	s.mu.Lock()
	delete(s.clients, conn)
	s.mu.Unlock()

	conn.Close()
}
