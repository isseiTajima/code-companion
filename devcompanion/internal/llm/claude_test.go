package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// anthropicResponse はモックサーバーが返す Anthropic API レスポンス形式。
func anthropicOKResponse(text string) []byte {
	resp := map[string]interface{}{
		"content": []map[string]string{{"text": text}},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestAnthropicGenerate_Success(t *testing.T) {
	t.Parallel()

	// Given: content[0].text を返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(anthropicOKResponse("よし！"))
	}))
	defer server.Close()

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	text, _, err := client.Generate(context.Background(), OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "テスト",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	})

	// Then: trimmed テキストが返る
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "よし！" {
		t.Fatalf("want %q, got %q", "よし！", text)
	}
}

func TestAnthropicGenerate_SetsRequiredHeaders(t *testing.T) {
	t.Parallel()

	// Given: リクエストヘッダーを検証するモックサーバー
	var gotAPIKey, gotVersion, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("content-type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(anthropicOKResponse("ok"))
	}))
	defer server.Close()

	client := NewAnthropicClient("my-secret-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	if _, _, err := client.Generate(context.Background(), OllamaInput{}); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	// Then: 必須ヘッダーが設定されている
	if gotAPIKey != "my-secret-key" {
		t.Errorf("x-api-key: want %q, got %q", "my-secret-key", gotAPIKey)
	}
	if gotVersion != anthropicAPIVersion {
		t.Errorf("anthropic-version: want %q, got %q", anthropicAPIVersion, gotVersion)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type: want %q, got %q", "application/json", gotContentType)
	}
}

func TestAnthropicGenerate_HTTPError(t *testing.T) {
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

	// Then: エラーが返る
	if err == nil {
		t.Fatal("want error for HTTP 500, got nil")
	}
}

func TestAnthropicGenerate_EmptyContent(t *testing.T) {
	t.Parallel()

	// Given: 空の content 配列を返すモックサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 100 * time.Millisecond

	// When: Generate を呼ぶ
	_, _, err := client.Generate(context.Background(), OllamaInput{})

	// Then: エラーが返る（コンテンツが空）
	if err == nil {
		t.Fatal("want error for empty content array, got nil")
	}
}

func TestAnthropicGenerate_Timeout(t *testing.T) {
	t.Parallel()

	// done チャネルでハンドラーを外部から中断できるようにする。
	// Go 1.26 では context キャンセル時に TCP 接続が即座に閉じられず
	// r.Context().Done() だけでは server.Close() がブロックするため。
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer server.Close() // LIFO: 後に実行（close(done) の後）
	defer close(done)    // LIFO: 先に実行してハンドラーをアンブロック

	client := NewAnthropicClient("test-key")
	client.endpoint = server.URL
	client.timeout = 50 * time.Millisecond

	// When: Generate を呼ぶ（タイムアウト発生）
	_, _, err := client.Generate(context.Background(), OllamaInput{})

	// Then: タイムアウトエラーが返る
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
}

func TestNewAnthropicClient_EnvFallback(t *testing.T) {
	// Given: ANTHROPIC_API_KEY 環境変数が設定されている（apiKey 引数は空）
	t.Setenv("ANTHROPIC_API_KEY", "env-api-key")

	// When: apiKey="" でクライアントを作成
	client := NewAnthropicClient("")

	// Then: 環境変数のキーが使われている
	if client.apiKey != "env-api-key" {
		t.Errorf("want apiKey=%q from env, got %q", "env-api-key", client.apiKey)
	}
}

func TestNewAnthropicClient_ExplicitKeyTakesPrecedence(t *testing.T) {
	// Given: ANTHROPIC_API_KEY 環境変数と明示的な apiKey 両方が設定されている
	t.Setenv("ANTHROPIC_API_KEY", "env-api-key")

	// When: 明示的な apiKey でクライアントを作成
	client := NewAnthropicClient("explicit-key")

	// Then: 明示的なキーが優先される
	if client.apiKey != "explicit-key" {
		t.Errorf("want apiKey=%q (explicit), got %q", "explicit-key", client.apiKey)
	}
}

func TestNewAnthropicClient_NoEnvNoKey_EmptyKey(t *testing.T) {
	// Given: 環境変数なし・apiKey 引数も空
	t.Setenv("ANTHROPIC_API_KEY", "")

	// When: 空 apiKey でクライアントを作成
	client := NewAnthropicClient("")

	// Then: クライアント自体は作成される（apiKey は空）
	if client == nil {
		t.Fatal("want non-nil client even with empty key")
	}
	if client.apiKey != "" {
		t.Errorf("want empty apiKey when no key given, got %q", client.apiKey)
	}
}
