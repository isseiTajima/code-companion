package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"
)

const (
	ollamaEndpoint = "http://localhost:11434/api/generate"
	ollamaTimeout  = 2 * time.Second
)

var promptTemplate = template.Must(template.New("prompt").Parse(
	`あなたは{{.Name}}という名前のデスクトップキャラクターです。口調は{{.Tone}}です。
現在の状態: {{.State}} ({{.Task}})
気持ち: {{.Mood}}
理由: {{.Reason}}
今の気持ちに合った短い一言（最大40文字）を自然な日本語で発言してください。`,
))

// OllamaInput はLLMへの入力パラメータ。
// コード本文・差分・ログ全文・ファイル名は含まない。
type OllamaInput struct {
	State  string
	Task   string
	Mood   string
	Name   string
	Tone   string
	Reason string
}

// OllamaClient はOllama APIのクライアント。
type OllamaClient struct {
	endpoint string
	model    string
	timeout  time.Duration
}

// NewOllamaClient は OllamaClient を作成する（タイムアウト固定2秒）。
func NewOllamaClient(model string) *OllamaClient {
	return &OllamaClient{
		endpoint: ollamaEndpoint,
		model:    model,
		timeout:  ollamaTimeout,
	}
}

// Generate はOllama APIへリクエストし、生成されたテキストを返す。
func (c *OllamaClient) Generate(ctx context.Context, in OllamaInput) (string, error) {
	prompt, err := renderPrompt(in)
	if err != nil {
		return "", fmt.Errorf("prompt render: %w", err)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":  c.model,
		"prompt": prompt,
		"stream": false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// 呼び出し元contextとは独立したタイムアウトを設定
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, c.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(result.Response), nil
}

func renderPrompt(in OllamaInput) (string, error) {
	var buf bytes.Buffer
	if err := promptTemplate.Execute(&buf, in); err != nil {
		return "", err
	}
	return buf.String(), nil
}
