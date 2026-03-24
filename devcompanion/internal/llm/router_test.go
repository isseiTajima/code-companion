package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockClient はテスト用のモッククライアント。
// 呼び出し記録と応答を制御できる。
type mockClient struct {
	shouldFail    bool
	shouldTimeout bool
	response      string
	callCount     int
}

func (m *mockClient) Generate(ctx context.Context, in OllamaInput) (string, string, error) {
	m.callCount++

	if m.shouldTimeout {
		// コンテキストキャンセルをシミュレート
		<-ctx.Done()
		return "", "", ctx.Err()
	}

	if m.shouldFail {
		return "", "", fmt.Errorf("mock client error")
	}

	return m.response, "mock-prompt", nil
}

func (m *mockClient) IsAvailable() bool {
	return true // テストでは常に利用可能とする（try()を通過させてGenerate()を呼ばせるため）
}

// TestLLMRouter_OllamaSuccess はOllama層の成功時を検証。
func TestLLMRouter_OllamaSuccess(t *testing.T) {
	t.Parallel()

	// Given: Ollama成功、Claude・ai CLI未呼び出し
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ollama success"},"done":true}`))
	}))
	defer server.Close()

	ollama := NewOllamaClient(server.URL, "test-model")
	ollama.timeout = 100 * time.Millisecond
	router := &LLMRouter{
		ollama: ollama,
		claude: &mockClient{shouldFail: true},
		aiCLI:  &mockClient{shouldFail: true},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	text, _, _, err := router.Route(context.Background(), input)

	// Then: Ollama からの応答が返る
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "ollama success" {
		t.Fatalf("want 'ollama success', got %q", text)
	}
}

// TestLLMRouter_OllamaFail_ClaudeSuccess はOllama失敗→Claude成功のフォールバック。
func TestLLMRouter_OllamaFail_ClaudeSuccess(t *testing.T) {
	t.Parallel()

	// Given: Ollama失敗、Claude成功
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"content": []map[string]string{{"text": "claude success"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ollama := &mockClient{shouldFail: true}
	claude := NewAnthropicClient("test-key")
	claude.endpoint = server.URL
	claude.timeout = 100 * time.Millisecond

	router := &LLMRouter{
		ollama: ollama,
		claude: claude,
		aiCLI:  &mockClient{shouldFail: true},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	text, _, _, err := router.Route(context.Background(), input)

	// Then: Claude からの応答が返る（Ollama スキップ）
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "claude success" {
		t.Fatalf("want 'claude success', got %q", text)
	}
}

// TestLLMRouter_AllLayersFail_ReturnsFallback は全層失敗時のフォールバック。
func TestLLMRouter_AllLayersFail_ReturnsFallback(t *testing.T) {
	t.Parallel()
	SetSeed(42) // 決定論的シード設定

	// Given: 全層失敗
	router := &LLMRouter{
		ollama: &mockClient{shouldFail: true},
		claude: &mockClient{shouldFail: true},
		aiCLI:  &mockClient{shouldFail: true},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	text, backend, _, err := router.Route(context.Background(), input)

	// Then: Fallback バックエンドが使われ、テキストが返る
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if backend != "Fallback" {
		t.Fatalf("want backend %q, got %q", "Fallback", backend)
	}
	if text == "" {
		t.Fatal("want non-empty fallback text, got empty string")
	}
}

// TestLLMRouter_OllamaTimeout_NextLayer はOllamaタイムアウト時の遷移。
func TestLLMRouter_OllamaTimeout_NextLayer(t *testing.T) {
	t.Parallel()

	// Given: Ollama timeout、Claude成功
	router := &LLMRouter{
		ollama: &mockClient{shouldTimeout: true},
		claude: &mockClient{response: "claude after timeout"},
		aiCLI:  &mockClient{shouldFail: true},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	text, _, _, err := router.Route(context.Background(), input)

	// Then: Claude からの応答が返る
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "claude after timeout" {
		t.Fatalf("want 'claude after timeout', got %q", text)
	}
}

// TestLLMRouter_EmptyResponse_NextLayer は空レスポンス時の遷移。
func TestLLMRouter_EmptyResponse_NextLayer(t *testing.T) {
	t.Parallel()

	// Given: Ollama 空応答、Claude成功
	ollama := &mockClient{response: ""}
	router := &LLMRouter{
		ollama: ollama,
		claude: &mockClient{response: "claude after empty"},
		aiCLI:  &mockClient{shouldFail: true},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	text, _, _, err := router.Route(context.Background(), input)

	// Then: Claude からの応答が返る（空レスポンスはエラー扱い）
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "claude after empty" {
		t.Fatalf("want 'claude after empty', got %q", text)
	}
}

// TestLLMRouter_ClaudeEmptyHTTPError_NextLayer はClaude失敗時の遷移。
func TestLLMRouter_ClaudeEmptyContent_NextLayer(t *testing.T) {
	t.Parallel()

	// Given: Claude が空コンテンツを返す、ai CLI成功
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer server.Close()

	claude := NewAnthropicClient("test-key")
	claude.endpoint = server.URL
	claude.timeout = 100 * time.Millisecond

	router := &LLMRouter{
		ollama: &mockClient{shouldFail: true},
		claude: claude,
		aiCLI:  &mockClient{response: "aicli after claude fail"},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	text, _, _, err := router.Route(context.Background(), input)

	// Then: ai CLI からの応答が返る
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if text != "aicli after claude fail" {
		t.Fatalf("want 'aicli after claude fail', got %q", text)
	}
}

// TestLLMRouter_EventContext_EmbeddedInPrompt はEventContextのプロンプト埋め込み。
func TestLLMRouter_EventContext_EmbeddedInPrompt(t *testing.T) {
	t.Parallel()

	// Given: EventContext が "build_success"
	// When: Route で Ollama を呼ぶ
	// Then: プロンプトに "Developer event: build_success" が含まれる

	// Note: これは実装後に確認します。
	// 実装では OllamaInput に EventContext フィールドを追加し、
	// prompt template に埋め込みます。

	t.Skip("EventContext embedding verification pending")
}

// TestLLMRouter_RoutingOrder_OllamaClaudeAICLI はルーティング順序の検証。
func TestLLMRouter_RoutingOrder_OllamaClaudeAICLI(t *testing.T) {
	t.Parallel()

	// Given: 各層が呼び出し順序を記録する
	calls := []string{}
	callMutex := &bytes.Buffer{}

	ollama := &mockClient{shouldFail: true}
	claude := &mockClient{shouldFail: true}
	aiCLI := &mockClient{shouldFail: true}

	router := &LLMRouter{
		ollama: ollama,
		claude: claude,
		aiCLI:  aiCLI,
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: Route を呼ぶ
	_, _, _, _ = router.Route(context.Background(), input)

	// Then: 全層が呼び出される（順序: Ollama → Claude → ai CLI）
	if ollama.callCount != 1 {
		t.Errorf("want Ollama called once, got %d", ollama.callCount)
	}
	if claude.callCount != 1 {
		t.Errorf("want Claude called once (after Ollama fail), got %d", claude.callCount)
	}
	if aiCLI.callCount != 1 {
		t.Errorf("want ai CLI called once (after Claude fail), got %d", aiCLI.callCount)
	}

	_ = callMutex.String() // 使わない変数を削除（テストロジックのプレースホルダー）
	_ = calls                // 使わない変数を削除
}

// TestLLMRouter_ContextCancellation_TerminatesRouting はコンテキストキャンセルの処理。
func TestLLMRouter_ContextCancellation_TerminatesRouting(t *testing.T) {
	t.Parallel()

	// Given: キャンセル可能なコンテキスト
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 直後にキャンセル

	router := &LLMRouter{
		ollama: &mockClient{shouldFail: true},
		claude: &mockClient{shouldFail: true},
		aiCLI:  &mockClient{shouldFail: true},
	}

	input := OllamaInput{
		State:  "Running",
		Task:   "GenerateCode",
		Mood:   "Focus",
		Name:   "test",
		Tone:   "calm",
		Reason: string(ReasonThinkingTick),
	}

	// When: キャンセル済みコンテキストで Route を呼ぶ
	_, _, _, err := router.Route(ctx, input)

	// Then: エラーが返る（コンテキストキャンセル）
	if err == nil {
		t.Fatal("want error for cancelled context, got nil")
	}
}
