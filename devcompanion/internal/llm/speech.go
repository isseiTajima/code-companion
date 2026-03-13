package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/i18n"
	"sakura-kodama/internal/memory"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

// SpeechGenerator はLLMを使用してセリフを生成する。
type SpeechGenerator struct {
	mu                sync.RWMutex
	router            *LLMRouter
	cache             *SpeechCache
	state             *SpeechState
	freq              *FrequencyController
	pool              *SpeechPool
	currentMood       MoodState
	successStreak     int
	failStreak        int
	sameFileEditCount int
	usingFallback     bool
	lastSpeech        string // 前回の発言を保持
	lastDetails       string // 前回の作業対象を保持
	poolLanguage      string // プール補充時に使用する言語設定
}

// NewSpeechGenerator は SpeechGenerator を作成する。
func NewSpeechGenerator(cfg *config.Config) *SpeechGenerator {
	sg := &SpeechGenerator{
		router: &LLMRouter{
			ollama: NewOllamaClient(cfg.OllamaEndpoint, cfg.Model),
			claude: NewAnthropicClient(cfg.AnthropicAPIKey),
			gemini: NewGeminiClient(cfg.GeminiAPIKey),
			aiCLI:  NewAICLIClient(),
		},
		cache:        NewSpeechCache(),
		state:        NewSpeechState(),
		freq:         NewFrequencyController(),
		pool:         NewSpeechPool(),
		currentMood:  MoodStateHappy,
		poolLanguage: cfg.Language,
	}
	SetSeed(time.Now().UnixNano())

	// 起動時に頻出カテゴリのプールをプリウォーム（最初のイベントでfallbackにならないよう）
	// 注意: 起動直後は学習データがないため空のプロファイル
	go sg.prewarmPools(cfg.Language, cfg.UserName, profile.DevProfile{})

	return sg
}

// prewarmPools は起動時に頻出カテゴリのプールを並列で事前充填する。
func (sg *SpeechGenerator) prewarmPools(language, userName string, prof profile.DevProfile) {
	prewarm := []struct{ personality, category string }{
		{"cute", "heartbeat"},
		{"cute", "working"},
		{"cute", "greeting"},
		{"genki", "achievement"},
		{"tsukime", "struggle"},
	}
	var wg sync.WaitGroup
	for _, p := range prewarm {
		wg.Add(1)
		go func(personality, category string) {
			defer wg.Done()
			key := poolKey(personality, category, language)
			sg.triggerRefill(key, personality, "normal", category, language, userName, prof)
		}(p.personality, p.category)
	}
	wg.Wait()
	// log.Printf("[POOL] Prewarm complete for language=%s", language)
}

func (sg *SpeechGenerator) Generate(e monitor.MonitorEvent, cfg *config.Config, reason Reason, prof profile.DevProfile, question string) (string, string, string) {
	if !sg.mu.TryLock() {
		return "", "", ""
	}
	defer sg.mu.Unlock()

	if cfg.Mute {
		return "", "", ""
	}

	now := time.Now()
	if !sg.freq.ShouldSpeak(reason, e.State, cfg, now) {
		return "", "", ""
	}

	speech, prompt, backend := sg.generateTextLocked(e, cfg, reason, prof, question)
	
	if speech == "" {
		return "", "", ""
	}

	sg.freq.RecordSpeak(reason, e.State, cfg, now)
	sg.lastSpeech = speech // 履歴に保存

	if reason == ReasonSuccess {
		sg.successStreak++
		sg.failStreak = 0
	} else if reason == ReasonFail {
		sg.successStreak = 0
		sg.failStreak++
	} else {
		sg.failStreak = 0
	}

	finalSpeech := postProcess(speech, cfg.Language)
	log.Printf("[DEBUG] Generated speech [%s]: '%s'", backend, finalSpeech)
	return finalSpeech, prompt, backend
}

func (sg *SpeechGenerator) IsUsingFallback() bool {
	return sg.usingFallback
}

func (sg *SpeechGenerator) UpdateLLMConfig(cfg *config.Config) {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	sg.router.ollama = NewOllamaClient(cfg.OllamaEndpoint, cfg.Model)
	sg.router.claude = NewAnthropicClient(cfg.AnthropicAPIKey)
	sg.router.gemini = NewGeminiClient(cfg.GeminiAPIKey)
}

func (sg *SpeechGenerator) OnUserClick(e monitor.MonitorEvent, cfg *config.Config, prof profile.DevProfile) (string, string, string) {
	return sg.Generate(e, cfg, ReasonUserClick, prof, "")
}

func (sg *SpeechGenerator) OnUserQuestion(e monitor.MonitorEvent, cfg *config.Config, prof profile.DevProfile, question string) (string, string, string) {
	return sg.Generate(e, cfg, ReasonUserQuestion, prof, question)
}

// GenerateQuestion uses LLM to create a personality question.
func (sg *SpeechGenerator) GenerateQuestion(userName string, trait types.TraitID, progress types.TraitProgress, recentBehavior string, language string) (types.Question, error) {
	sg.mu.RLock()
	router := sg.router
	sg.mu.RUnlock()

	qLang := "ja"
	if language == "en" {
		qLang = "en"
	}

	input := OllamaInput{
		UserName:     userName,
		TraitID:      string(trait),
		TraitLabel:   i18n.T(qLang, "trait."+string(trait)),
		CurrentStage: progress.CurrentStage,
		LastAnswer:   progress.LastAnswer,
		PastAnswers:  progress.AskedTopics,
		Behavior:     recentBehavior, // Stage 2 用のコンテキスト
		Language:     "question_" + qLang,
		RandomSeed:   time.Now().UnixNano() % 100000,
	}

	text, _, _, err := router.Route(context.Background(), input)
	if err != nil {
		return types.Question{}, err
	}

	log.Printf("[DEBUG] Raw question response: %s", text)

	cleaned := stripCodeBlock(text)
	// モデルが JSON 終端に 」を使う場合、JSON 構造文字の前の 」は " に変換する
	cleaned = regexp.MustCompile(`」([,}\]])`).ReplaceAllString(cleaned, `"$1`)
	cleaned = strings.ReplaceAll(cleaned, "」", "")
	cleaned = strings.ReplaceAll(cleaned, "「", "")
	var q types.Question
	if err := json.Unmarshal([]byte(cleaned), &q); err != nil {
		return types.Question{}, fmt.Errorf("json unmarshal failed. cleaned text: %s, error: %w", cleaned, err)
	}
	q.TraitID = trait
	return q, nil
}

func (sg *SpeechGenerator) generateTextLocked(e monitor.MonitorEvent, cfg *config.Config, reason Reason, prof profile.DevProfile, question string) (string, string, string) {
	now := time.Now().UTC()
	newMood := InferMoodState(now, sg.successStreak, reason)
	sg.currentMood = newMood

	// 同一ファイル編集回数の追跡（sharpタイプ判定に使用）
	if reason == ReasonActiveEdit && e.Details != "" {
		if e.Details == sg.lastDetails {
			sg.sameFileEditCount++
		} else {
			sg.sameFileEditCount = 0
		}
	} else if reason == ReasonSuccess || reason == ReasonGitCommit || reason == ReasonGitPush {
		sg.sameFileEditCount = 0
	}
	sg.lastDetails = e.Details
	sg.poolLanguage = cfg.Language

	// 生成戦略を strategy.go のテーブルで決定する。
	// 新しい Reason を追加した際は strategy.go の reasonStrategies に追記する。
	if strategyFor(reason, question != "") == StrategyDirect {
		return sg.generateDirect(e, cfg, reason, prof, question)
	}

	// それ以外はプールから取り出す
	personality := sg.inferPersonalityType(reason, cfg)
	category := poolCategory(reason)
	key := poolKey(personality, category, cfg.Language)

	// プールから重複なしで取り出す（最大5回試行）
	for i := 0; i < 5; i++ {
		speech, ok := sg.pool.Pop(key)
		if !ok {
			break
		}
		if sg.state != nil && sg.state.IsDuplicate(speech) {
			log.Printf("[POOL] Skipped duplicate from pool: %s", speech)
			continue
		}
		if sg.pool.NeedsRefill(key) {
			go sg.triggerRefill(key, personality, string(cfg.RelationshipMode), category, cfg.Language, cfg.UserName, prof)
		}
		sg.usingFallback = false
		// プール生成テキスト内の〇〇プレースホルダーをユーザー名に置換
		if cfg.UserName != "" {
			speech = strings.ReplaceAll(speech, "〇〇", cfg.UserName)
		}
		if sg.state != nil {
			sg.state.AddLine(speech)
		}
		return speech, "[POOL]", "Pool"
	}

	// プールが空または全試行が重複: 非同期補充してフォールバック
	go sg.triggerRefill(key, personality, string(cfg.RelationshipMode), category, cfg.Language, cfg.UserName, prof)
	sg.usingFallback = true
	return sg.fallbackSpeech(reason, cfg), "[FALLBACK-POOL]", "Fallback"
}

// generateDirect はLLMを直接呼び出してセリフを生成する（UserQuestion用）。
func (sg *SpeechGenerator) generateDirect(e monitor.MonitorEvent, cfg *config.Config, reason Reason, prof profile.DevProfile, question string) (string, string, string) {
	moodStr := string(sg.currentMood)

	workMem, _ := memory.BuildMemory()
	memStr := ""
	if workMem != nil {
		memStr = workMem.String()
	}

	input := OllamaInput{
		State:            string(e.State),
		Task:             string(e.Task),
		Behavior:         humanizeBehavior(string(e.Behavior.Type), cfg.Language),
		SessionMode:      string(e.Session.Mode),
		FocusLevel:       e.Session.FocusLevel,
		Mood:             moodStr,
		Name:             cfg.Name,
		UserName:         cfg.UserName,
		Tone:             cfg.Tone,
		Reason:           humanizeReason(reason, cfg.Language),
		Event:            reasonToEventContext(reason),
		Details:          e.Details,
		RelationshipLvl:  prof.Relationship.Level,
		Trust:            prof.Relationship.Trust,
		NightCoder:       prof.NightCoder,
		CommitFrequency:  prof.CommitFrequency,
		BuildFailRate:    prof.BuildFailRate,
		TimeOfDay:        getTimeOfDay(time.Now().Hour(), cfg.Language),
		Language:         cfg.Language,
		Question:         question,
		IsAnswerReaction: reason == ReasonQuestionAnswered,
		WorkMemory:       memStr,
		LastAnswer:       sg.lastSpeech,
		PersonalityType:  sg.inferPersonalityType(reason, cfg),
		RelationshipMode: string(cfg.RelationshipMode),
		LearnedTraits:    make(map[string]float64),
		RandomSeed:       time.Now().UnixNano() % 100000,
	}

	for k, v := range prof.Personality.Traits {
		input.LearnedTraits[string(k)] = v
	}

	maxRetries := 3
	dupCount := 0
	var text, backend, prompt string

	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			input.RandomSeed = time.Now().UnixNano() % 100000
		}
		var err error
		text, backend, prompt, err = sg.router.Route(context.Background(), input)
		if err != nil || backend == "Fallback" {
			sg.usingFallback = true
			return sg.fallbackSpeech(reason, cfg), "[FALLBACK]", "Fallback"
		}
		if sg.state != nil && sg.state.IsDuplicate(text) {
			dupCount++
			log.Printf("[INFO] Duplicate detected (%d/%d), retrying: %s", dupCount, maxRetries, text)
			continue
		}
		break
	}

	if dupCount >= maxRetries {
		sg.usingFallback = true
		return sg.fallbackSpeech(reason, cfg), "[FALLBACK-DUP]", "Fallback"
	}

	sg.usingFallback = false
	if sg.state != nil && text != "" {
		sg.state.AddLine(text)
	}
	return text, prompt, backend
}

// triggerRefill はバックグラウンドでプールを補充する。
func (sg *SpeechGenerator) triggerRefill(key, personality, relationship, category, language, userName string, prof profile.DevProfile) {
	if sg.pool.IsRefilling(key) {
		return
	}
	sg.pool.SetRefilling(key, true)
	defer sg.pool.SetRefilling(key, false)

	// 直近の発言履歴をavoidリストとして注入（バッチ生成の重複を防ぐ）
	var recentLines []string
	if sg.state != nil {
		recentLines = sg.state.GetRecentLines(5)
	}

	// 動的Avoidリスト: 過去に破棄されたセリフパターンをバッチプロンプトに注入
	discardedPatterns := sg.pool.GetDiscarded(key)

	req := BatchRequest{
		Personality:       personality,
		RelationshipMode:  relationship,
		Category:          category,
		Language:          language,
		UserName:          userName,
		LearnedTraits:     make(map[string]float64),
		Count:             poolBatchSize,
		RecentLines:       recentLines,
		DiscardedPatterns: discardedPatterns,
	}

	for k, v := range prof.Personality.Traits {
		req.LearnedTraits[string(k)] = v
	}

	// log.Printf("[POOL] Refilling pool for %s (batch=%d)", key, poolBatchSize)
	speeches, err := sg.router.BatchGenerate(context.Background(), req)
	if err != nil {
		// log.Printf("[POOL] BatchGenerate failed for %s: %v", key, err)
		return
	}
	// 生成数が少なすぎる場合はもう1回リトライ
	if len(speeches) < 2 {
		more, retryErr := sg.router.BatchGenerate(context.Background(), req)
		if retryErr == nil && len(more) > len(speeches) {
			speeches = more
		}
	}
	if len(speeches) == 0 {
		return
	}

	// 生成されたセリフをバリデーション。破棄されたものは動的Avoidリストに追加。
	validSpeeches := make([]string, 0, len(speeches))
	for _, s := range speeches {
		if isValidSpeechForLang(s, language) {
			validSpeeches = append(validSpeeches, s)
		} else {
			log.Printf("[POOL] Discarded unnatural speech: %s", s)
			sg.pool.AddDiscarded(key, s) // 動的Avoidリストに記録
		}
	}

	// 評価LLMで上位evalKeepCount件に絞り込む（複数候補がある場合のみ）
	if len(validSpeeches) > evalKeepCount {
		var recentForEval []string
		if sg.state != nil {
			recentForEval = sg.state.GetRecentLines(3)
		}
		if selected := sg.evaluateCandidates(context.Background(), validSpeeches, recentForEval, language); selected != nil {
			filtered := make([]string, 0, len(selected))
			for _, idx := range selected {
				filtered = append(filtered, validSpeeches[idx])
			}
			log.Printf("[EVAL] Selected %d/%d speeches via evaluator", len(filtered), len(validSpeeches))
			validSpeeches = filtered
		}
	}

	if len(validSpeeches) > 0 {
		sg.pool.Push(key, validSpeeches)
	} else {
		// 全件破棄された場合は一定時間リトライを抑制する（Ollama無駄呼び出し防止）
		log.Printf("[POOL] All speeches discarded for %s, setting cooldown %v", key, poolRefillCooldown)
		sg.pool.SetCooldown(key, poolRefillCooldown)
	}
}

// poolCategory はイベントの理由からプールカテゴリを返す。
func poolCategory(reason Reason) string {
	switch reason {
	case ReasonThinkingTick, ReasonIdle:
		return "heartbeat"
	case ReasonActiveEdit, ReasonDocWriting, ReasonAISessionActive, ReasonProductiveToolActivity:
		return "working"
	case ReasonSuccess, ReasonGitCommit, ReasonGitPush:
		return "achievement"
	case ReasonFail, ReasonLongInactivity:
		return "struggle"
	default:
		return "greeting"
	}
}

func reasonToEventContext(reason Reason) string {
	switch reason {
	case ReasonSuccess:
		return "build_success"
	case ReasonFail:
		return "build_failed"
	case ReasonGreeting:
		return "greeting"
	default:
		return ""
	}
}

func humanizeBehavior(b, lang string) string {
	key := "behavior." + b
	result := i18n.T(lang, key)
	if result == key {
		return b // 未定義のキーはそのまま返す
	}
	return result
}

func humanizeReason(r Reason, lang string) string {
	key := "reason." + string(r)
	result := i18n.T(lang, key)
	if result == key {
		return i18n.T(lang, "reason.default")
	}
	return result
}

// stripCodeBlock はモデルが返すテキストからJSON部分を抽出し、整形する。
func stripCodeBlock(s string) string {
	// 1. マークダウンのコードブロック記法があれば中身を取り出す
	if idx := strings.Index(s, "```"); idx >= 0 {
		content := s[idx+3:]
		// json などの言語指定があればスキップ
		if endLine := strings.Index(content, "\n"); endLine >= 0 && endLine < 10 {
			content = content[endLine+1:]
		}
		if endIdx := strings.Index(content, "```"); endIdx >= 0 {
			s = content[:endIdx]
		} else {
			s = content
		}
	}

	// 2. 最初の { と 最後の } の間を抜き出す（説明文対策）
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	// 3. 改行や不要な空白を除去して1行にまとめる（小規模モデルの不正な改行対策）
	lines := strings.Split(s, "\n")
	var buffer strings.Builder
	for _, line := range lines {
		buffer.WriteString(strings.TrimSpace(line))
	}
	
	return buffer.String()
}

// wrongScriptRunes は lang の設定と合わない文字スクリプトが含まれているか判定する。
// 「言語設定外の文字が混入した = LLM が言語を間違えた」として破棄判定に使う。
func wrongScriptRunes(s, lang string) bool {
	for _, r := range s {
		// いずれの言語でもあってはならないスクリプト
		switch {
		case r >= 0xAC00 && r <= 0xD7A3: return true // ハングル音節
		case r >= 0x1100 && r <= 0x11FF: return true // ハングル字母
		case r >= 0x3130 && r <= 0x318F: return true // ハングル互換字母
		case r >= 0x0600 && r <= 0x06FF: return true // アラビア語
		case r >= 0x0900 && r <= 0x097F: return true // デーヴァナーガリー
		case r >= 0x0400 && r <= 0x04FF: return true // キリル文字
		case r >= 0x0590 && r <= 0x05FF: return true // ヘブライ語
		case r >= 0x0E00 && r <= 0x0E7F: return true // タイ語
		}
		// 英語設定では日本語・CJK 系も不正
		if lang == "en" {
			switch {
			case r >= 0x3040 && r <= 0x309F: return true // ひらがな
			case r >= 0x30A0 && r <= 0x30FF: return true // カタカナ
			case r >= 0x4E00 && r <= 0x9FFF: return true // CJK統合漢字
			case r >= 0x3400 && r <= 0x4DBF: return true // CJK拡張A
			}
		}
	}
	return false
}

func postProcess(s, lang string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// 1. 共通の不要記号・装飾の削除
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "「", "")
	s = strings.ReplaceAll(s, "」", "")
	s = strings.ReplaceAll(s, "『", "")
	s = strings.ReplaceAll(s, "』", "")
	s = strings.ReplaceAll(s, "“", "")
	s = strings.ReplaceAll(s, "”", "")
	s = strings.ReplaceAll(s, "〘", "")
	s = strings.ReplaceAll(s, "〙", "")
	s = strings.ReplaceAll(s, "【", "")
	s = strings.ReplaceAll(s, "】", "")

	// 2. カッコ書き（ト書き・感情表現）の削除
	// ( ), （ ）, [ ], < > などの内部を削除
	reBrackets := regexp.MustCompile(`[（\(\{<\[].*?[）\)\}>\]]`)
	s = reBrackets.ReplaceAllString(s, "")

	// 3. 特殊な記号や絵文字の断片的なものを削除
	reSymbols := regexp.MustCompile(`[φðıिच]`)
	s = reSymbols.ReplaceAllString(s, "")

	s = strings.TrimSpace(s)

	if s == "" {
		return ""
	}

	runes := []rune(s)
	if len(runes) > 80 {
		runes = runes[:80]
	}
	s = string(runes)

	// 4. "先輩" (Senpai) が複数ある場合は1つにする
	if strings.Count(s, "先輩") > 1 {
		first := strings.Index(s, "先輩")
		s = s[:first+6] + strings.ReplaceAll(s[first+6:], "先輩", "")
	}

	// 5. 言語設定外の文字スクリプトが含まれていたら破棄（LLM 言語混入）
	if wrongScriptRunes(s, lang) {
		log.Printf("[WARN] postProcess: wrong-script chars detected (lang=%s), discarding: %s", lang, s)
		return ""
	}

	return s
}

// isValidSpeech はセリフが自然か、不純物が混じっていないかチェックする。
func isValidSpeech(s string) bool {
	return isValidSpeechForLang(s, "ja")
}

func isValidSpeechForLang(s, lang string) bool {
	if s == "" {
		return false
	}

	// アルファベットのみの単語が長すぎる場合は怪しい（コード混入の疑い）
	words := strings.Fields(s)
	for _, w := range words {
		if len(w) > 15 && regexp.MustCompile(`^[a-zA-Z]+$`).MatchString(w) {
			return false
		}
	}

	if lang == "en" {
		// 英語モード: 日本語文字が混入していないかチェック
		if regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`).MatchString(s) {
			return false
		}
		// 春・花の詩的比喩、日本語スラングを弾く
		bannedEN := []string{"blossom", "spring breeze", "spring wind", "unfurl", "gentle stream", "petal", "senpai", "cherry",
			"lovely to see", "lovely to watch", "i feel calm", "i feel safe", "i feel peaceful", "watching you work", "observing your"}
		sl := strings.ToLower(s)
		for _, b := range bannedEN {
			if strings.Contains(sl, b) {
				return false
			}
		}
		return true
	}

	// 日本語モード: 禁止ワード・比喩のチェック
	banned := []string{"魔法", "ダンス", "宝石", "芸術", "宝物"}
	for _, b := range banned {
		if strings.Contains(s, b) {
			return false
		}
	}

	// 日本語（ひらがな・カタカナ・漢字）が全く含まれていないのはNG
	if !regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`).MatchString(s) {
		return false
	}

	return true
}

// FrequencyController は発話頻度を制御する。
type FrequencyController struct {
	mu                sync.Mutex
	lastState         types.ContextState
	lastSpeakTime     time.Time
	cooldownUntil     time.Time
	consecutive       int
	lastWebSpeakTime  time.Time
}

func NewFrequencyController() *FrequencyController {
	return &FrequencyController{}
}

func (fc *FrequencyController) ShouldSpeak(reason Reason, state types.ContextState, cfg *config.Config, now time.Time) bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// 常に発話するイベント（クールダウン無視）
	alwaysSpeak := reason == ReasonUserClick ||
		reason == ReasonGreeting ||
		reason == ReasonInitSetup ||
		reason == ReasonDevSessionStarted ||
		reason == ReasonAISessionStarted ||
		reason == ReasonGitCommit ||
		reason == ReasonGitPush ||
		reason == ReasonUserQuestion ||
		reason == ReasonQuestionAnswered
	if alwaysSpeak {
		return true
	}

	// Webブラウジングは専用クールダウン（3分）で制御
	if reason == ReasonWebBrowsing {
		if !fc.lastWebSpeakTime.IsZero() && now.Sub(fc.lastWebSpeakTime) < 3*time.Minute {
			return false
		}
		return true
	}

	if !fc.lastSpeakTime.IsZero() && now.Sub(fc.lastSpeakTime) < 30*time.Second {
		return false
	}

	switch reason {
	case ReasonSuccess, ReasonFail:
		if state != fc.lastState {
			return true
		}
		return false

	case ReasonThinkingTick:
		if !cfg.Monologue || now.Before(fc.cooldownUntil) {
			return false
		}
		
		interval := 10 * time.Minute
		switch cfg.SpeechFrequency {
		case 1:
			interval = 20 * time.Minute
		case 3:
			interval = 1 * time.Minute 
		}

		if now.Sub(fc.lastSpeakTime) < interval {
			return false
		}
		return true
	}

	return true
}

func (fc *FrequencyController) RecordSpeak(reason Reason, state types.ContextState, cfg *config.Config, now time.Time) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.lastSpeakTime = now
	fc.lastState = state
	if reason == ReasonWebBrowsing {
		fc.lastWebSpeakTime = now
	}
	
	if reason == ReasonThinkingTick {
		fc.consecutive++
		if fc.consecutive >= 3 {
			fc.cooldownUntil = now.Add(15 * time.Minute)
			fc.consecutive = 0
		}
	} else {
		fc.consecutive = 0
	}
}

// genki  : ビルド成功、コミット/プッシュ時、または調子が良い（成功継続）とき
// tsukime: 同一ファイル3回以上編集 / 長時間アイドル / ビルド連続失敗（2回以上）
// cute   : デフォルト
func (sg *SpeechGenerator) inferPersonalityType(reason Reason, cfg *config.Config) string {
	// Config で指定されている場合はそれを優先
	if cfg.PersonaStyle == types.StyleGenki || cfg.PersonaStyle == types.StyleCute || cfg.PersonaStyle == types.StyleTsukime {
		return string(cfg.PersonaStyle)
	}

	// 互換性マッピング
	switch cfg.PersonaStyle {
	case types.StyleEnergetic:
		return "genki"
	case types.StyleStrict:
		return "tsukime"
	case types.StyleSoft:
		return "cute"
	}

	// 自動推論
	// 1. Tsukime (優先度高: 苦戦しているときは気づかってあげる)
	if (reason == ReasonFail && sg.failStreak >= 1) ||
		sg.sameFileEditCount >= 3 ||
		reason == ReasonLongInactivity {
		return "tsukime"
	}

	// 2. Genki (成功時や、ノリに乗っているとき)
	isSuccessful := reason == ReasonSuccess || reason == ReasonGitCommit || reason == ReasonGitPush
	isFeelingGood := sg.successStreak >= 1 || sg.currentMood == MoodStateHappy

	if isSuccessful || (isFeelingGood && reason != ReasonThinkingTick) {
		return "genki"
	}

	// 3. Cute (通常時)
	return "cute"
}

func getTimeOfDay(h int, lang string) string {
	var key string
	switch {
	case h >= 5 && h < 10:
		key = "time.morning"
	case h >= 10 && h < 17:
		key = "time.noon"
	case h >= 17 && h < 20:
		key = "time.afternoon"
	case h >= 20 && h < 23:
		key = "time.evening"
	default:
		key = "time.night"
	}
	return i18n.T(lang, key)
}

func (sg *SpeechGenerator) fallbackSpeech(reason Reason, cfg *config.Config) string {
	text := FallbackSpeech(reason, cfg.Language)
	userName := cfg.UserName
	if userName == "" {
		userName = "先輩"
	}
	text = strings.ReplaceAll(text, "{{UserName}}", userName)
	text = strings.ReplaceAll(text, "{{username}}", userName)
	text = strings.ReplaceAll(text, "{UserName}", userName)
	text = strings.ReplaceAll(text, "{username}", userName)
	return text
}

// IsAvailable は特定のバックエンドが利用可能かチェックする。
func (sg *SpeechGenerator) IsAvailable(backend string) bool {
	switch backend {
	case "ollama":
		return sg.router.ollama != nil && sg.router.ollama.IsAvailable()
	case "claude":
		return sg.router.claude != nil && sg.router.claude.IsAvailable()
	case "gemini":
		return sg.router.gemini != nil && sg.router.gemini.IsAvailable()
	}
	return false
}
