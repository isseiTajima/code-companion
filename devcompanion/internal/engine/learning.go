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
			le.curiosity[types.TraitDebuggingStyle] += 0.3
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
			le.curiosity[types.TraitFavoriteDrink] += 0.05
		}
		if state == string(types.StateStuck) {
			le.curiosity[types.TraitDebuggingStyle] += 0.35
			le.curiosity[types.TraitStressRelief] += 0.25
			le.curiosity[types.TraitEncouragementStyle] += 0.2
			le.curiosity[types.TraitPraisePreference] += 0.1
		}
		if state == string(types.StateAIPairing) {
			le.curiosity[types.TraitExperimentationStyle] += 0.3
			le.curiosity[types.TraitRiskTolerance] += 0.2
			le.curiosity[types.TraitTechInterest] += 0.15
		}
		if state == string(types.StateIdle) {
			le.curiosity[types.TraitLifeStyle] += 0.1
			le.curiosity[types.TraitHobby] += 0.1
			le.curiosity[types.TraitFoodPreference] += 0.1
			le.curiosity[types.TraitFavoriteSnack] += 0.05
			le.curiosity[types.TraitFavoriteSeason] += 0.05
		}
		if state == string(types.StateProcrastinating) {
			le.curiosity[types.TraitGamePreference] += 0.2
			le.curiosity[types.TraitAnimePreference] += 0.15
			le.curiosity[types.TraitFavoriteWeather] += 0.1
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
	var candidates []types.TraitID
	for trait, score := range le.curiosity {
		progress := prof.Evolution[trait]
		// すでに信頼度が1.0（完了）のものは自動では聞かない
		if progress.Confidence >= 1.0 {
			continue
		}
		// 直近 TraitAnswerCooldown 以内に回答済みのトレイトはスキップ
		if progress.LastUpdated != "" {
			lastAnswered := types.StrToTime(progress.LastUpdated)
			if !lastAnswered.IsZero() && time.Since(lastAnswered) < TraitAnswerCooldown {
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

		// 候補の中からランダムに選択
		bestTrait := candidates[rand.Intn(len(candidates))]

		go le.triggerQuestion(bestTrait)

		le.curiosity[bestTrait] *= 0.1 // 大幅にリセット
		le.lastQuestion = time.Now()
		le.dailyCount++
	}
}

// allTraits は TriggerQuestion でランダム選択する際に使うトレイット全リスト。
var allTraits = []types.TraitID{
	types.TraitFocusStyle, types.TraitFeedbackPreference, types.TraitInterruptionTolerance,
	types.TraitDebuggingStyle, types.TraitCodeReviewStyle, types.TraitExperimentationStyle,
	types.TraitWorkPace, types.TraitBreakStyle, types.TraitDeadlineStyle, types.TraitMultitaskStyle,
	types.TraitThinkingStyle, types.TraitCuriosityLevel, types.TraitRiskTolerance, types.TraitPerfectionism,
	types.TraitLearningStyle, types.TraitMotivationSource, types.TraitTeachingStyle,
	types.TraitWorkspaceStyle, types.TraitNotificationStyle, types.TraitSilencePreference, types.TraitBackgroundNoise,
	types.TraitEncouragementStyle, types.TraitPraisePreference, types.TraitStressRelief, types.TraitAlonePreference,
	types.TraitLifeStyle, types.TraitMusicHabit, types.TraitFoodPreference, types.TraitFavoriteDrink, types.TraitFavoriteSnack,
	types.TraitHobby, types.TraitGamePreference, types.TraitAnimePreference, types.TraitReadingHabit, types.TraitTechInterest,
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
		case "tsukime", "strict":
			return "One thing I wanted to check with you."
		default: // cute, soft
			return "Um... I've been wondering something..."
		}
	}
	switch personality {
	case "genki", "energetic":
		return "ちょっと聞いていいですか！"
	case "tsukime", "strict":
		return "一つ確認させてください。"
	default: // cute, soft
		return "あの…聞きたいんですが…"
	}
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

	// 回答に対するリアクションを生成（回答テキストをコンテキストとして渡す）
	go func(answerText string) {
		time.Sleep(500 * time.Millisecond) // 少し間を置く
		ev := le.dispatcher.LastEvent()
		le.dispatcher.DispatchSpeech("monitor_event", ev, "question_answered", answerText)
	}(text)
}
