package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AICLIClient は ~/.bin/ai CLI を呼び出してLLM応答を生成する。
type AICLIClient struct {
	timeout time.Duration
}

// NewAICLIClient は AICLIClient を作成する。
func NewAICLIClient() *AICLIClient {
	return &AICLIClient{
		timeout: 10 * time.Second,
	}
}

func (c *AICLIClient) Generate(ctx context.Context, in OllamaInput) (string, string, error) {
	prompt, err := renderPrompt(in)
	if err != nil {
		return "", "", fmt.Errorf("render prompt: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", prompt, fmt.Errorf("get home dir: %w", err)
	}

	aiPath := filepath.Join(homeDir, ".bin", "ai")
	// 存在確認
	if _, err := os.Stat(aiPath); err != nil {
		return "", prompt, fmt.Errorf("ai cli not found at %s", aiPath)
	}

	cmd := exec.CommandContext(ctx, aiPath, "-p", prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// stderrの内容があればそれを含める
		if stderr.Len() > 0 {
			return "", prompt, fmt.Errorf("ai cli error: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
		}
		// 130エラーはコマンド実行エラーとして扱う
		return "", prompt, fmt.Errorf("ai cli error: %w", err)
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", prompt, fmt.Errorf("ai cli returned empty output")
	}

	return result, prompt, nil
}

func (c *AICLIClient) IsAvailable() bool {
	return false // Always skip legacy CLI in auto-router
}
