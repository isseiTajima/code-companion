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
	Personality           string
	RelationshipMode      string
	Category              string
	Language              string
	UserName              string
	LearnedTraits         map[string]float64 // 学習されたユーザーの特性 (後方互換)
	LearnedTraitLabels    map[string]string  // 学習済み特性のテキストラベル（回答内容）
	PersonalMemorySummary string             // ユーザーとの会話から得た個人情報サマリー
	WorkingDuration       string             // 現在の作業継続時間ラベル（"", "short", "medium", "long"）
	Count                 int
	RecentLines           []string // avoid list: 直近の発言履歴
	DiscardedPatterns     []string // 動的Avoidリスト: 過去に破棄されたセリフ
}

// BatchClient は複数セリフをまとめて生成できるバックエンドのインターフェース。
type BatchClient interface {
	BatchGenerate(ctx context.Context, req BatchRequest) ([]string, error)
}

// routerLayer はルーティング優先度順のバックエンド1層を表す。
type routerLayer struct {
	name    string
	client  LLMClient
	timeout time.Duration
}

// LLMRouter は複数のLLMバックエンドを優先度順にルーティングする。
type LLMRouter struct {
	ollama LLMClient
	claude LLMClient
	gemini LLMClient
	aiCLI  LLMClient
}

// orderedLayers は優先度順のバックエンド一覧を返す。
// Route と BatchGenerate はこのリストをループするだけでよく、
// バックエンド追加・順序変更はここだけを修正すればよい。
func (r *LLMRouter) orderedLayers() []routerLayer {
	return []routerLayer{
		{"Ollama", r.ollama, ollamaRouterTimeout},
		{"Claude", r.claude, claudeRouterTimeout},
		{"Gemini", r.gemini, geminiRouterTimeout},
		{"Gemini-CLI", r.aiCLI, aicliRouterTimeout},
	}
}

// Route はプロンプトをLLMバックエンドにルーティングし、(応答テキスト, 使用したレイヤー名, 使用プロンプト, エラー) を返す。
func (r *LLMRouter) Route(ctx context.Context, input OllamaInput) (string, string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", "", err
	}
	for _, layer := range r.orderedLayers() {
		if result, prompt, ok := r.try(ctx, layer.client, layer.timeout, input, layer.name); ok {
			return result, layer.name, prompt, nil
		}
	}
	// Fallback（プロンプトなし）
	return FallbackSpeech(Reason(input.Reason), input.Language), "Fallback", "", nil
}

// BatchGenerate はバッチセリフ生成を試みる。BatchClient を実装しているバックエンドを順に試す。
func (r *LLMRouter) BatchGenerate(ctx context.Context, req BatchRequest) ([]string, error) {
	for _, layer := range r.orderedLayers() {
		if layer.client == nil || !layer.client.IsAvailable() {
			continue
		}
		bc, ok := layer.client.(BatchClient)
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
