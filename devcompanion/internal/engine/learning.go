package engine

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

const (
	CuriosityThreshold  = 0.8
	CuriosityDecay      = 0.98 // per minute
	MinQuestionInterval = 1 * time.Hour
	MaxQuestionsPerDay  = 3
	ContextWindow       = 1 * time.Hour
	// 同じトレイトに回答してからこの時間は再び聞かない
	TraitAnswerCooldown = 12 * time.Hour
)

// LearningEngine は Sakura の好奇心と性格学習を管理する。
//
// 不変条件:
//   - profileStore と dispatcher は nil であってはならない
//   - cfg は nil であってはならない
//   - curiosity は nil であってはならない
//   - dailyCount は 0 以上 MaxQuestionsPerDay 以下
type LearningEngine struct {
	mu          sync.Mutex
	curiosity   map[types.TraitID]float64
	lastQuestion time.Time
	dailyCount  int
	lastDecay   time.Time

	profileStore *profile.ProfileStore
	dispatcher   SpeechDispatcher
	cfg          *config.Config
}

// NewLearningEngine は LearningEngine を作成する。
//
// 事前条件:
//   - ps は nil であってはならない
//   - d は nil であってはならない
//   - cfg は nil であってはならない
func NewLearningEngine(ps *profile.ProfileStore, d SpeechDispatcher, cfg *config.Config) *LearningEngine {
	if ps == nil {
		panic("engine: NewLearningEngine: profileStore must not be nil")
	}
	if d == nil {
		panic("engine: NewLearningEngine: dispatcher must not be nil")
	}
	if cfg == nil {
		panic("engine: NewLearningEngine: cfg must not be nil")
	}
	return &LearningEngine{
		curiosity:    make(map[types.TraitID]float64),
		lastDecay:    time.Now(),
		profileStore: ps,
		dispatcher:   d,
		cfg:          cfg,
	}
}

// UpdateConfig は設定を更新する。Engine.UpdateConfig から呼ばれる。
// 事前条件: cfg は nil であってはならない。
func (le *LearningEngine) UpdateConfig(cfg *config.Config) {
	if cfg == nil {
		panic("engine: LearningEngine.UpdateConfig: cfg must not be nil")
	}
	le.mu.Lock()
	defer le.mu.Unlock()
	le.cfg = cfg
}

// ProcessEvent はイベントに基づいて好奇心スコアを更新する。
func (le *LearningEngine) ProcessEvent(ev types.Event) {
	le.mu.Lock()
	defer le.mu.Unlock()

	// 1. Decay all traits
	now := time.Now()
	minutes := now.Sub(le.lastDecay).Minutes()
	if minutes >= 1.0 {
		for trait := range le.curiosity {
			le.curiosity[trait] *= CuriosityDecay
		}
		le.lastDecay = now
	}

	// 2. Trait-specific boosts
	switch ev.Type {
	case "monitor_event":
		state := ev.Payload["state"].(string)
		if state == string(types.StateSuccess) {
			le.curiosity[types.TraitFeedbackPreference] += 0.2
			le.curiosity[types.TraitPerfectionism] += 0.15
			le.curiosity[types.TraitMotivationSource] += 0.1
		}
		if state == string(types.StateFail) {
			le.curiosity[types.TraitFeedbackPreference] += 0.2
			le.curiosity[types.TraitStressRelief] += 0.15
			le.curiosity[types.TraitEncouragementStyle] += 0.1
		}
		if state == string(types.StateDeepWork) {
			le.curiosity[types.TraitInterruptionTolerance] += 0.25
			le.curiosity[types.TraitSilencePreference] += 0.2
			le.curiosity[types.TraitMusicHabit] += 0.15
			le.curiosity[types.TraitAlonePreference] += 0.1
		}
		if state == string(types.StateCoding) {
			le.curiosity[types.TraitFocusStyle] += 0.15
			le.curiosity[types.TraitBreakStyle] += 0.1
			le.curiosity[types.TraitMultitaskStyle] += 0.1
			le.curiosity[types.TraitFavoriteDrink] += 0.1  // コーディング中の飲み物
			le.curiosity[types.TraitMusicHabit] += 0.12    // 作業BGM
			le.curiosity[types.TraitFavoriteSnack] += 0.08 // 間食
			le.curiosity[types.TraitHobby] += 0.07         // 気分転換
		}
		if state == string(types.StateStuck) {
			le.curiosity[types.TraitDebuggingStyle] += 0.35
			le.curiosity[types.TraitStressRelief] += 0.25
			le.curiosity[types.TraitEncouragementStyle] += 0.2
			le.curiosity[types.TraitPraisePreference] += 0.1
			le.curiosity[types.TraitHobby] += 0.1          // 詰まったときの気晴らし
		}
		if state == string(types.StateAIPairing) {
			// AI と組んでいる → 好奇心旺盛・チャレンジ精神を個人性格として聞く
			le.curiosity[types.TraitRiskTolerance] += 0.2  // 新しいこと好き？慎重派？
			le.curiosity[types.TraitCuriosityLevel] += 0.15
		}
		if state == string(types.StateIdle) {
			le.curiosity[types.TraitLifeStyle] += 0.15
			le.curiosity[types.TraitHobby] += 0.15
			le.curiosity[types.TraitFoodPreference] += 0.12
			le.curiosity[types.TraitFavoriteSnack] += 0.1
			le.curiosity[types.TraitFavoriteSeason] += 0.1
			le.curiosity[types.TraitConversationStyle] += 0.1
			le.curiosity[types.TraitReadingHabit] += 0.08
		}
		if state == string(types.StateProcrastinating) {
			le.curiosity[types.TraitGamePreference] += 0.2
			le.curiosity[types.TraitAnimePreference] += 0.15
			le.curiosity[types.TraitFavoriteWeather] += 0.1
			le.curiosity[types.TraitHobby] += 0.15
		}
		if state == string(types.StateDeepWork) {
			// すでに上でブーストしているが、personal 系も追加
			le.curiosity[types.TraitFavoriteDrink] += 0.1 // 集中中の飲み物
			le.curiosity[types.TraitLifeStyle] += 0.08    // 夜型？朝型？
		}
		if state == string(types.StateSuccess) {
			// 既存に追加
			le.curiosity[types.TraitHobby] += 0.08        // 達成後の楽しみ
			le.curiosity[types.TraitFavoriteSnack] += 0.07
		}
	case "observation_event":
		obsType := ev.Payload["type"].(string)
		if obsType == "git_commit" || obsType == "git_push" {
			le.curiosity[types.TraitWorkPace] += 0.25
			le.curiosity[types.TraitDeadlineStyle] += 0.15
			le.curiosity[types.TraitPerfectionism] += 0.1
		}
		if obsType == "idle_start" {
			le.curiosity[types.TraitHobby] += 0.2
			le.curiosity[types.TraitBreakStyle] += 0.15
			le.curiosity[types.TraitStressRelief] += 0.1
			le.curiosity[types.TraitConversationStyle] += 0.1
			le.curiosity[types.TraitReadingHabit] += 0.05
		}
	}

	le.evaluateTrigger()
}

func (le *LearningEngine) evaluateTrigger() {
	if le.dailyCount >= MaxQuestionsPerDay || time.Since(le.lastQuestion) < MinQuestionInterval {
		return
	}

	prof := le.profileStore.Get()

	// 閾値を超えている、かつ再質問可能なトレイトをリストアップ
	relationshipLevel := prof.Relationship.Level
	var candidates []types.TraitID
	for trait, score := range le.curiosity {
		// プライバシーレベルによる解放制御
		if privLevel, ok := traitPrivacy[trait]; ok {
			if relationshipLevel < int(privLevel) {
				continue
			}
		}
		progress := prof.Evolution[trait]
		// すでに信頼度が1.0（完了）のものは自動では聞かない
		if progress.Confidence >= 1.0 {
			continue
		}
		// 直近 TraitAnswerCooldown 以内に質問済み or 回答済みのトレイトはスキップ
		// LastAsked: 質問発火時刻（スキップしても記録）
		// LastUpdated: 回答時刻
		lastRef := progress.LastAsked
		if progress.LastUpdated > lastRef {
			lastRef = progress.LastUpdated
		}
		if lastRef != "" {
			lastTime := types.StrToTime(lastRef)
			if !lastTime.IsZero() && time.Since(lastTime) < TraitAnswerCooldown {
				continue
			}
		}
		if score >= CuriosityThreshold {
			candidates = append(candidates, trait)
		}
	}

	if len(candidates) > 0 {
		lastEv := le.dispatcher.LastEvent()
		if lastEv.State == types.StateDeepWork {
			return
		}

		// personal/hobby/lifestyle 系を dev 系より優先してウェイト付き選択する。
		// dev 系ばかり溜まりやすい環境でもプライベートな質問が混ざるようにする。
		bestTrait := weightedTraitSelect(candidates)

		go le.triggerQuestion(bestTrait)

		le.curiosity[bestTrait] *= 0.1 // 大幅にリセット
		le.lastQuestion = time.Now()
		le.dailyCount++
	}
}

// personalTraitWeight は personal/hobby/lifestyle 系 trait のウェイト（dev 系は 1）。
// 高いほど選ばれやすくなり、技術質問偏重を緩和する。
var personalTraitWeight = map[types.TraitID]int{
	types.TraitHobby:                 4,
	types.TraitGamePreference:        4,
	types.TraitAnimePreference:       4,
	types.TraitReadingHabit:          4,
	types.TraitFoodPreference:        3,
	types.TraitFavoriteDrink:         3,
	types.TraitFavoriteSnack:         3,
	types.TraitMusicHabit:            3,
	types.TraitLifeStyle:             3,
	types.TraitFavoriteSeason:        3,
	types.TraitFavoriteWeather:       3,
	types.TraitPersonalityAttraction: 4,
	types.TraitConversationStyle:     3,
	types.TraitCommunicationStyle:    2,
	types.TraitStressRelief:          2,
	types.TraitAlonePreference:       2,
	types.TraitEncouragementStyle:    2,
	types.TraitPraisePreference:      2,
	types.TraitMotivationSource:      2,
}

// weightedTraitSelect はウェイト付きランダムで trait を選択する。
func weightedTraitSelect(candidates []types.TraitID) types.TraitID {
	total := 0
	for _, t := range candidates {
		w := 1
		if v, ok := personalTraitWeight[t]; ok {
			w = v
		}
		total += w
	}
	r := rand.Intn(total)
	for _, t := range candidates {
		w := 1
		if v, ok := personalTraitWeight[t]; ok {
			w = v
		}
		r -= w
		if r < 0 {
			return t
		}
	}
	return candidates[len(candidates)-1]
}

// allTraits は TriggerQuestion でランダム選択する際に使うトレイット全リスト。
// 技術系（debugging_style, code_review_style, experiment_style, tech_interest）と
// 開発/働き方系（focus_style, feedback_preference, interruption_tolerance, work_pace,
// break_style, deadline_style, multitask_style, risk_tolerance）は
// 「技術のことわからない後輩」キャラ設定と合わず、セリフ生成にも役立たないため除外。
var allTraits = []types.TraitID{
	types.TraitThinkingStyle, types.TraitCuriosityLevel, types.TraitPerfectionism,
	types.TraitLearningStyle, types.TraitMotivationSource, types.TraitTeachingStyle,
	types.TraitWorkspaceStyle, types.TraitNotificationStyle, types.TraitSilencePreference, types.TraitBackgroundNoise,
	types.TraitEncouragementStyle, types.TraitPraisePreference, types.TraitStressRelief, types.TraitAlonePreference,
	types.TraitLifeStyle, types.TraitMusicHabit, types.TraitFoodPreference, types.TraitFavoriteDrink, types.TraitFavoriteSnack,
	types.TraitHobby, types.TraitGamePreference, types.TraitAnimePreference, types.TraitReadingHabit,
	types.TraitCommunicationStyle, types.TraitConversationStyle, types.TraitPersonalityAttraction,
	types.TraitFavoriteSeason, types.TraitFavoriteWeather,
}

// TriggerQuestion は指定した特性に関する質問を強制的に発生させる（デバッグ・テスト用）。
// traitID が空の場合はランダムに選択する。
func (le *LearningEngine) TriggerQuestion(traitID string) {
	trait := types.TraitID(traitID)
	if trait == "" {
		trait = allTraits[rand.Intn(len(allTraits))]
	}
	go le.triggerQuestion(trait)
}

func (le *LearningEngine) triggerQuestion(trait types.TraitID) {
	le.mu.Lock()
	cfg := le.cfg
	le.mu.Unlock()

	prof := le.profileStore.Get()
	progress := prof.Evolution[trait]

	// 直近1時間の行動をサマリー（Stage 2 用）
	recentBehavior := ""
	if progress.CurrentStage == 2 {
		lastEv := le.dispatcher.LastEvent()
		recentBehavior = fmt.Sprintf("State: %s, Task: %s", lastEv.State, lastEv.Task)
	}

	q, err := le.dispatcher.GenerateQuestion(cfg.UserName, trait, progress, recentBehavior, cfg.Language)
	if err != nil {
		fmt.Printf("[LEARNING] Failed to generate question: %v\n", err)
		return
	}

	fmt.Printf("[LEARNING] Sakura is curious about %s (Stage %d) because of recent activity\n", trait, progress.CurrentStage)

	// 質問発火時刻を記録（スキップされても12時間は再度聞かない）
	le.profileStore.RecordTraitAsked(trait)

	le.dispatcher.DispatchEvent(types.Event{
		Type: "question_event",
		Payload: map[string]interface{}{
			"trait_id": string(q.TraitID),
			"preamble": preambleForPersonality(string(cfg.PersonaStyle), cfg.Language),
			"question": q.Text,
			"options":  q.Options,
		},
	})
}

func preambleForPersonality(personality, lang string) string {
	if lang == "en" {
		switch personality {
		case "genki", "energetic":
			return "Hey, can I ask you something real quick?"
		case "cool", "strict":
			return "One thing I wanted to check with you."
		default: // cute, soft
			return "Um... I've been wondering something..."
		}
	}
	switch personality {
	case "genki", "energetic":
		return "ちょっと聞いていいですか！"
	case "cool", "strict":
		return "一つ確認させてください。"
	default: // cute, soft
		return "あの…聞きたいんですが…"
	}
}

// traitPrivacy は各トレイトの開示レベルを定義する。未登録はLow（0）扱い。
var traitPrivacy = map[types.TraitID]types.TraitPrivacyLevel{
	// Medium: メンタル・ストレス・内面
	types.TraitEncouragementStyle: types.TraitPrivacyMedium,
	types.TraitPraisePreference:   types.TraitPrivacyMedium,
	types.TraitStressRelief:       types.TraitPrivacyMedium,
	types.TraitAlonePreference:    types.TraitPrivacyMedium,
	types.TraitMotivationSource:   types.TraitPrivacyMedium,
	// High: 人間関係・パーソナルアトラクション
	types.TraitPersonalityAttraction: types.TraitPrivacyHigh,
}

// traitTags は各トレイトのメモリタグを定義する。
var traitTags = map[types.TraitID][]string{
	types.TraitFocusStyle:            {"dev", "focus"},
	types.TraitFeedbackPreference:    {"dev", "feedback"},
	types.TraitInterruptionTolerance: {"dev", "focus"},
	types.TraitDebuggingStyle:        {"dev", "debugging"},
	types.TraitCodeReviewStyle:       {"dev"},
	types.TraitExperimentationStyle:  {"dev", "curiosity"},
	types.TraitWorkPace:              {"workstyle", "rhythm"},
	types.TraitBreakStyle:            {"workstyle", "rhythm"},
	types.TraitDeadlineStyle:         {"workstyle"},
	types.TraitMultitaskStyle:        {"workstyle"},
	types.TraitThinkingStyle:         {"thinking"},
	types.TraitCuriosityLevel:        {"thinking", "curiosity"},
	types.TraitRiskTolerance:         {"thinking"},
	types.TraitPerfectionism:         {"thinking"},
	types.TraitLearningStyle:         {"learning"},
	types.TraitMotivationSource:      {"learning", "mental"},
	types.TraitTeachingStyle:         {"learning"},
	types.TraitWorkspaceStyle:        {"environment"},
	types.TraitNotificationStyle:     {"environment"},
	types.TraitSilencePreference:     {"environment"},
	types.TraitBackgroundNoise:       {"environment"},
	types.TraitEncouragementStyle:    {"mental"},
	types.TraitPraisePreference:      {"mental"},
	types.TraitStressRelief:          {"mental", "stress"},
	types.TraitAlonePreference:       {"mental"},
	types.TraitLifeStyle:             {"lifestyle"},
	types.TraitMusicHabit:            {"lifestyle", "music"},
	types.TraitFoodPreference:        {"lifestyle", "food"},
	types.TraitFavoriteDrink:         {"lifestyle", "food"},
	types.TraitFavoriteSnack:         {"lifestyle", "food"},
	types.TraitHobby:                 {"hobby"},
	types.TraitGamePreference:        {"hobby", "game"},
	types.TraitAnimePreference:       {"hobby", "anime"},
	types.TraitReadingHabit:          {"hobby", "reading"},
	types.TraitTechInterest:          {"dev", "curiosity"},
	types.TraitCommunicationStyle:    {"communication"},
	types.TraitConversationStyle:     {"communication"},
	types.TraitPersonalityAttraction: {"personal", "relationship"},
	types.TraitFavoriteSeason:        {"lifestyle"},
	types.TraitFavoriteWeather:       {"lifestyle"},
}

// HandleAnswer はユーザーの回答を処理し、プロファイルを更新してリアクションを生成する。
//
// 事前条件:
//   - trait は空文字列であってはならない
//   - optionIndex は -1 以上 2 以下（-1 = 対象なし/自由入力/スキップ）
func (le *LearningEngine) HandleAnswer(trait types.TraitID, optionIndex int, text string) {
	if trait == "" {
		panic("engine: LearningEngine.HandleAnswer: trait must not be empty")
	}
	if optionIndex < -1 || optionIndex > 2 {
		panic(fmt.Sprintf("engine: LearningEngine.HandleAnswer: optionIndex must be -1..2, got %d", optionIndex))
	}

	// optionIndex == -1 は「対象なし/自由入力」: プロファイル更新はスキップ
	if optionIndex >= 0 {
		le.profileStore.RecordTraitUpdate(trait, float64(optionIndex)/2.0, text)
	}

	// PersonalMemory に記録（「対象なし」以外の有効な回答のみ）
	if text != "" && text != "対象なし" {
		source := "question_answer"
		if optionIndex < 0 {
			source = "free_text"
		}
		tags := traitTags[trait]
		if tags == nil {
			tags = []string{}
		}
		le.profileStore.RecordPersonalMemory(types.PersonalMemory{
			TraitID:   string(trait),
			Content:   text,
			Tags:      tags,
			CreatedAt: types.TimeToStr(time.Now()),
			Source:    source,
		})
	}

	// 回答に対するリアクションを生成（回答テキストをコンテキストとして渡す）
	go func(answerText string) {
		time.Sleep(500 * time.Millisecond) // 少し間を置く
		ev := le.dispatcher.LastEvent()
		le.dispatcher.DispatchSpeech("monitor_event", ev, "question_answered", answerText)
	}(text)
}
