package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaGenerate_RetriesOnceThenSucceeds(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":{"content":""},"done":false}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"  success  "},"done":true}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "gemma")
	client.timeout = 100 * time.Millisecond

	text, _, err := client.Generate(context.Background(), OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "テスト",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	})
	if err != nil {
		t.Fatalf("want no error after retry, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("want exactly 2 attempts (1 retry), got %d", attempts)
	}
	if text != "success" {
		t.Fatalf("want trimmed response %q, got %q", "success", text)
	}
}

func TestOllamaGenerate_AllRetriesFailReturnError(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":{"content":""},"done":false}`))
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL, "gemma")
	client.timeout = 50 * time.Millisecond

	if _, _, err := client.Generate(context.Background(), OllamaInput{}); err == nil {
		t.Fatal("want error after retry exhaustion, got nil")
	}
	if attempts != retryAttempts {
		t.Fatalf("want %d attempts, got %d", retryAttempts, attempts)
	}
}

func TestNewOllamaClient_DefaultEndpoint(t *testing.T) {
	t.Parallel()

	client := NewOllamaClient("", "gemma")
	if client.endpoint != defaultOllamaEndpoint {
		t.Fatalf("want default endpoint %q, got %q", defaultOllamaEndpoint, client.endpoint)
	}
}
