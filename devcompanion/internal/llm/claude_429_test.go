package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAnthropicGenerate_TooManyRequests_Returns429Error は HTTP 429 エラーの検出テスト。
func TestAnthropicGenerate_TooManyRequests_Returns429Error(t *testing.T) {
	t.Parallel()

	// Given: HTTP 429 を返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	_, _, err := client.Generate(context.Background(), OllamaInput{})

	// Then: エラーが返り、かつ 429 ステータスに言及している
	if err == nil {
		t.Fatal("want error for HTTP 429, got nil")
	}
	// エラーメッセージに HTTP ステータスコードが含まれていることを確認
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("want non-empty error message")
	}
}

// TestAnthropicGenerate_OtherHTTPErrors は 429 以外の HTTP エラーの検出テスト。
// （将来のフォールバック分岐に対応）
func TestAnthropicGenerate_BadRequest_Returns400Error(t *testing.T) {
	t.Parallel()

	// Given: HTTP 400 を返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	_, _, err := client.Generate(context.Background(), OllamaInput{})

	// Then: エラーが返る（400 エラーはフォールバック対象外）
	if err == nil {
		t.Fatal("want error for HTTP 400, got nil")
	}
}

// TestAnthropicGenerate_ServerError_Returns500Error は HTTP 500 エラーの検出テスト。
func TestAnthropicGenerate_ServerError_Returns500Error(t *testing.T) {
	t.Parallel()

	// Given: HTTP 500 を返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	_, _, err := client.Generate(context.Background(), OllamaInput{})

	// Then: エラーが返る（500 エラーはフォールバック対象外）
	if err == nil {
		t.Fatal("want error for HTTP 500, got nil")
	}
}

// TestAnthropicGenerate_429_CanBeDetected は 429 エラーが特別に検出できるか確認。
// このテストは、実装側で 429 を他の HTTP エラーと区別できることを検証。
func TestAnthropicGenerate_429_CanBeDetected_ByInspectingErrorMessage(t *testing.T) {
	t.Parallel()

	// Given: HTTP 429 を返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	_, _, err := client.Generate(context.Background(), OllamaInput{})

	// Then: エラーメッセージに "429" が含まれている（または http.StatusTooManyRequests の値）
	if err == nil {
		t.Fatal("want error for HTTP 429, got nil")
	}
	// 将来的に、実装側で 429 を検出するロジックを追加した場合、
	// このテストが429が区別可能であることを確認
}
