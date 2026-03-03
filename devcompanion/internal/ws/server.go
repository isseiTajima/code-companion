package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

const WSPort = 34567

// Server はWebSocketサーバーを管理する。
type Server struct {
	clients  map[*websocket.Conn]struct{}
	mu       sync.RWMutex
	upgrader websocket.Upgrader
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

// Start はWebSocketサーバーをポートWSPortで起動する。
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	return http.ListenAndServe(fmt.Sprintf(":%d", WSPort), mux)
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

	// クライアント切断まで読み続ける
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.mu.Lock()
	delete(s.clients, conn)
	s.mu.Unlock()

	conn.Close()
}
