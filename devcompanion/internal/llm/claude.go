package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const anthropicAPIVersion = "2023-06-01"

// AnthropicClient は Claude API へのリクエストを担当する。
type AnthropicClient struct {
	apiKey   string
	model    string
	endpoint string
	timeout  time.Duration
}

// NewAnthropicClient は AnthropicClient を作成する。
func NewAnthropicClient(apiKey string) *AnthropicClient {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &AnthropicClient{
		apiKey:   apiKey,
		model:    "claude-3-5-sonnet-20240620",
		endpoint: "https://api.anthropic.com/v1/messages",
		timeout:  10 * time.Second,
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *AnthropicClient) Generate(ctx context.Context, in OllamaInput) (string, string, error) {
	if c.apiKey == "" {
		return "", "", fmt.Errorf("anthropic api key is empty")
	}

	prompt, err := renderPrompt(in)
	if err != nil {
		return "", "", err
	}

	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 300,
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", prompt, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", prompt, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

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
		return "", prompt, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, string(body))
	}

	var res anthropicResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", prompt, err
	}

	if res.Error != nil {
		return "", prompt, fmt.Errorf("anthropic api error: %s", res.Error.Message)
	}

	if len(res.Content) > 0 {
		return res.Content[0].Text, prompt, nil
	}

	return "", prompt, fmt.Errorf("anthropic returned empty content")
}

func (c *AnthropicClient) IsAvailable() bool {
	return c.apiKey != ""
}
