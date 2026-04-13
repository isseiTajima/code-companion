package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// OllamaManager handles model operations like pulling, creating, and deleting models.
type OllamaManager struct {
	endpoint string
}

func NewOllamaManager(endpoint string) *OllamaManager {
	return &OllamaManager{endpoint: endpoint}
}

func (m *OllamaManager) UpdateEndpoint(endpoint string) {
	m.endpoint = endpoint
}

func (m *OllamaManager) baseEndpoint() string {
	base := strings.TrimRight(strings.Split(m.endpoint, "/api/")[0], "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	return base
}

// PullModel pulls a model from the Ollama library.
func (m *OllamaManager) PullModel(modelName string, onProgress func(map[string]interface{})) error {
	pullURL := m.baseEndpoint() + "/api/pull"
	body, _ := json.Marshal(map[string]interface{}{
		"name":   modelName,
		"stream": true,
	})
	resp, err := http.Post(pullURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line map[string]interface{}
		if json.Unmarshal(scanner.Bytes(), &line) == nil {
			if onProgress != nil {
				onProgress(line)
			}
		}
	}
	return nil
}

// CreateSakuraModel creates a derived model with character definition.
func (m *OllamaManager) CreateSakuraModel(baseModel string) (string, error) {
	sakuraName := sakuraModelName(baseModel)
	systemPrompt := `あなたはAIコンパニオンの「さくら」です。技術のことはよくわからないけれど、ユーザーのそばで応援したいと思っている、親しみやすいキャラクターです。人間味のある言葉をかけてください。
以下のルールを必ず守ってください：
- 一人称は必ず「私」を使います（僕、サクラは禁止）。
- 「ずっといますよ」「みています」などの無意味な存在アピールや語尾を絶対に付けないでください。
- 自然な日本語の話し言葉で答える（120文字程度まで、ニュースは180文字まで）。
- ユーザーが過去に話したことや、現在の作業の意図を汲み取った人間味のある反応をする。
- 「頑張ってください」などの無難な定型文は避け、具体的な気づきを話す。
- 書き言葉・翻訳調・詩的な比喩は使わない。
- %・*・#などの記号は使わない。
- サービス業のような敬語（「お手伝いします」等）は禁止。`
	modelfile := fmt.Sprintf("FROM %s\nSYSTEM \"\"\"%s\"\"\"\n", baseModel, systemPrompt)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      sakuraName,
		"modelfile": modelfile,
		"stream":    true,
	})
	resp, err := http.Post(m.baseEndpoint()+"/api/create", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var line map[string]interface{}
		if json.Unmarshal(scanner.Bytes(), &line) == nil {
			if errMsg, ok := line["error"].(string); ok {
				return "", fmt.Errorf("create model error: %s", errMsg)
			}
		}
	}
	return sakuraName, nil
}

// DeleteModel deletes a model from Ollama.
func (m *OllamaManager) DeleteModel(modelName string) error {
	body, _ := json.Marshal(map[string]string{"name": modelName})
	req, err := http.NewRequest(http.MethodDelete, m.baseEndpoint()+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama delete failed: %s", resp.Status)
	}
	return nil
}

// ListModels returns a list of installed models.
func (m *OllamaManager) ListModels() ([]string, error) {
	resp, err := http.Get(m.baseEndpoint() + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

func sakuraModelName(baseModel string) string {
	name := baseModel
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return "sakura-" + name
}
