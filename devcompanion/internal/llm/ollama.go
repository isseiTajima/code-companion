package llm

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"text/template"
	"time"

	"sakura-kodama/internal/i18n"
)

const (
	defaultOllamaEndpoint = "http://localhost:11434/api/generate"
	defaultOllamaChatEndpoint = "http://localhost:11434/api/chat"
	ollamaTimeout         = 30 * time.Second
	retryAttempts         = 3
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

var promptTemplates = make(map[string]*template.Template)

func init() {
	langs := []string{"ja", "en"}
	for _, lang := range langs {
		data, err := promptFS.ReadFile(fmt.Sprintf("prompts/%s.tmpl", lang))
		if err != nil {
			log.Printf("[WARN] Failed to load prompt template for %s: %v", lang, err)
			continue
		}
		tmpl, err := template.New(lang).Parse(string(data))
		if err != nil {
			log.Printf("[WARN] Failed to parse prompt template for %s: %v", lang, err)
			continue
		}
		promptTemplates[lang] = tmpl
	}

	// Load language-specific question templates
	for _, qlang := range []string{"ja", "en"} {
		qkey := "question_" + qlang
		data, err := promptFS.ReadFile(fmt.Sprintf("prompts/%s.tmpl", qkey))
		if err != nil {
			log.Printf("[WARN] Failed to load question template for %s: %v", qlang, err)
			continue
		}
		tmpl, err := template.New(qkey).Parse(string(data))
		if err != nil {
			log.Printf("[WARN] Failed to parse question template for %s: %v", qlang, err)
			continue
		}
		promptTemplates[qkey] = tmpl
	}
}

// OllamaInput はLLMへの入力パラメータ。
type OllamaInput struct {
	State            string
	Task             string
	Behavior         string // 行動 (coding, debugging, etc)
	SessionMode      string // モード (deep_focus, struggling, etc)
	FocusLevel       float64
	Mood             string
	Name             string
	UserName         string
	Tone             string
	Reason           string
	Event            string
	Details          string
	RelationshipLvl  int
	Trust            int
	NightCoder       bool
	CommitFrequency  string
	BuildFailRate    string
	TimeOfDay        string
	Language         string // ja, en
	Question         string // ユーザーからの直接の質問
	WorkMemory       string // 直近の作業メモリの要約
	TraitID          string // 学習用特性ID
	TraitLabel       string // 特性の説明ラベル（i18n から引く）
	CurrentStage     int    // 進化ステージ
	LastAnswer       string // 前回の回答
	PersonalityType  string // "genki", "cute", "tsukime"
	RelationshipMode string // "normal", "lover"
	LearnedTraits    map[string]float64 // 学習されたユーザーの特性 (0.0 - 1.0)
	RandomSeed       int64  // 毎回異なる値を注入してプロンプトの一意性を保証
}

// OllamaClient はOllama APIのクライアント。
type OllamaClient struct {
	endpoint string
	model    string
	timeout  time.Duration
}

// NewOllamaClient は OllamaClient を作成する。
func NewOllamaClient(endpoint, model string) *OllamaClient {
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}
	return &OllamaClient{
		endpoint: endpoint,
		model:    model,
		timeout:  ollamaTimeout,
	}
}

// chatEndpoint は /api/generate エンドポイントから /api/chat エンドポイントを導出する。
func (c *OllamaClient) chatEndpoint() string {
	if strings.Contains(c.endpoint, "/api/generate") {
		return strings.Replace(c.endpoint, "/api/generate", "/api/chat", 1)
	}
	if strings.Contains(c.endpoint, "/api/") {
		return c.endpoint // already a specific API path
	}
	// bare base URL (e.g. from tests): append /api/chat
	return strings.TrimRight(c.endpoint, "/") + "/api/chat"
}

// Generate はOllama Chat APIへリクエストし、生成されたテキストと使用プロンプトを返す。
// /api/generate（text completion）ではなく /api/chat（instruction following）を使用することで
// モデルがコンテキストを「続ける」のではなく「応答する」と正しく解釈する。
func (c *OllamaClient) Generate(ctx context.Context, in OllamaInput) (string, string, error) {
	chatEP := c.chatEndpoint()
	log.Printf("[DEBUG] Ollama requesting model: '%s' at %s", c.model, chatEP)
	prompt, err := renderPrompt(in)
	if err != nil {
		return "", "", fmt.Errorf("prompt render: %w", err)
	}

	messages := []map[string]string{}
	temperature := 1.0
	if strings.HasPrefix(in.Language, "question") {
		var sysMsg string
		if strings.HasSuffix(in.Language, "_en") {
			sysMsg = "You are Sakura, a junior engineer companion. Output ONLY the JSON in the specified format, nothing else."
		} else {
			sysMsg = "あなたは開発者の後輩「サクラ」です。指定された形式のJSONのみを出力してください。"
		}
		messages = append(messages,
			map[string]string{"role": "system", "content": sysMsg},
			map[string]string{"role": "user", "content": prompt},
		)
		temperature = 0.7
	} else {
		messages = append(messages,
			map[string]string{"role": "user", "content": prompt},
		)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    c.model,
		"messages": messages,
		"stream":   false,
		"options": map[string]interface{}{
			"temperature":    temperature,
			"repeat_penalty": 1.3,
			"top_p":          0.9,
			"seed":           in.RandomSeed,
		},
	})
	if err != nil {
		return "", prompt, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(1 * time.Second)
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
		req, reqErr := http.NewRequestWithContext(timeoutCtx, http.MethodPost, chatEP, bytes.NewReader(reqBody))
		if reqErr != nil {
			cancel()
			lastErr = fmt.Errorf("create request: %w", reqErr)
			break
		}
		req.Header.Set("Content-Type", "application/json")

		resp, httpErr := http.DefaultClient.Do(req)
		if httpErr != nil {
			cancel()
			lastErr = fmt.Errorf("Ollama connection failed (Is Ollama running?): %w", httpErr)
			continue
		}

		var result struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		cancel()

		if decodeErr != nil {
			lastErr = fmt.Errorf("decode response: %w", decodeErr)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("ollama returned status %d", resp.StatusCode)
			continue
		}
		return cleanSpeechOutput(result.Message.Content), prompt, nil
	}

	return "", prompt, lastErr
}

// cleanSpeechOutput はモデルが末尾に付けるスコア記号などのゴミ文字を除去する。
func cleanSpeechOutput(s string) string {
	s = strings.TrimSpace(s)
	// "%X", "%!", "%'" のようなパターンを末尾から除去
	cleaned := regexp.MustCompile(`[\s%*#+\[\]]+$`).ReplaceAllString(s, "")
	return strings.TrimSpace(cleaned)
}

func (c *OllamaClient) IsAvailable() bool {
	return c.endpoint != ""
}

// GenerateRaw はプリビルドされたプロンプトでOllamaを呼び出す（評価・分類用途）。
func (c *OllamaClient) GenerateRaw(ctx context.Context, prompt string) (string, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1, // 低温度で安定したフォーマット出力
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatEndpoint(), bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return strings.TrimSpace(result.Message.Content), nil
}

// BatchGenerate は複数のセリフをまとめて生成する（BatchClient インターフェースを実装）。
func (c *OllamaClient) BatchGenerate(ctx context.Context, req BatchRequest) ([]string, error) {
	prompt := buildBatchPrompt(req)

	reqBody, err := json.Marshal(map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature":    0.9,
			"repeat_penalty": 1.2,
			"top_p":          0.9,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal batch request: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, c.chatEndpoint(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create batch request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("batch generate request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch generate returned status %d", resp.StatusCode)
	}

	return parseBatchResponse(result.Message.Content), nil
}

func buildBatchPrompt(req BatchRequest) string {
	lang := req.Language
	if lang == "" {
		lang = "ja"
	}

	pd := i18n.T(lang, "batch.personality."+req.Personality)
	if pd == "batch.personality."+req.Personality {
		pd = i18n.T(lang, "batch.personality.cute")
	}

	md := i18n.T(lang, "batch.mode."+req.RelationshipMode)
	if md == "batch.mode."+req.RelationshipMode {
		md = i18n.T(lang, "batch.mode.normal")
	}

	cd := i18n.T(lang, "batch.category."+req.Category)
	if cd == "batch.category."+req.Category {
		cd = i18n.T(lang, "batch.category.heartbeat")
	}

	avoidSection := ""
	if len(req.RecentLines) > 0 {
		header := i18n.T(lang, "batch.avoid_header")
		avoidSection += "\n" + header + "\n"
		for _, line := range req.RecentLines {
			avoidSection += "- " + line + "\n"
		}
	}
	// 動的Avoidリスト: 過去に破棄されたセリフパターンを追加
	if len(req.DiscardedPatterns) > 0 {
		discardedHeader := i18n.T(lang, "batch.discarded_header")
		avoidSection += "\n" + discardedHeader + "\n"
		for _, p := range req.DiscardedPatterns {
			avoidSection += "× " + p + "\n"
		}
	}

	userName := req.UserName
	if userName == "" {
		userName = "先輩"
	}

	traitsSection := ""
	if len(req.LearnedTraits) > 0 {
		if lang == "en" {
			traitsSection = "\n[Known about the user]\n"
		} else {
			traitsSection = "\n【先輩について分かっていること】\n"
		}
		for id, val := range req.LearnedTraits {
			label := i18n.T(lang, "trait."+id)
			if label == "trait."+id {
				label = id // i18n キーが未定義の場合はIDをそのまま使う
			}
			traitsSection += fmt.Sprintf("- %s: %.1f\n", label, val)
		}
	}

	tmpl := i18n.T(lang, "batch.template")
	// tmpl must handle: userName, count, count, userName, pd, md, cd, traitsSection, avoidSection
	return fmt.Sprintf(tmpl, userName, req.Count, req.Count, userName, pd, md, cd, traitsSection, avoidSection)
}

// listPrefixRe は Unicode 数字（全角・ベンガル等を含む）から始まる番号付きリストの行頭を除去する。
var listPrefixRe = regexp.MustCompile(`^\p{N}+[\s\.．）\)、:：]+\s*`)

func parseBatchResponse(raw string) []string {
	lines := strings.Split(raw, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Unicode数字で始まる番号付きリスト除去（全角・ベンガル数字等も対応）
		line = listPrefixRe.ReplaceAllString(line, "")
		// 記号行頭除去: "- ", "・ ", "* ", "• "
		for _, prefix := range []string{"- ", "・ ", "* ", "• "} {
			if strings.HasPrefix(line, prefix) {
				line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
				break
			}
		}
		line = strings.TrimSpace(line)
		processed := postProcess(line)
		if processed != "" {
			result = append(result, processed)
		}
	}
	return result
}

func renderPrompt(in OllamaInput) (string, error) {
	lang := in.Language
	if lang == "" {
		lang = "ja"
	}
	tmpl, ok := promptTemplates[lang]
	if !ok {
		tmpl = promptTemplates["ja"]
	}
	if tmpl == nil {
		return "", fmt.Errorf("no prompt template available for language: %s", lang)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, in); err != nil {
		return "", err
	}
	return buf.String(), nil
}
