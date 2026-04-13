package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiClient は Google AI Studio API を呼び出す。
type GeminiClient struct {
	apiKey  string
	model   string
	timeout time.Duration
}

// NewGeminiClient は GeminiClient を作成する。
func NewGeminiClient(apiKey string) *GeminiClient {
	return &GeminiClient{
		apiKey:  strings.TrimSpace(apiKey),
		model:   "models/gemini-flash-latest", // 最新の安定板エイリアスに変更
		timeout: 20 * time.Second,
	}
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (c *GeminiClient) Generate(ctx context.Context, in OllamaInput) (string, string, error) {
	if c.apiKey == "" {
		return "", "", fmt.Errorf("gemini api key is empty")
	}

	prompt, err := renderPrompt(in)
	if err != nil {
		return "", "", err
	}

	// v1beta を使用し、モデル名に models/ が含まれていることを前提とする
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s:generateContent?key=%s", c.model, c.apiKey)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", prompt, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", prompt, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", prompt, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", prompt, err
	}

	if resp.StatusCode != http.StatusOK {
		msg := string(body)
		// エラー内容に含まれるキーをマスク
		if c.apiKey != "" {
			msg = strings.ReplaceAll(msg, c.apiKey, "REDACTED")
		}
		return "", prompt, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, msg)
	}

	var res geminiResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", prompt, err
	}

	if len(res.Candidates) > 0 && len(res.Candidates[0].Content.Parts) > 0 {
		return res.Candidates[0].Content.Parts[0].Text, prompt, nil
	}

	return "", prompt, fmt.Errorf("gemini returned empty candidates")
}

func (c *GeminiClient) IsAvailable() bool {
	return c.apiKey != ""
}
