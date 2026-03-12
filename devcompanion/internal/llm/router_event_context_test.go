package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRouter_EventContext_BuildSuccessEmbedding(t *testing.T) {
	t.Parallel()
	var receivedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 {
			receivedPrompt = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true}`))
	}))
	defer server.Close()

	ollama := NewOllamaClient(server.URL, "test")
	ollama.timeout = 100 * time.Millisecond
	router := &LLMRouter{ollama: ollama}

	input := OllamaInput{
		Language: "ja",
		Reason:   humanizeReason(ReasonSuccess, "ja"),
		Behavior: humanizeBehavior("active_edit", "ja"),
	}
	_, _, _, _ = router.Route(context.Background(), input)

	if !strings.Contains(receivedPrompt, "アプリが動くところまで確認できた") {
		t.Errorf("prompt missing humanized reason: %s", receivedPrompt)
	}
}

func TestRouter_EventContext_EmptyEvent(t *testing.T) {
	t.Parallel()
	var receivedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 {
			receivedPrompt = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true}`))
	}))
	defer server.Close()

	ollama := NewOllamaClient(server.URL, "test")
	router := &LLMRouter{ollama: ollama}

	input := OllamaInput{
		Language: "ja",
		Event:    "",
	}
	_, _, _, _ = router.Route(context.Background(), input)

	if strings.Contains(receivedPrompt, "直近のイベント:") {
		t.Errorf("prompt should not contain event label when empty: %s", receivedPrompt)
	}
}
