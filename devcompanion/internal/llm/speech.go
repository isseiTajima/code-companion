package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
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
	mu                  sync.RWMutex
	router              *LLMRouter
	cache               *SpeechCache
	state               *SpeechState
	freq                *FrequencyController
	pool                *SpeechPool
	currentMood         MoodState
	successStreak       int
	failStreak          int
	sameFileEditCount   int
	usingFallback       bool
	lastSpeech          string     // 前回の発言を保持
	lastDetails         string     // 前回の作業対象を保持
	convHistory         []ConvTurn // UserQuestion の直近会話履歴
	lastConvAt          time.Time  // 最後に UserQuestion を受けた時刻（文脈タイムアウト判定用）
	poolLanguage        string    // プール補充時に使用する言語設定
	codingSessionStart  time.Time // 現在のコーディングセッション開始時刻
	lastCodingEventAt   time.Time // 最後にコーディングイベントを受けた時刻
	// personality 安定化
	personality         *PersonalityManager
	genkiSpeechCount    int    // genki 状態での発話数（減衰カウンタ）
	// 今日のコミット数（SituationHint用）
	todayCommits        int
	todayCommitsDate    string // "20060102" 形式
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
		personality:  NewPersonalityManager(),
	}
	SetSeed(time.Now().UnixNano())

	// 起動時に頻出カテゴリのプールをプリウォーム（最初のイベントでfallbackにならないよう）
	// 注意: 起動直後は学習データがないため空のプロファイル
	go sg.prewarmSpeechPools(cfg.Language, cfg.UserName, string(cfg.PersonaStyle), profile.DevProfile{})

	return sg
}

// prewarmSpeechPools は起動時に頻出カテゴリのプールを事前充填する。
//
// personalityStyle が明示設定されている場合（cute/genki/cool/oneesan）はその1種のみを充填する。
// 未設定の場合は PersonalityManager が cute/cool/genki を動的に切り替えるため3種を充填する。
// これにより未使用 personality のバッチ生成コストを削減する。
//
// 将来的に personality のライブ切り替えを実装する際は、
// 切り替えイベント受信時に新 personality の refill をトリガーする形で拡張する。
func (sg *SpeechGenerator) prewarmSpeechPools(language, userName, personalityStyle string, prof profile.DevProfile) {
	// 挨拶生成にリソースを譲るため少し待機
	time.Sleep(3 * time.Second)

	// prewarm 対象の personality を決定する
	// - 明示設定あり → その1種のみ
	// - 未設定 → PersonalityManager が遷移できる3種（cute/cool/genki）
	fixedPersonalities := map[string]bool{"cute": true, "genki": true, "cool": true, "oneesan": true}
	var warmPersonalities []string
	if fixedPersonalities[personalityStyle] {
		warmPersonalities = []string{personalityStyle}
	} else {
		warmPersonalities = []string{"cute", "genki", "cool"}
	}

	// reaction を並列充填（UserClick 即応のため）
	var wg sync.WaitGroup
	for _, p := range warmPersonalities {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			key := poolKey(p, "reaction", language)
			sg.triggerRefillFast(key, p, "reaction", language, userName, prof)
		}(p)
	}
	wg.Wait()

	// reaction 以外のカテゴリは直列（Ollama への同時リクエスト集中を避ける）
	allSequential := []struct{ personality, category string }{
		{"genki", "working"},
		{"genki", "heartbeat"},
		{"genki", "achievement"},
		{"cute", "greeting"},
		{"cute", "heartbeat"},
		{"cute", "working"},
		{"cute", "session_start"},
		{"cute", "ai_start"},
		{"cute", "ai_pairing"},
		{"cute", "private"},
		{"cool", "session_start"},
		{"cool", "ai_start"},
		{"cool", "struggle"},
		{"cool", "night"},
		{"cool", "private"},
		{"oneesan", "working"},
		{"oneesan", "private"},
		{"genki", "private"},
	}
	warmSet := make(map[string]bool, len(warmPersonalities))
	for _, p := range warmPersonalities {
		warmSet[p] = true
	}
	for _, p := range allSequential {
		if !warmSet[p.personality] {
			continue
		}
		key := poolKey(p.personality, p.category, language)
		sg.triggerRefillFast(key, p.personality, p.category, language, userName, prof)
	}
}

// reasonMeta は各 Reason の性質をまとめたテーブル。
// 新しい Reason を追加する際はここに1行追記するだけでよい。
type reasonMeta struct {
	highPriority bool   // true = mu.Lock()（必ず発話）、false = TryLock（スキップ可）
	poolCategory string // "heartbeat" / "working" / "achievement" / "struggle" / "greeting"
	eventContext string // OllamaInput.Event に渡す文字列（空文字はデフォルト）
	alwaysSpeak  bool   // true = クールダウン無視で必ず発話
	isRoutine    bool   // true = SpeechFrequency 連動インターバル制御
	webCooldown  bool   // true = 専用 3 分クールダウン（WebBrowsing 専用）
}

var reasonTable = map[Reason]reasonMeta{
	// --- 高優先度（必ず発話） ---
	ReasonGreeting:         {highPriority: true, poolCategory: "greeting", eventContext: "greeting", alwaysSpeak: true},
	ReasonInitSetup:        {highPriority: true, poolCategory: "greeting", eventContext: "greeting", alwaysSpeak: true},
	ReasonUserQuestion:     {highPriority: true, poolCategory: "reaction", alwaysSpeak: true},
	ReasonQuestionAnswered: {highPriority: true, poolCategory: "reaction", alwaysSpeak: true},
	ReasonUserClick:        {highPriority: true, poolCategory: "reaction", eventContext: "user_click", alwaysSpeak: true},
	// --- alwaysSpeak（クールダウン無視） ---
	ReasonDevSessionStarted:     {poolCategory: "session_start", alwaysSpeak: true},
	ReasonAISessionStarted:      {poolCategory: "ai_start", alwaysSpeak: true},
	ReasonGitCommit:             {poolCategory: "achievement", alwaysSpeak: true},
	ReasonGitPush:               {poolCategory: "achievement", alwaysSpeak: true},
	// --- 通常発話（SpeechFrequency 連動） ---
	ReasonActiveEdit:             {poolCategory: "working", isRoutine: true},
	ReasonDocWriting:             {poolCategory: "working", isRoutine: true},
	ReasonAISessionActive:        {poolCategory: "ai_pairing", isRoutine: true},
	ReasonProductiveToolActivity: {poolCategory: "working", isRoutine: true},
	ReasonNightWork:              {poolCategory: "night", isRoutine: true},
	ReasonIdle:                   {poolCategory: "heartbeat", isRoutine: true},
	ReasonLongInactivity:         {poolCategory: "struggle", isRoutine: true},
	ReasonInitObservation:        {poolCategory: "working", isRoutine: true},
	ReasonInitSupport:            {poolCategory: "working", isRoutine: true},
	ReasonInitCuriosity:          {poolCategory: "working", isRoutine: true, eventContext: "initiative_curiosity"},
	ReasonInitMemory:             {poolCategory: "working", isRoutine: true},
	ReasonInitWeather:            {poolCategory: "working", isRoutine: true},
	// --- 専用クールダウン ---
	ReasonWebBrowsing: {poolCategory: "working", webCooldown: true, eventContext: "web_browsing"},
	// --- 状態変化トリガー ---
	ReasonSuccess:      {poolCategory: "achievement", eventContext: "build_success"},
	ReasonFail:         {poolCategory: "struggle", eventContext: "build_failed"},
	ReasonGitAdd:       {poolCategory: "achievement"},
	ReasonThinkingTick: {poolCategory: "heartbeat"},
}

// reasonInfo は reasonTable から取得する。未登録の Reason は greeting カテゴリのデフォルト値を返す。
func reasonInfo(r Reason) reasonMeta {
	if m, ok := reasonTable[r]; ok {
		return m
	}
	return reasonMeta{poolCategory: "greeting"}
}

// highPriorityReason は TryLock でなく Lock() を使うべき高優先度 Reason。
func highPriorityReason(r Reason) bool {
	return reasonInfo(r).highPriority
}

func (sg *SpeechGenerator) Generate(e monitor.MonitorEvent, cfg *config.Config, reason Reason, prof profile.DevProfile, question string) (string, string, string) {
	if highPriorityReason(reason) {
		log.Printf("[LLM] Generate: high-priority reason=%s, acquiring lock", reason)
		sg.mu.Lock() // 高優先度: 必ず実行（別の生成が終わるまで待つ）
		log.Printf("[LLM] Generate: lock acquired for reason=%s", reason)
	} else if !sg.mu.TryLock() {
		return "", "", ""
	}
	defer sg.mu.Unlock()

	if cfg.Mute {
		log.Printf("[LLM] Generate: muted, skipping reason=%s", reason)
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

	// Audit: 表示されたセリフを記録
	globalAudit.LogSpeech(speech, string(reason), sg.inferPersonalityType(reason, cfg), poolCategory(reason), cfg.Language, sourceFromBackend(backend))

	// UserQuestion の場合は会話履歴に記録（連続会話のコンテキスト維持）
	if reason == ReasonUserQuestion && question != "" && speech != "" {
		const convTimeout = 15 * time.Minute
		// タイムアウト超過 = 話題が変わったとみなし履歴をリセット
		if !sg.lastConvAt.IsZero() && time.Since(sg.lastConvAt) > convTimeout {
			sg.convHistory = nil
		}
		sg.convHistory = append(sg.convHistory, ConvTurn{Role: "user", Text: question})
		sg.convHistory = append(sg.convHistory, ConvTurn{Role: "sakura", Text: speech})
		sg.lastConvAt = now
	}

	if reason == ReasonSuccess {
		sg.successStreak++
		sg.failStreak = 0
	} else if reason == ReasonFail && !e.IsAISession {
		// AIセッション中のエラーはAIエージェントのもの。failStreakを増やさない
		sg.successStreak = 0
		sg.failStreak++
	} else {
		sg.failStreak = 0
	}

	sg.updatePersonality(reason)

	log.Printf("[DEBUG] Generated speech [%s]: '%s'", backend, speech)
	return speech, prompt, backend
}

func (sg *SpeechGenerator) IsUsingFallback() bool {
	return sg.usingFallback
}

func (sg *SpeechGenerator) UpdateLLMConfig(cfg *config.Config) {
	newRouter := &LLMRouter{
		ollama: NewOllamaClient(cfg.OllamaEndpoint, cfg.Model),
		claude: NewAnthropicClient(cfg.AnthropicAPIKey),
		gemini: NewGeminiClient(cfg.GeminiAPIKey),
		aiCLI:  sg.router.aiCLI, // 設定非依存なので既存インスタンスを引き継ぐ
	}
	// ロック待ちで SaveConfig が詰まらないよう goroutine で非同期に更新する
	go func() {
		sg.mu.Lock()
		sg.router = newRouter
		sg.mu.Unlock()
		// 設定変更時はプールをフラッシュして新しい設定で即再生成させる
		sg.pool.ClearAll()
	}()
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
	// LLM が options を ["a"],["b"],["c"] と出力する壊れ形式を ["a","b","c"] に修復する
	cleaned = strings.ReplaceAll(cleaned, "],[", ",")
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

	// コーディングセッション継続時間を自前で管理する。
	// e.Session.StartTime はアプリ起動時刻で変わらないため使用不可。
	const codingIdleTimeout = 30 * time.Minute
	isCodingEvent := e.State == types.StateCoding || e.State == types.StateDeepWork ||
		reason == ReasonActiveEdit || reason == ReasonGitCommit || reason == ReasonGitPush
	if isCodingEvent {
		if sg.codingSessionStart.IsZero() || now.Sub(sg.lastCodingEventAt) > codingIdleTimeout {
			sg.codingSessionStart = now // アイドル後の再開でリセット
			// セッションリセット時はパーソナリティも cute に戻す
			sg.personality.SetCurrent(PersonalityCute)
			sg.genkiSpeechCount = 0
		}
		sg.lastCodingEventAt = now
	}
	workingDuration := ""
	sessionMins := 0
	if isCodingEvent && !sg.codingSessionStart.IsZero() {
		sessionMins = int(now.Sub(sg.codingSessionStart).Minutes())
		switch {
		case sessionMins >= 120:
			workingDuration = "long"
		case sessionMins >= 30:
			workingDuration = "medium"
		case sessionMins >= 10:
			workingDuration = "short"
		}
	}

	// 今日のコミット数を追跡（日付が変わったらリセット）
	if reason == ReasonGitCommit {
		today := now.Format("20060102")
		if sg.todayCommitsDate != today {
			sg.todayCommits = 0
			sg.todayCommitsDate = today
		}
		sg.todayCommits++
	}
	situationHint := buildSituationHint(sessionMins, sg.todayCommits)

	// 生成戦略を strategy.go のテーブルで決定する。
	// 新しい Reason を追加した際は strategy.go の reasonStrategies に追記する。
	if strategyFor(reason, question != "") == StrategyDirect {
		return sg.generateDirect(e, cfg, reason, prof, question)
	}

	// それ以外はプールから取り出す
	personality := sg.inferPersonalityType(reason, cfg)
	category := poolCategory(reason)
	// AIセッション中の通常作業イベントは ai_pairing カテゴリを使う
	// (AIエージェントがファイルを編集しているのにユーザーが打鍵中扱いになるのを防ぐ)
	if e.IsAISession && category == "working" {
		category = "ai_pairing"
	}
	// heartbeat の 25%、working の 5% をプライベートトークに切り替える
	if category == "heartbeat" || category == "working" {
		threshold := float32(0.25)
		if category == "working" {
			threshold = 0.05
		}
		rndMu.Lock()
		doPrivate := rnd.Float32() < threshold
		rndMu.Unlock()
		if doPrivate {
			category = "private"
		}
	}
	key := poolKey(personality, category, cfg.Language)

	// プールから重複なしで取り出す（最大5回試行）
	// 重複と判定されたセリフは消費せずプール末尾に戻す
	var dups []string
	for i := 0; i < 5; i++ {
		speech, ok := sg.pool.Pop(key)
		if !ok {
			break
		}
		if sg.state != nil && sg.state.IsDuplicate(speech) {
			log.Printf("[POOL] Skipped duplicate from pool: %s", speech)
			dups = append(dups, speech)
			continue
		}
		// 使わなかった重複候補をプール末尾に戻す
		if len(dups) > 0 {
			sg.pool.Push(key, dups)
			dups = nil
		}
		if sg.pool.NeedsRefill(key) {
			go sg.triggerRefill(key, personality, string(cfg.RelationshipMode), category, cfg.Language, cfg.UserName, cfg.Dialect, prof, workingDuration, situationHint, cfg.SpeechFrequency)
		}
		sg.usingFallback = false
		// プール生成テキスト内の〇〇プレースホルダーをユーザー名に置換
		if cfg.UserName != "" {
			speech = strings.ReplaceAll(speech, "〇〇", cfg.UserName)
		}
		speech = postProcess(speech, cfg.Language)
		if speech == "" {
			continue
		}
		if sg.state != nil {
			sg.state.AddLine(speech)
		}
		// 発話済みセリフをAvoidリストに追加（次の補充時に重複生成を防ぐ）
		sg.pool.AddDiscarded(key, speech)
		return speech, "[POOL]", "Pool"
	}
	// 全候補が重複だった場合もプールに戻す
	if len(dups) > 0 {
		sg.pool.Push(key, dups)
	}

	// プールが空または全試行が重複: 非同期補充してフォールバック
	log.Printf("[POOL] Pool empty for key=%s (IsRefilling=%v), triggering refill", key, sg.pool.IsRefilling(key))
	go sg.triggerRefill(key, personality, string(cfg.RelationshipMode), category, cfg.Language, cfg.UserName, cfg.Dialect, prof, workingDuration, situationHint, cfg.SpeechFrequency)
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

	// UserName 未設定時はデフォルト呼称を使う（モデルが勝手に名前を補完するのを防ぐ）
	userName := cfg.UserName
	if userName == "" {
		userName = i18n.T(cfg.Language, "speech.default_username")
	}

	input := OllamaInput{
		State:            string(e.State),
		Task:             string(e.Task),
		Behavior:         humanizeBehavior(string(e.Behavior.Type), cfg.Language),
		SessionMode:      string(e.Session.Mode),
		FocusLevel:       e.Session.FocusLevel,
		Mood:             moodStr,
		Name:             cfg.Name,
		UserName:         userName,
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
		IsAnswerReaction:      reason == ReasonQuestionAnswered,
		WorkMemory:            memStr,
		PersonalMemorySummary: buildPersonalMemorySummary(prof.PersonalMemories, cfg.Language, e.Details+" "+question),
		LastAnswer:            sg.lastSpeech,
		ConversationHistory:   sg.convHistory,
		PersonalityType:       sg.inferPersonalityType(reason, cfg),
		RelationshipMode:      string(cfg.RelationshipMode),
		Dialect:               cfg.Dialect,
		Season:                currentSeason(cfg.Language),
		LearnedTraits:         make(map[string]float64),
		LearnedTraitLabels:    make(map[string]string),
		RandomSeed:            time.Now().UnixNano() % 100000,
		IsAISession:           e.IsAISession,
		NewsContext:           e.NewsContext,
		NewsPreferences:       buildNewsPreferences(prof),
		WeatherContext:        e.WeatherContext,
		ExampleSpeeches:       pickExampleSpeeches(cfg.Language, sg.inferPersonalityType(reason, cfg), reason),
		VoiceAtoms:            buildVoiceAtoms(cfg.Language, sg.inferPersonalityType(reason, cfg), reason),
	}

	for k, v := range prof.Personality.Traits {
		input.LearnedTraits[string(k)] = v
	}
	// 回答テキストを優先して渡す（float より意味が明確）
	// 複数回答がある場合は全履歴を結合してLLMに矛盾を認識させる
	for k, prog := range prof.Evolution {
		label := traitLabelFromProgress(prog)
		if label != "" {
			input.LearnedTraitLabels[string(k)] = label
		}
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
			if sg.router.consumeConnWarn() {
				return connWarnSpeech(cfg.Language), "[CONN-WARN]", "Fallback"
			}
			return sg.fallbackSpeech(reason, cfg), "[FALLBACK]", "Fallback"
		}
		// postProcess をリトライループ内で適用し、言語混入があればリトライ
		// ニュース・天気は2文構成なので文字数上限を広げる
		if reason == ReasonInitCuriosity || reason == ReasonInitWeather {
			text = postProcess(text, cfg.Language, 150)
		} else {
			text = postProcess(text, cfg.Language)
		}
		if text == "" {
			dupCount++
			log.Printf("[INFO] postProcess discarded output (%d/%d), retrying", retry+1, maxRetries)
			continue
		}
		// validator チェック: pool と同等の品質フィルターを direct generation にも適用
		if ok, rejBy := isValidSpeechUnifiedDetailed(text, cfg.Language); !ok {
			dupCount++
			log.Printf("[INFO] validator rejected direct output (%d/%d): %s", retry+1, maxRetries, text)
			globalAudit.LogRejected(text, string(reason), rejBy)
			continue
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

// triggerRefill はバックグラウンドでプールを補充する（Evaluatorあり）。
// speechFreq は SpeechFrequency 設定値（1/2/3）。高頻度時は RecentLines を増やして重複を防ぐ。
func (sg *SpeechGenerator) triggerRefill(key, personality, relationship, category, language, userName, dialect string, prof profile.DevProfile, workingDuration, situationHint string, speechFreq int) {
	sg.refill(key, personality, relationship, category, language, userName, dialect, prof, workingDuration, situationHint, speechFreq, true)
}

// triggerRefillFast はプレウォーム専用の高速補充（Evaluatorなし）。
// Ollama への追加リクエストを避けるため、起動時の一括プレウォームで使う。
func (sg *SpeechGenerator) triggerRefillFast(key, personality, category, language, userName string, prof profile.DevProfile) {
	sg.refill(key, personality, "normal", category, language, userName, "", prof, "", "", 0, false)
}

func (sg *SpeechGenerator) refill(key, personality, relationship, category, language, userName, dialect string, prof profile.DevProfile, workingDuration, situationHint string, speechFreq int, withEval bool) {
	if sg.pool.IsRefilling(key) {
		return
	}
	sg.pool.SetRefilling(key, true)
	defer sg.pool.SetRefilling(key, false)

	// UpdateLLMConfig との data race を防ぐため、ロック下でスナップショットを取る。
	// refill はゴルーチンとして起動されるため sg.mu を保持していない。
	sg.mu.RLock()
	router := sg.router
	sg.mu.RUnlock()

	// 直近の発言履歴をavoidリストとして注入（バッチ生成の重複を防ぐ）
	// 高頻度設定時（freq=3）は履歴を多めに渡し、重複をより強く防ぐ
	recentCount := 40
	if speechFreq >= 3 {
		recentCount = 60
	}
	var recentLines []string
	if sg.state != nil {
		recentLines = sg.state.GetRecentLines(recentCount)
	}

	// 動的Avoidリスト: 過去に破棄されたセリフパターンをバッチプロンプトに注入
	discardedPatterns := sg.pool.GetDiscarded(key)

	// 趣味嗜好系のtraitはバッチ生成に渡さない。
	// favorite_season=夏 などをモデルに渡すと季節を obsess するため。
	batchSkipTraits := map[string]bool{
		"favorite_season": true,
		"favorite_food":   true,
		"hobby":           true,
		"favorite_music":  true,
		"favorite_color":  true,
	}
	traitLabels := make(map[string]string)
	for k, prog := range prof.Evolution {
		if batchSkipTraits[string(k)] {
			continue
		}
		label := traitLabelFromProgress(prog)
		if label != "" {
			traitLabels[string(k)] = label
		}
	}

	// PersonalMemory はinitiative_memoryカテゴリのみに渡す。
	// heartbeat/support/observation 等の一般バッチに渡すと4Bモデルが
	// 趣味嗜好（好きな季節など）をテーマとして obsess するため。
	personalMem := ""
	if strings.Contains(category, "memory") {
		personalMem = buildPersonalMemorySummaryRandom(prof.PersonalMemories, language, category+" "+dialect)
	}

	req := BatchRequest{
		Personality:           personality,
		RelationshipMode:      relationship,
		Category:              category,
		Language:              language,
		UserName:              userName,
		LearnedTraits:         make(map[string]float64),
		LearnedTraitLabels:    traitLabels,
		PersonalMemorySummary: personalMem,
		WorkingDuration:       workingDuration,
		SituationHint:         situationHint,
		Count:                 poolBatchSize,
		RecentLines:           recentLines,
		DiscardedPatterns:     discardedPatterns,
		Dialect:               dialect,
		Season:                currentSeason(language),
	}

	for k, v := range prof.Personality.Traits {
		req.LearnedTraits[string(k)] = v
	}

	log.Printf("[POOL] Refilling pool for %s (batch=%d)", key, poolBatchSize)
	speeches, err := router.BatchGenerate(context.Background(), req)
	if err != nil {
		log.Printf("[POOL] BatchGenerate failed for %s: %v", key, err)
		return
	}
	// 生成数が少なすぎる場合はもう1回リトライ
	if len(speeches) < 2 {
		more, retryErr := router.BatchGenerate(context.Background(), req)
		if retryErr == nil && len(more) > len(speeches) {
			speeches = more
		}
	}
	if len(speeches) == 0 {
		log.Printf("[POOL] BatchGenerate returned 0 speeches for %s", key)
		return
	}

	// log.Printf("[POOL] BatchGenerate returned %d speeches for %s:", len(speeches), key)
	// for i, s := range speeches {
	// 	log.Printf("[POOL]   [%d] %q", i+1, s)
	// }

	// 生成されたセリフをバリデーション。破棄されたものは動的Avoidリストに追加。
	validSpeeches := make([]string, 0, len(speeches))
	// 既出セリフを除外するためのdiscardedセット
	discardedSet := make(map[string]bool)
	for _, d := range sg.pool.GetDiscarded(key) {
		discardedSet[d] = true
	}
	// log.Printf("[POOL] Validation results:")
	for _, s := range speeches {
		if ok, rejBy := isValidSpeechUnifiedDetailed(s, language); !ok {
			sg.pool.AddDiscarded(key, s) // 動的Avoidリストに記録
			globalAudit.LogBatchRejected(s, key, rejBy)
			continue
		}
		// バッチセリフは短い一言想定なので55字超えは長すぎる（プールには入れず再生成を促す）
		if len([]rune(s)) > 55 {
			sg.pool.AddDiscarded(key, s)
			globalAudit.LogBatchRejected(s, key, "MaxChars:55")
			continue
		}
		// 既出（discarded）セリフはプールに追加しない（同じセリフの再循環を防ぐ）
		if discardedSet[s] {
			globalAudit.LogBatchRejected(s, key, "AlreadyShown")
			continue
		}
		validSpeeches = append(validSpeeches, s)
	}
	// log.Printf("[POOL] Validation: %d/%d passed", len(validSpeeches), len(speeches))

	// 評価LLMで上位evalKeepCount件に絞り込む（複数候補がある場合のみ）
	if withEval && len(validSpeeches) > evalKeepCount {
		var recentForEval []string
		if sg.state != nil {
			recentForEval = sg.state.GetRecentLines(8)
		}
		log.Printf("[EVAL] Evaluating %d valid candidates:", len(validSpeeches))
		for i, s := range validSpeeches {
			log.Printf("[EVAL]   [%d] %q", i+1, s)
		}
		if selected := sg.evaluateCandidates(context.Background(), validSpeeches, recentForEval, language); selected != nil {
			filtered := make([]string, 0, len(selected))
			for _, idx := range selected {
				filtered = append(filtered, validSpeeches[idx])
			}
			log.Printf("[EVAL] Selected %d/%d via evaluator:", len(filtered), len(validSpeeches))
			for i, s := range filtered {
				log.Printf("[EVAL]   -> [%d] %q", i+1, s)
			}
			validSpeeches = filtered
		} else {
			log.Printf("[EVAL] Evaluator returned nil (using all %d valid speeches)", len(validSpeeches))
		}
	}

	if len(validSpeeches) > 0 {
		sg.pool.Push(key, validSpeeches)
		log.Printf("[POOL] Pushed %d/%d speeches to pool %s", len(validSpeeches), len(speeches), key)
	} else {
		// 全件破棄された場合は一定時間リトライを抑制する（Ollama無駄呼び出し防止）
		log.Printf("[POOL] All speeches discarded for %s, setting cooldown %v", key, poolRefillCooldown)
		sg.pool.SetCooldown(key, poolRefillCooldown)
	}
}

// poolCategory はイベントの理由からプールカテゴリを返す。
func poolCategory(reason Reason) string {
	return reasonInfo(reason).poolCategory
}

func reasonToEventContext(reason Reason) string {
	return reasonInfo(reason).eventContext
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

func postProcess(s, lang string, maxLen ...int) string {
	return postProcessUnified(s, lang, maxLen...)
}

// isValidSpeech はセリフが自然か、不純物が混じっていないかチェックする。
func isValidSpeech(s string) bool {
	return isValidSpeechUnified(s, "ja")
}

func isValidSpeechForLang(s, lang string) bool {
	return isValidSpeechUnified(s, lang)
}

// buildPersonalMemorySummary は PersonalMemories から関連性の高いもの、または直近のものをサマリー文字列に変換する。
// contextKeywords: 検索のヒント（作業詳細など）
func buildPersonalMemorySummary(mems []types.PersonalMemory, lang string, contextKeywords string) string {
	if len(mems) == 0 {
		return ""
	}

	// 抽出候補
	var candidates []types.PersonalMemory

	// 1. キーワードマッチングによる優先抽出
	if contextKeywords != "" {
		keywords := strings.Fields(strings.ToLower(contextKeywords))
		for i := len(mems) - 1; i >= 0; i-- {
			m := mems[i]
			content := strings.ToLower(m.Content)
			for _, kw := range keywords {
				if len(kw) > 1 && strings.Contains(content, kw) {
					candidates = append(candidates, m)
					break
				}
			}
			if len(candidates) >= 2 {
				break
			}
		}
	}

	// 2. 直近のメモリを補充
	start := len(mems) - 5
	if start < 0 {
		start = 0
	}
	recent := mems[start:]
	for i := len(recent) - 1; i >= 0; i-- {
		// 重複チェック
		isDup := false
		for _, c := range candidates {
			if c.Content == recent[i].Content {
				isDup = true
				break
			}
		}
		if !isDup {
			candidates = append(candidates, recent[i])
		}
		if len(candidates) >= 5 {
			break
		}
	}

	var sb strings.Builder
	memFmt := map[string]string{"ja": "- %s「%s」\n", "en": "- %s: \"%s\"\n"}
	fmtStr := memFmt[lang]
	if fmtStr == "" {
		fmtStr = memFmt["ja"]
	}
	for _, m := range candidates {
		fmt.Fprintf(&sb, fmtStr, memoryTimeLabel(m.CreatedAt, lang), m.Content)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// buildPersonalMemorySummaryRandom はバッチ生成用。
// 25%の確率で、コンテキストに関連するものまたは直近のものから1件だけ渡す。
func buildPersonalMemorySummaryRandom(mems []types.PersonalMemory, lang string, contextKeywords string) string {
	if len(mems) == 0 {
		return ""
	}
	// 4回に1回だけ使用（バッチ内での重複・執着を防止）
	if rand.Intn(4) != 0 {
		return ""
	}

	var candidates []types.PersonalMemory

	// 1. キーワードマッチングによる優先抽出（1件）
	if contextKeywords != "" {
		keywords := strings.Fields(strings.ToLower(contextKeywords))
		for i := len(mems) - 1; i >= 0; i-- {
			m := mems[i]
			content := strings.ToLower(m.Content)
			for _, kw := range keywords {
				if len(kw) > 1 && strings.Contains(content, kw) {
					candidates = append(candidates, m)
					break
				}
			}
			if len(candidates) >= 1 {
				break
			}
		}
	}

	// 2. キーワードヒットがない場合は直近3件からランダムに1件
	if len(candidates) == 0 {
		start := len(mems) - 3
		if start < 0 {
			start = 0
		}
		recent := mems[start:]
		candidates = append(candidates, recent[rand.Intn(len(recent))])
	}

	m := candidates[0]
	memFmt := map[string]string{"ja": "- %s「%s」", "en": "- %s: \"%s\""}
	fmtStr := memFmt[lang]
	if fmtStr == "" {
		fmtStr = memFmt["ja"]
	}
	return fmt.Sprintf(fmtStr, memoryTimeLabel(m.CreatedAt, lang), m.Content)
}

// currentSeason は現在月から季節を返す（ja/en対応）。
// buildSituationHint は技術情報を使わず「隣にいれば気づける観察」に変換した文字列を返す。
// バッチプロンプトの SituationHint として注入し、LLM が具体的な状況をアンカーにできるようにする。
// 注意: セッション時間はPCつけっぱなし運用では信頼性がないため使用しない。
func buildSituationHint(sessionMins, todayCommits int) string {
	_ = sessionMins // 使用しない（作業時間コメントを避けるため）
	var parts []string
	switch {
	case todayCommits >= 5:
		parts = append(parts, fmt.Sprintf("今日もう%d回、なんか区切りをつけてます", todayCommits))
	case todayCommits >= 2:
		parts = append(parts, fmt.Sprintf("今日%d回、区切りがついた感じがしました", todayCommits))
	case todayCommits == 1:
		parts = append(parts, "さっきなんか一区切りついた感じがしました")
	}
	return strings.Join(parts, "、")
}

func currentSeason(lang string) string {
	m := time.Now().Month()
	type seasonEntry struct{ ja, en string }
	var s seasonEntry
	switch {
	case m >= 3 && m <= 5:
		s = seasonEntry{"春", "spring"}
	case m >= 6 && m <= 8:
		s = seasonEntry{"夏", "summer"}
	case m >= 9 && m <= 11:
		s = seasonEntry{"秋", "autumn"}
	default:
		s = seasonEntry{"冬", "winter"}
	}
	if lang == "en" {
		return s.en
	}
	return s.ja
}

// timeLabelEntry はひとつの時間帯ラベルを保持する。
type timeLabelEntry struct {
	threshold time.Duration
	ja, en    string
}

// timeLabelTable は短い順に並んだ時間帯ラベル定義。
var timeLabelTable = []timeLabelEntry{
	{2 * time.Hour,       "さっき",  "just now"},
	{24 * time.Hour,      "この前",  "earlier today"},
	{48 * time.Hour,      "昨日",    "yesterday"},
	{7 * 24 * time.Hour,  "先日",    "the other day"},
}

var timeLabelFallback = timeLabelEntry{0, "先週", "last week"}
var timeLabelZero     = timeLabelEntry{0, "以前", "before"}

// memoryTimeLabel は ISO 8601 タイムスタンプを相対表現に変換する。
func memoryTimeLabel(timestamp, lang string) string {
	t := types.StrToTime(timestamp)
	if t.IsZero() {
		return pickLabel(timeLabelZero, lang)
	}
	d := time.Since(t)
	for _, e := range timeLabelTable {
		if d < e.threshold {
			return pickLabel(e, lang)
		}
	}
	return pickLabel(timeLabelFallback, lang)
}

func pickLabel(e timeLabelEntry, lang string) string {
	if lang == "en" {
		return e.en
	}
	return e.ja
}

// FrequencyController は発話頻度を制御する。
type FrequencyController struct {
	mu               sync.Mutex
	lastState        types.ContextState
	lastSpeakTime    time.Time
	cooldownUntil    time.Time
	consecutive      int
	lastWebSpeakTime time.Time
	lastImportantAt  time.Time // GitCommit/Push/Success/Fail直後のタイムスタンプ
	lastClickAt      time.Time // UserClick直後のWebBrowsing抑制用
}

// routineInterval は SpeechFrequency に基づく通常イベント（active_edit等）の最小発話間隔を返す。
//
//   - freq=1 (低): 5分   — セリフが希少になり、一言一言が際立つ
//   - freq=2 (中): 3分   — デフォルト。2〜3分に1回程度
//   - freq=3 (高): 90秒  — ユーザーが頻度を望んでいる場合
//   - freq=4 (dev): 15秒  — 開発・テスト用超高頻度モード
func routineInterval(freq int) time.Duration {
	switch freq {
	case 1:
		return 5 * time.Minute
	case 3:
		return 90 * time.Second
	case 4:
		return 15 * time.Second
	default: // 2
		return 3 * time.Minute
	}
}

func NewFrequencyController() *FrequencyController {
	return &FrequencyController{}
}

func (fc *FrequencyController) ShouldSpeak(reason Reason, state types.ContextState, cfg *config.Config, now time.Time) bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	meta := reasonInfo(reason)
	switch {
	case meta.alwaysSpeak:
		return true
	case meta.webCooldown:
		return fc.allowWeb(now)
	}

	// 上記以外は共通の hardFloor を通す
	if !fc.passHardFloor(cfg, now) {
		return false
	}

	switch {
	case meta.isRoutine:
		return fc.allowRoutine(cfg, now)
	default:
		return fc.allowDefault(reason, state, cfg, now)
	}
}

// allowWeb は webCooldown 系 Reason の発話可否を判定する。
// hardFloor をスキップし、UserClick 直後抑制と専用クールダウン（3分）のみで制御する。
func (fc *FrequencyController) allowWeb(now time.Time) bool {
	if !fc.lastClickAt.IsZero() && now.Sub(fc.lastClickAt) < 30*time.Second {
		return false
	}
	if !fc.lastWebSpeakTime.IsZero() && now.Sub(fc.lastWebSpeakTime) < 3*time.Minute {
		return false
	}
	return true
}

// passHardFloor は共通の最小発話間隔チェックを行う。
// 通常30秒、dev超高頻度モード(freq=4)は10秒。
func (fc *FrequencyController) passHardFloor(cfg *config.Config, now time.Time) bool {
	hardFloor := 30 * time.Second
	if cfg.SpeechFrequency == 4 {
		hardFloor = 10 * time.Second
	}
	return fc.lastSpeakTime.IsZero() || now.Sub(fc.lastSpeakTime) >= hardFloor
}

// allowRoutine は isRoutine 系 Reason（active_edit 等）の発話可否を判定する。
// 重要イベント直後2分抑制と SpeechFrequency 連動インターバルで制御する。
func (fc *FrequencyController) allowRoutine(cfg *config.Config, now time.Time) bool {
	const postImportantSuppression = 2 * time.Minute
	if !fc.lastImportantAt.IsZero() && now.Sub(fc.lastImportantAt) < postImportantSuppression {
		return false
	}
	return now.Sub(fc.lastSpeakTime) >= routineInterval(cfg.SpeechFrequency)
}

// allowDefault は個別ロジックを持つ Reason（Success/Fail/ThinkingTick 等）の発話可否を判定する。
func (fc *FrequencyController) allowDefault(reason Reason, state types.ContextState, cfg *config.Config, now time.Time) bool {
	switch reason {
	case ReasonSuccess, ReasonFail:
		return state != fc.lastState
	case ReasonThinkingTick:
		if !cfg.Monologue || now.Before(fc.cooldownUntil) {
			return false
		}
		interval := 10 * time.Minute
		switch cfg.SpeechFrequency {
		case 1:
			interval = 20 * time.Minute
		case 3:
			interval = 5 * time.Minute
		case 4:
			interval = 1 * time.Minute
		}
		return now.Sub(fc.lastSpeakTime) >= interval
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
	if reason == ReasonUserClick {
		fc.lastClickAt = now
	}

	// 重要イベントとして記録（直後の通常発話抑制に使う）
	isImportant := reason == ReasonGitCommit || reason == ReasonGitPush ||
		reason == ReasonSuccess || reason == ReasonFail || reason == ReasonDevSessionStarted
	if isImportant {
		fc.lastImportantAt = now
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

// inferPersonalityType はキャッシュされた currentPersonality を返す。
// Config 固定指定がある場合はそちらを優先。
// 実際の遷移ロジックは updatePersonality で管理する。
func (sg *SpeechGenerator) inferPersonalityType(reason Reason, cfg *config.Config) string {
	// Config で指定されている場合はそれを優先
	if cfg.PersonaStyle == types.StyleGenki || cfg.PersonaStyle == types.StyleCute ||
		cfg.PersonaStyle == types.StyleCool || cfg.PersonaStyle == types.StyleOneesan {
		return string(cfg.PersonaStyle)
	}
	switch cfg.PersonaStyle {
	case types.StyleEnergetic:
		return "genki"
	case types.StyleStrict:
		return "cool"
	case types.StyleSoft:
		return "cute"
	}
	// ToneからPersonalityへのフォールバック（既存設定の後方互換）
	switch cfg.Tone {
	case "genki":
		return "genki"
	case "calm":
		return "cute"
	case "oneesan":
		return "oneesan"
	case "tsundere":
		return "cool"
	}

	return string(sg.personality.Current())
}

// updatePersonality はセリフ生成後に呼ばれ、personality の遷移を管理する。
//
// 遷移ルール:
//   - cool への遷移: failStreak >= 2 / longSession / sameFile多発 / LongInactivity
//     → 解除条件: successStreak >= 2 でcuteに戻す（genki は直接上書きしない）
//   - genki への遷移: success/commit/push イベント時（cool中は遷移しない）
//     → 自動減衰: 3発話後に cute に戻る
//   - cute: デフォルト / セッションリセット時
func (sg *SpeechGenerator) updatePersonality(reason Reason) {
	duration := time.Duration(0)
	if !sg.codingSessionStart.IsZero() {
		duration = time.Since(sg.codingSessionStart)
	}

	ctx := PersonalityContext{
		Reason:            reason,
		FailStreak:        sg.failStreak,
		SuccessStreak:     sg.successStreak,
		SameFileEditCount: sg.sameFileEditCount,
		GenkiSpeechCount:  sg.genkiSpeechCount,
		SessionDuration:   duration,
	}

	next := sg.personality.Update(ctx)

	// genki 減衰カウンタの更新（内部状態との同期）
	if next == PersonalityGenki {
		if isSuccessMilestone(reason) {
			sg.genkiSpeechCount = 0
		} else {
			sg.genkiSpeechCount++
		}
	} else {
		sg.genkiSpeechCount = 0
	}
}

func isSuccessMilestone(r Reason) bool {
	return r == ReasonSuccess || r == ReasonGitCommit || r == ReasonGitPush
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

// traitLabelFromProgress は TraitProgress から LLM に渡すラベル文字列を生成する。
// 複数回答がある場合は全履歴を " / " で結合し、LLMに回答の幅（矛盾含む）を伝える。
func traitLabelFromProgress(prog types.TraitProgress) string {
	// 有効な回答だけを収集（"対象なし" を除く）
	var answers []string
	seen := make(map[string]bool)
	for _, a := range prog.AskedTopics {
		if a != "" && a != "対象なし" && !seen[a] {
			seen[a] = true
			answers = append(answers, a)
		}
	}
	// AskedTopics になくて LastAnswer にある場合は追加
	if prog.LastAnswer != "" && prog.LastAnswer != "対象なし" && !seen[prog.LastAnswer] {
		answers = append(answers, prog.LastAnswer)
	}
	if len(answers) == 0 {
		return ""
	}
	return strings.Join(answers, " / ")
}

func (sg *SpeechGenerator) fallbackSpeech(reason Reason, cfg *config.Config) string {
	lang := cfg.Language
	if lang == "" {
		lang = "ja"
	}
	userName := cfg.UserName
	if userName == "" {
		userName = i18n.T(lang, "speech.default_username")
	}
	substitute := func(text string) string {
		text = strings.ReplaceAll(text, "{{UserName}}", userName)
		text = strings.ReplaceAll(text, "{{username}}", userName)
		text = strings.ReplaceAll(text, "{UserName}", userName)
		text = strings.ReplaceAll(text, "{username}", userName)
		return text
	}

	// greeting系はFallbackSpeech側で時間帯処理があるためそちらに委譲
	if reason == ReasonGreeting || reason == ReasonInitSetup {
		return substitute(FallbackSpeech(reason, lang))
	}

	// 候補リストを全て取得してシャッフルし、重複しない最初の1件を返す
	key := "speech.fallback." + string(reason)
	candidates := i18n.TVariant(lang, key)
	if len(candidates) == 0 || (len(candidates) == 1 && candidates[0] == key) {
		return substitute(FallbackSpeech(reason, lang))
	}

	shuffled := make([]string, len(candidates))
	copy(shuffled, candidates)
	rndMu.Lock()
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rnd.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	rndMu.Unlock()

	var last string
	for _, c := range shuffled {
		text := substitute(c)
		last = text
		if sg.state == nil || !sg.state.IsDuplicate(text) {
			if sg.state != nil {
				sg.state.AddLine(text) // fallbackも重複追跡に追加
			}
			return text
		}
	}
	// 全候補が重複した場合は最後のものを返す（AddLineはしない：全重複時は効果なし）
	return last
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

// reasonToExampleCategory は Reason を few-shot 例文カテゴリに変換する。
func reasonToExampleCategory(r Reason) string {
	switch r {
	case ReasonGitCommit, ReasonGitPush, ReasonGitAdd, ReasonSuccess:
		return "achievement"
	case ReasonFail, ReasonLongInactivity:
		return "struggle"
	case ReasonIdle:
		return "concern"
	case ReasonNightWork:
		return "night_work"
	case ReasonAISessionActive:
		return "ai_pairing"
	case ReasonAISessionStarted:
		return "ai_pairing"
	case ReasonUserClick, ReasonGreeting, ReasonDevSessionStarted, ReasonInitObservation, ReasonInitSupport:
		return "presence"
	default:
		return "working"
	}
}

// betaExamplePersonality は examples/atoms 取得に使う personality を返す。
// 現在は cute ベースで例文品質を統一するため、常に "cute" を返す（betaモード）。
// 各 personality の examples が十分な品質になったら per-personality に戻す。
func betaExamplePersonality(_ string) string {
	return "cute"
}

// pickExampleSpeeches は personality・reason に合った few-shot 例文を選んで返す。
// ニュース/天気/質問系は例文不要なので空を返す。
func pickExampleSpeeches(lang, personality string, reason Reason) []string {
	personality = betaExamplePersonality(personality)
	switch reason {
	case ReasonInitCuriosity, ReasonInitWeather,
		ReasonUserQuestion, ReasonQuestionAnswered:
		return nil
	}

	cat := reasonToExampleCategory(reason)
	catKey := "speech.examples." + personality + "." + cat
	presKey := "speech.examples." + personality + ".presence"

	catExamples := i18n.TVariant(lang, catKey)
	presExamples := i18n.TVariant(lang, presKey)

	rndMu.Lock()
	rnd.Shuffle(len(catExamples), func(i, j int) { catExamples[i], catExamples[j] = catExamples[j], catExamples[i] })
	rnd.Shuffle(len(presExamples), func(i, j int) { presExamples[i], presExamples[j] = presExamples[j], presExamples[i] })
	rndMu.Unlock()

	// category から5件 + presence から3件（重複なし）
	result := make([]string, 0, 8)
	for i, s := range catExamples {
		if i >= 5 { break }
		result = append(result, s)
	}
	for i, s := range presExamples {
		if i >= 3 { break }
		result = append(result, s)
	}
	return result
}

// buildVoiceAtoms はキャラクター固有の声のパーツ（語り出し/気持ち/締め）をランダムに選んでテキスト化する。
// 組み合わせデモを1行添えて small model がパーツの使い方を理解しやすくする。
func buildVoiceAtoms(lang, personality string, reason Reason) string {
	personality = betaExamplePersonality(personality)
	switch reason {
	case ReasonInitCuriosity, ReasonInitWeather,
		ReasonUserQuestion, ReasonQuestionAnswered:
		return ""
	}

	base := "speech.examples." + personality + ".atoms."
	openers := i18n.TVariant(lang, base+"openers")
	feelings := i18n.TVariant(lang, base+"feelings")
	closers := i18n.TVariant(lang, base+"closers")

	if len(openers) == 0 && len(feelings) == 0 && len(closers) == 0 {
		return ""
	}

	rndMu.Lock()
	rnd.Shuffle(len(openers), func(i, j int) { openers[i], openers[j] = openers[j], openers[i] })
	rnd.Shuffle(len(feelings), func(i, j int) { feelings[i], feelings[j] = feelings[j], feelings[i] })
	rnd.Shuffle(len(closers), func(i, j int) { closers[i], closers[j] = closers[j], closers[i] })
	rndMu.Unlock()

	pick := func(list []string, n int) []string {
		if len(list) > n {
			return list[:n]
		}
		return list
	}
	quote := func(items []string) string {
		out := make([]string, len(items))
		for i, s := range items {
			out[i] = `"` + s + `"`
		}
		return strings.Join(out, " / ")
	}

	openerLabel := i18n.T(lang, "speech.atoms_label.openers")
	feelingLabel := i18n.T(lang, "speech.atoms_label.feelings")
	closerLabel  := i18n.T(lang, "speech.atoms_label.closers")
	comboFmt     := i18n.T(lang, "speech.atoms_label.combo_format")

	demo := ""
	if len(openers) > 0 && len(feelings) > 0 && len(closers) > 0 && comboFmt != "" {
		demo = "\n← " + fmt.Sprintf(comboFmt, openers[0], feelings[0], closers[0])
	}

	return fmt.Sprintf("%s: %s\n%s: %s\n%s: %s%s",
		openerLabel, quote(pick(openers, 6)),
		feelingLabel, quote(pick(feelings, 6)),
		closerLabel,  quote(pick(closers, 6)),
		demo)
}

// buildBatchExampleSection は Pool 補充時に使う few-shot 例文セクションを組み立てる。
// personality と category から直接引くので reason の変換を経由しない。
func buildBatchExampleSection(lang, personality, category string) string {
	catKey := "speech.examples." + personality + "." + category
	examples := i18n.TVariant(lang, catKey)
	if len(examples) == 0 {
		return ""
	}

	rndMu.Lock()
	rnd.Shuffle(len(examples), func(i, j int) { examples[i], examples[j] = examples[j], examples[i] })
	rndMu.Unlock()

	if len(examples) > 5 {
		examples = examples[:5]
	}

	header := i18n.T(lang, "speech.batch_examples_header")
	var sb strings.Builder
	sb.WriteString("\n[" + header + "]\n")
	for _, ex := range examples {
		sb.WriteString("- " + ex + "\n")
	}
	return sb.String()
}

// buildNewsPreferences はプロフィールのニュース関心履歴をLLM向けテキストに変換する。
func buildNewsPreferences(prof profile.DevProfile) string {
	ni := prof.NewsInterests
	if len(ni.LikedHeadlines) == 0 && len(ni.DislikedHeadlines) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(ni.LikedHeadlines) > 0 {
		// 最新3件だけ渡す（古すぎる情報は邪魔）
		liked := ni.LikedHeadlines
		if len(liked) > 3 {
			liked = liked[len(liked)-3:]
		}
		sb.WriteString("興味あり: ")
		sb.WriteString(strings.Join(liked, " / "))
	}
	if len(ni.DislikedHeadlines) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		disliked := ni.DislikedHeadlines
		if len(disliked) > 3 {
			disliked = disliked[len(disliked)-3:]
		}
		sb.WriteString("興味なし: ")
		sb.WriteString(strings.Join(disliked, " / "))
	}
	return sb.String()
}
