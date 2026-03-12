package llm

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	ollamaRouterTimeout = 30 * time.Second
	claudeRouterTimeout = 10 * time.Second
	geminiRouterTimeout = 20 * time.Second
	aicliRouterTimeout  = 10 * time.Second
)

type LLMClient interface {
	// Generate returns (response, prompt, error)
	Generate(ctx context.Context, in OllamaInput) (string, string, error)
	IsAvailable() bool
}

// BatchRequest はバッチセリフ生成のパラメータをまとめた型。
type BatchRequest struct {
	Personality       string
	RelationshipMode  string
	Category          string
	Language          string
	UserName          string
	LearnedTraits     map[string]float64 // 学習されたユーザーの特性
	Count             int
	RecentLines       []string // avoid list: 直近の発言履歴
	DiscardedPatterns []string // 動的Avoidリスト: 過去に破棄されたセリフ
}

// BatchClient は複数セリフをまとめて生成できるバックエンドのインターフェース。
type BatchClient interface {
	BatchGenerate(ctx context.Context, req BatchRequest) ([]string, error)
}

// LLMRouter は複数のLLMバックエンドを優先度順にルーティングする。
type LLMRouter struct {
	ollama LLMClient
	claude LLMClient
	gemini LLMClient
	aiCLI  LLMClient
}

// Route はプロンプトをLLMバックエンドにルーティングし、(応答テキスト, 使用したレイヤー名, 使用プロンプト, エラー) を返す。
func (r *LLMRouter) Route(ctx context.Context, input OllamaInput) (string, string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", "", err
	}

	// Layer 1: Ollama
	if result, prompt, ok := r.try(ctx, r.ollama, ollamaRouterTimeout, input, "Ollama"); ok {
		return result, "Ollama", prompt, nil
	}

	// Layer 2: Claude
	if result, prompt, ok := r.try(ctx, r.claude, claudeRouterTimeout, input, "Claude"); ok {
		return result, "Claude", prompt, nil
	}

	// Layer 3: Gemini (API)
	if result, prompt, ok := r.try(ctx, r.gemini, geminiRouterTimeout, input, "Gemini"); ok {
		return result, "Gemini", prompt, nil
	}

	// Layer 4: ai CLI (Legacy)
	if result, prompt, ok := r.try(ctx, r.aiCLI, aicliRouterTimeout, input, "Gemini-CLI"); ok {
		return result, "Gemini-CLI", prompt, nil
	}

	// Layer 5: Fallback (Fallback時はプロンプトなし)
	return FallbackSpeech(Reason(input.Reason), input.Language), "Fallback", "", nil
}

// BatchGenerate はバッチセリフ生成を試みる。BatchClient を実装しているバックエンドを順に試す。
func (r *LLMRouter) BatchGenerate(ctx context.Context, req BatchRequest) ([]string, error) {
	for _, client := range []LLMClient{r.ollama, r.claude, r.gemini, r.aiCLI} {
		if client == nil || !client.IsAvailable() {
			continue
		}
		bc, ok := client.(BatchClient)
		if !ok {
			continue
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		speeches, err := bc.BatchGenerate(timeoutCtx, req)
		cancel()
		if err == nil && len(speeches) > 0 {
			return speeches, nil
		}
		if err != nil {
			log.Printf("[POOL] BatchGenerate failed on backend: %v", err)
		}
	}
	return nil, fmt.Errorf("no batch-capable backend available")
}

func (r *LLMRouter) try(ctx context.Context, client LLMClient, timeout time.Duration, input OllamaInput, name string) (string, string, bool) {
	if client == nil || !client.IsAvailable() {
		return "", "", false
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	result, prompt, err := client.Generate(timeoutCtx, input)
	if err != nil {
		log.Printf("[DEBUG] %s error: %v", name, err)
		return "", "", false
	}
	return result, prompt, result != ""
}
