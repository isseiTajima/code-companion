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
	Question         string // ユーザーからの直接の質問、またはユーザーの回答テキスト
	IsAnswerReaction bool   // true: ユーザーが質問に回答した後のリアクション（ReasonQuestionAnswered）
	WorkMemory       string // 直近の作業メモリの要約
	TraitID          string // 学習用特性ID
	TraitLabel       string // 特性の説明ラベル（i18n から引く）
	CurrentStage     int    // 進化ステージ
	LastAnswer       string   // 前回の回答
	PastAnswers      []string // このトレイトへの過去回答履歴（LLMに重複質問を避けさせるため）
	PersonalityType  string // "genki", "cute", "cool"
	RelationshipMode string // "normal", "lover"
	LearnedTraits        map[string]float64 // 学習されたユーザーの特性 (後方互換)
	LearnedTraitLabels   map[string]string  // 学習済み特性のテキストラベル（回答内容）
	PersonalMemorySummary string            // ユーザーの会話から得た個人情報のサマリー（複数行）
	RandomSeed           int64              // 毎回異なる値を注入してプロンプトの一意性を保証
	IsAISession          bool               // AIエージェントが動いている（バイブコーディング中）
	NewsContext          string             // ニュース見出し（InitCuriosity用）
	NewsPreferences      string             // 好き/嫌いな見出しのサマリー（継続的学習用）
	WeatherContext       string             // 天気情報（InitWeather用）
	ExampleSpeeches      []string           // few-shot例文（性格・状況別に選定済み）
	VoiceAtoms           string             // キャラクター固有の声のパーツ（語り出し/気持ち/締め）
	ConversationHistory  []ConvTurn         // 直近の会話履歴（UserQuestion用）
	Dialect              string             // 方言指定: "" | "hakata" | "kyoto" | "kansai"
	Season               string             // 現在の季節（"春"/"夏"/"秋"/"冬" or "spring" etc）
}

// ConvTurn は会話の1ターンを表す。
type ConvTurn struct {
	Role string // "user" または "sakura"
	Text string
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
		"think":    false, // Qwen3.5+ の thinking モードを無効化（出力の肥大化・遅延防止）
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
			// 指数バックオフ的に少し待機
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
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
			
			// タイムアウトエラー（DeadlineExceeded）の場合はリトライしても無駄なことが多いため中断
			if ctx.Err() == context.DeadlineExceeded || strings.Contains(httpErr.Error(), "context deadline exceeded") {
				break
			}
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
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				break // 4xx クライアントエラー（404等）はリトライしない
			}
			continue // 5xx サーバーエラーはリトライの価値あり
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
		"think":  false, // Qwen3.5+ の thinking モードを無効化
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
		"think":  false, // Qwen3.5+ の thinking モードを無効化
		// format: JSON配列スキーマを指定し、grammar-based 生成でフォーマット違反を物理的に排除
		"format": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"type": "string"},
		},
		"options": map[string]interface{}{
			"temperature":    0.7, // 0.9→0.7: 小モデルはこのほうがルール遵守率が高い
			"repeat_penalty": 1.2,
			"top_p":          0.9,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal batch request: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
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

	// structured output (JSON配列) としてパース。失敗時はテキストパースにフォールバック。
	raw := stripCodeFences(strings.TrimSpace(result.Message.Content))
	var speeches []string
	if err := json.Unmarshal([]byte(raw), &speeches); err != nil {
		speeches = parseBatchResponse(raw, req.Language)
	} else {
		// JSON パース成功時も postProcess を適用
		var processed []string
		for _, s := range speeches {
			if p := postProcess(s, req.Language); p != "" {
				processed = append(processed, p)
			}
		}
		speeches = processed
	}
	return speeches, nil
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

	traitsHeader := i18n.T(lang, "batch.traits_header")
	traitsSection := ""
	if len(req.LearnedTraitLabels) > 0 {
		traitsSection = traitsHeader
		for id, answer := range req.LearnedTraitLabels {
			label := i18n.T(lang, "trait."+id)
			if label == "trait."+id {
				label = id
			}
			traitsSection += fmt.Sprintf("- %s: %s\n", label, answer)
		}
	} else if len(req.LearnedTraits) > 0 {
		// 後方互換: LearnedTraitLabels がない場合は float を使う
		traitsSection = traitsHeader
		for id, val := range req.LearnedTraits {
			label := i18n.T(lang, "trait."+id)
			if label == "trait."+id {
				label = id
			}
			traitsSection += fmt.Sprintf("- %s: %.1f\n", label, val)
		}
	}

	// 個人情報サマリーを traitsSection に追記
	if req.PersonalMemorySummary != "" {
		if traitsSection == "" {
			traitsSection = traitsHeader
		}
		traitsSection += req.PersonalMemorySummary + "\n"
	}

	// 作業時間コンテキストは注入しない
	// （PCつけっぱなし運用では session 時間が実際の作業時間と一致しないため）
	workTimeSection := ""

	tmpl := i18n.T(lang, "batch.template")
	// tmpl must handle: userName, count, count, userName, pd, md, cd, traitsSection, workTimeSection, avoidSection
	prompt := fmt.Sprintf(tmpl, userName, req.Count, req.Count, userName, pd, md, cd, traitsSection, workTimeSection, avoidSection)

	// 現在の季節をプロンプトに追加（PersonalMemoryの季節情報と混同させないため）
	if req.Season != "" {
		prompt += fmt.Sprintf("\n【現在の季節】今は%sです。季節に関する発言はこの季節に合わせること。", req.Season)
	}

	// SituationHint: 技術不要の観察情報（時間・コミット数）を自然語で注入
	// この情報を使うとより具体的なセリフが生成できる（使用は任意）
	if req.SituationHint != "" {
		prompt += fmt.Sprintf("\n【今の状況メモ（技術がわからなくても隣で見ていれば分かること）】%s", req.SituationHint)
	}

	// private カテゴリのみ: サクラのキャラクタープロフィールを注入して一貫した人格を保つ
	if req.Category == "private" {
		if profile := i18n.T(lang, "batch.sakura_profile"); profile != "" && profile != "batch.sakura_profile" {
			prompt += "\n" + profile
		}
	}

	// 方言指定があればプロンプトに追加
	if req.Dialect != "" && lang == "ja" {
		switch req.Dialect {
		case "hakata":
			prompt += "\n【方言指定】全セリフを博多弁で書くこと。語尾は「〜やけん」「〜とー」「〜ばい」「〜たい」「〜しとーと？」を使う。標準語語尾（〜です・〜ます）は禁止。"
		case "kyoto":
			prompt += "\n【方言指定】全セリフを京都弁で書くこと。「〜やわ」「〜えー」「〜はる」「〜どすえ」を使い、はんなりとした柔らかい口調で。"
		case "kansai":
			prompt += "\n【方言指定】全セリフを関西弁で書くこと。「〜やん」「〜やで」「〜ねん」「〜ちゃう？」「〜やろ」「めっちゃ」を自然に使うこと。"
		}
	}

	// few-shot 例文をプロンプト末尾に追加（Pool 補充にも品質ハーネスを効かせる）
	if exSection := buildBatchExampleSection(lang, req.Personality, req.Category); exSection != "" {
		prompt += exSection
	}

	// 末尾出力指示: 生成直前に最重要制約を再提示する（小モデルは最後に見た指示に最も引きずられる）
	if lang == "ja" {
		prompt += "\n\n出力（JSON文字列配列、1件20〜40字のセリフのみ）:"
	} else {
		prompt += "\n\nOutput (JSON string array, 20-40 char speech lines only):"
	}
	return prompt
}

// listPrefixRe は Unicode 数字（全角・ベンガル等を含む）から始まる番号付きリストの行頭を除去する。
var listPrefixRe = regexp.MustCompile(`^\p{N}+[\s\.．）\)、:：]+\s*`)

// stripCodeFences はモデルが出力する ```json ... ``` のコードフェンスを除去する。
func stripCodeFences(s string) string {
	fenceIdx := strings.Index(s, "```")
	if fenceIdx < 0 {
		return s
	}
	// ``` の直後から最初の改行までが開始フェンス行（"```json" 等）
	afterFence := s[fenceIdx:]
	newlineInFence := strings.Index(afterFence, "\n")
	if newlineInFence < 0 {
		return s
	}
	s = afterFence[newlineInFence+1:]
	// 閉じフェンス ``` を末尾から除去
	if end := strings.LastIndex(s, "```"); end >= 0 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
}

func parseBatchResponse(raw, lang string) []string {
	// まずJSON配列としてパース試行（モデルがJSON形式で返す場合）
	trimmedRaw := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmedRaw, "[") {
		var jsonArr []string
		if err := json.Unmarshal([]byte(trimmedRaw), &jsonArr); err == nil {
			var result []string
			for _, s := range jsonArr {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				if p := postProcess(s, lang); p != "" {
					result = append(result, p)
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}

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
		// JSON文字列リテラルのクリーニング: "text", → text
		line = cleanJSONStringLiteral(line)
		line = strings.TrimSpace(line)
		if line == "" || isJSONArtifact(line) {
			continue
		}
		// `, "` を含む行は複数セリフが連結されたもの → 分割して個別に処理
		var segments []string
		if strings.Contains(line, `", "`) || strings.Contains(line, `","`) {
			for _, seg := range splitJSONArray(line) {
				seg = strings.TrimSpace(seg)
				if seg != "" && !isJSONArtifact(seg) {
					segments = append(segments, seg)
				}
			}
		} else {
			segments = []string{line}
		}
		for _, seg := range segments {
			if processed := postProcess(seg, lang); processed != "" {
				result = append(result, processed)
			}
		}
	}
	return result
}

// splitJSONArray は `speech1", "speech2", "speech3` 形式の行を個々のセリフに分割する。
func splitJSONArray(s string) []string {
	// `", "` または `","` で分割し、各セグメントの前後の " を除去する
	parts := strings.Split(s, `", "`)
	if len(parts) == 1 {
		parts = strings.Split(s, `","`)
	}
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, `"`)
		p = strings.TrimSuffix(p, `"`)
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isJSONArtifact は parseBatchResponse 後のゴミ（括弧・数字・空記号のみ）を検出する。
func isJSONArtifact(s string) bool {
	for _, r := range []rune(s) {
		if r != '[' && r != ']' && r != '{' && r != '}' && r != '"' && r != ',' && r != ' ' && r != '	' {
			return false
		}
	}
	return true
}

// cleanJSONStringLiteral は行頭・末尾のJSON文字列フォーマット（"text", / "text"] 等）を除去する。
func cleanJSONStringLiteral(s string) string {
	s = strings.TrimPrefix(s, "[") // 先頭の [ を除去
	s = strings.TrimRight(s, ",]") // 末尾の , ] を除去
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	} else if len(s) >= 1 && s[0] == '"' {
		// 先頭の " だけある場合: JSON配列要素が途中で切れているケース
		// 例: "セリフ", "次の要素", null → 最初の閉じ " までを取る
		s = s[1:]
		if idx := strings.Index(s, `",`); idx >= 0 {
			s = s[:idx]
		} else if idx := strings.Index(s, `"`); idx >= 0 {
			s = s[:idx]
		}
	}
	return s
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
