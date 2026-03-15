package types

import "time"

// --- Signal Layer ---

type SignalSource string

const (
	SourceProcess SignalSource = "process"
	SourceFS      SignalSource = "filesystem"
	SourceGit     SignalSource = "git"
	SourceAgent   SignalSource = "agent"
	SourceSystem  SignalSource = "system"
	SourceWeb     SignalSource = "web"
)

type SignalType string

const (
	SigProcessStarted   SignalType = "process_started"
	SigProcessStopped   SignalType = "process_stopped"
	SigFileModified     SignalType = "file_modified"
	SigManyFilesChanged SignalType = "many_files_changed"
	SigGitCommit        SignalType = "git_commit"
	SigLogHint          SignalType = "log_hint"
	SigIdleStart        SignalType = "idle_start"
	SigSystemWake       SignalType = "system_wake"
	SigSystemSleep      SignalType = "system_sleep"
	SigWebNavigated     SignalType = "web_navigated"
)

type Signal struct {
	Type      SignalType   `json:"type"`
	Source    SignalSource `json:"source"`
	Value     string       `json:"value"`
	Message   string       `json:"message"`
	Timestamp string       `json:"timestamp"`
}

// --- Behavior Layer ---

type BehaviorType string

const (
	BehaviorCoding          BehaviorType = "coding"
	BehaviorDebugging       BehaviorType = "debugging"
	BehaviorResearching     BehaviorType = "researching"
	BehaviorAIPairing       BehaviorType = "ai_pair_programming"
	BehaviorRefactoring     BehaviorType = "refactoring"
	BehaviorBreak           BehaviorType = "break"
	BehaviorProcrastinating BehaviorType = "procrastinating"
	BehaviorUnknown         BehaviorType = "unknown"
)

type Behavior struct {
	Type      BehaviorType `json:"type"`
	StartTime string       `json:"start_time"`
	EndTime   string       `json:"end_time"`
	Score     float64      `json:"score"`
}

// --- Session Layer ---

type Mode string

const (
	ModeDeepFocus     Mode = "deep_focus"
	ModeProductiveFlow Mode = "productive_flow"
	ModeStruggling     Mode = "struggling"
	ModeCasualWork     Mode = "casual_work"
	ModeOnBreak        Mode = "on_break"
	ModeIdle           Mode = "idle"
)

type SessionState struct {
	Mode           Mode      `json:"mode"`
	StartTime      string    `json:"start_time"`
	LastActivity   string    `json:"last_activity"`
	FocusLevel     float64   `json:"focus_level"`
	ProgressScore  int       `json:"progress_score"`
}

// --- Context Layer ---

type ContextState string

const (
	StateIdle             ContextState = "IDLE"
	StateCoding           ContextState = "CODING"
	StateAIPairing        ContextState = "AI_PAIR_PROGRAMMING"
	StateDeepWork         ContextState = "DEEP_WORK"
	StateStuck            ContextState = "STUCK"
	StateProcrastinating  ContextState = "PROCRASTINATING"
	StateSessionEnding    ContextState = "SESSION_ENDING"
	StateThinking         ContextState = "THINKING"
	StateSuccess          ContextState = "SUCCESS"
	StateFail             ContextState = "FAIL"
)

type ContextInfo struct {
	State      ContextState `json:"state"`
	Confidence float64      `json:"confidence"`
	StartTime  string       `json:"start_time"`
	LastSignal string       `json:"last_signal"`
}

type ContextDecision struct {
	State      ContextState `json:"state"`
	Confidence float64      `json:"confidence"`
	Signals    []SignalType `json:"signals"`
	Reasons    []string     `json:"reasons"`
}

// --- Event Layer ---

type HighLevelEvent string

const (
	EventAISessionStarted       HighLevelEvent = "AI_SESSION_STARTED"
	EventAISessionActive        HighLevelEvent = "AI_SESSION_ACTIVE"
	EventDevSessionStarted      HighLevelEvent = "DEV_SESSION_STARTED"
	EventDevEditing             HighLevelEvent = "DEV_EDITING"
	EventGitActivity            HighLevelEvent = "GIT_ACTIVITY"
	EventProductiveToolActivity HighLevelEvent = "PRODUCTIVE_TOOL_ACTIVITY"
	EventDocWriting             HighLevelEvent = "DOC_WRITING"
	EventLongInactivity         HighLevelEvent = "LONG_INACTIVITY"
	EventWebBrowsing            HighLevelEvent = "WEB_BROWSING"
)

// Event はシステム全体で流通する汎用的なイベント構造体。
type Event struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// --- Persona Layer ---

type PersonaStyle string

const (
	StyleSoft      PersonaStyle = "soft"
	StyleEnergetic PersonaStyle = "energetic"
	StyleStrict    PersonaStyle = "strict"
	StyleGenki     PersonaStyle = "genki"
	StyleCute      PersonaStyle = "cute"
	StyleTsukime   PersonaStyle = "tsukime"
)

type RelationshipMode string

const (
	RelationshipNormal RelationshipMode = "normal"
	RelationshipLover  RelationshipMode = "lover"
)

// --- Inner State Layer ---

type EmotionState string

const (
	EmotionSupportive EmotionState = "supportive"
	EmotionExcited    EmotionState = "excited"
	EmotionQuiet      EmotionState = "quiet"
	EmotionConcerned  EmotionState = "concerned"
)

// WorldModel represents Sakura's interpretation of the developer's situation.
type WorldModel struct {
	IsDeepWork      bool      `json:"is_deep_work"`
	StrugglingLevel float64   `json:"struggling_level"` // 0.0 - 1.0
	Momentum        float64   `json:"momentum"`         // 0.0 - 1.0
	LastActive      string    `json:"last_active"`
}

// ProjectMoment stores a significant milestone or event in the project history.
type ProjectMoment struct {
	Type      string `json:"type"` // milestone, struggle, success
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// PersonalMemory はユーザーが質問回答・自由入力で語った個人的な情報を記録する。
// サクラが後日「この前〇〇って言ってましたよね」と自然に引用するために使う。
type PersonalMemory struct {
	TraitID   string   `json:"trait_id,omitempty"` // 元となったtrait（質問回答時）
	Content   string   `json:"content"`             // 回答・発言テキスト
	Tags      []string `json:"tags"`                // カテゴリタグ e.g. ["hobby", "stress"]
	CreatedAt string   `json:"created_at"`
	Source    string   `json:"source"` // "question_answer" | "free_text"
}

// TraitPrivacyLevel は各 TraitID の開示レベルを定義する。
// relationship_level がこの値を超えると質問対象になる。
type TraitPrivacyLevel int

const (
	TraitPrivacyLow    TraitPrivacyLevel = 0  // 誰にでも聞ける: 開発スタイル・趣味
	TraitPrivacyMedium TraitPrivacyLevel = 30 // ある程度親しい: メンタル・ストレス解消法
	TraitPrivacyHigh   TraitPrivacyLevel = 60 // 親密: 好きな人のタイプ等
)

// --- Proactive Layer ---

type InitiativeType string

const (
	InitObservation InitiativeType = "observation"
	InitSupport     InitiativeType = "support"
	InitCuriosity   InitiativeType = "curiosity"
	InitMemory      InitiativeType = "memory"
)

type InitiativeState struct {
	LastTime   string         `json:"last_time"`
	LastType   InitiativeType `json:"last_type"`
	DailyCount int            `json:"daily_count"`
}

// --- Personality Learning Layer ---

type TraitCategory string

const (
	CategoryDevelopment TraitCategory = "development"
	CategoryHobby       TraitCategory = "hobby"
	CategoryWorkStyle   TraitCategory = "workstyle"
)

type TraitID string

const (
	// --- 🧑‍💻 開発スタイル ---
	TraitFocusStyle            TraitID = "focus_style"            // 集中型 vs こまめ休憩
	TraitFeedbackPreference    TraitID = "feedback_preference"    // 厳しめ vs 優しめ
	TraitInterruptionTolerance TraitID = "interruption_tolerance" // 割り込み耐性
	TraitDebuggingStyle        TraitID = "debugging_style"        // 仮説型 vs 総当り
	TraitCodeReviewStyle       TraitID = "code_review_style"      // 厳密 vs ラフ
	TraitExperimentationStyle  TraitID = "experiment_style"       // 新技術試す派か否か

	// --- ⏱ 作業リズム ---
	TraitWorkPace      TraitID = "work_pace"      // 朝型・夜型
	TraitBreakStyle    TraitID = "break_style"    // 短休憩 vs 長休憩
	TraitDeadlineStyle TraitID = "deadline_style" // 早め vs ギリギリ
	TraitMultitaskStyle TraitID = "multitask_style" // 並行作業可否

	// --- 🧠 思考スタイル ---
	TraitThinkingStyle  TraitID = "thinking_style"  // 直感型 vs 論理型
	TraitCuriosityLevel TraitID = "curiosity_level" // 新しいもの好き度
	TraitRiskTolerance  TraitID = "risk_tolerance"  // 新ツール試す度
	TraitPerfectionism  TraitID = "perfectionism"   // 完璧主義度

	// --- 📚 学習・成長 ---
	TraitLearningStyle    TraitID = "learning_style"   // 本・動画・実践
	TraitMotivationSource TraitID = "motivation_source" // 好奇心・成果・締め切り
	TraitTeachingStyle    TraitID = "teaching_style"   // 人に教えるタイプか

	// --- 🖥 作業環境 ---
	TraitWorkspaceStyle    TraitID = "workspace_style"    // デスク整理度
	TraitNotificationStyle TraitID = "notification_style" // 通知多い vs 少ない
	TraitSilencePreference TraitID = "silence_preference" // 静かな環境好き
	TraitBackgroundNoise   TraitID = "background_noise"   // カフェ作業など

	// --- ❤️ モチベーション・メンタル ---
	TraitEncouragementStyle TraitID = "encouragement_style" // 励まし方の好み
	TraitPraisePreference   TraitID = "praise_preference"   // 褒められたい度
	TraitStressRelief       TraitID = "stress_relief"       // ストレス解消方法
	TraitAlonePreference    TraitID = "alone_preference"    // 一人作業好き度

	// --- ☕ ライフスタイル ---
	TraitLifeStyle      TraitID = "lifestyle"       // 生活リズム・習慣
	TraitMusicHabit     TraitID = "music_habit"     // 作業中の音楽習慣
	TraitFoodPreference TraitID = "food_preference" // 飲食の好み全般
	TraitFavoriteDrink  TraitID = "favorite_drink"  // よく飲むもの
	TraitFavoriteSnack  TraitID = "favorite_snack"  // 作業中のおやつ

	// --- 🎮 趣味・カルチャー ---
	TraitHobby           TraitID = "hobby"            // 趣味・休日の過ごし方
	TraitGamePreference  TraitID = "game_preference"  // ゲームの好み
	TraitAnimePreference TraitID = "anime_preference" // アニメ・漫画の好み
	TraitReadingHabit    TraitID = "reading_habit"    // 読書習慣
	TraitTechInterest    TraitID = "tech_interest"    // 気になっている技術・分野

	// --- 🧑‍🤝‍🧑 人間関係 ---
	TraitCommunicationStyle    TraitID = "communication_style"    // コミュニケーションスタイル
	TraitConversationStyle     TraitID = "conversation_style"     // 話すの好き vs 聞くの好き
	TraitPersonalityAttraction TraitID = "personality_attraction" // 好きな人のタイプ・性格

	// --- 🌤 雑談トリガー ---
	TraitFavoriteSeason  TraitID = "favorite_season"  // 好きな季節
	TraitFavoriteWeather TraitID = "favorite_weather" // 好きな天気
)

// UserPersonality stores the learned traits (0.0 - 1.0).
type UserPersonality struct {
	Traits map[TraitID]float64 `json:"traits"`
}

// TraitProgress tracks the evolution stage of a specific trait.
type TraitProgress struct {
	CurrentStage int       `json:"current_stage"` // 0, 1, 2
	Confidence   float64   `json:"confidence"`    // 0.0 - 1.0
	LastAnswer   string    `json:"last_answer"`
	AskedTopics  []string  `json:"asked_topics"`
	LastUpdated  string    `json:"last_updated"` // 鮮度管理
}

// Question represents a personality question generated by LLM.
type Question struct {
	TraitID  TraitID  `json:"trait_id"`
	Preamble string   `json:"preamble"`
	Text     string   `json:"question"`
	Options  []string `json:"options"`
}

// TimeToStr converts time.Time to RFC3339 string.
func TimeToStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// StrToTime parses RFC3339 string back to time.Time.
func StrToTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
