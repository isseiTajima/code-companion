package llm

import (
	"sakura-kodama/internal/config"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

// Speaker はセリフを生成するインターフェース。
// SpeechGenerator の具体実装に依存せずにテストやモックを可能にする。
//
// 事前条件（各実装が保証すること）:
//   - cfg は nil であってはならない
//   - reason は空文字列であってはならない
type Speaker interface {
	// Generate はイベントと設定に基づいてセリフを生成する。
	// 戻り値: (speech, prompt, backend) — speech が空の場合は発話しない。
	Generate(e monitor.MonitorEvent, cfg *config.Config, reason Reason, prof profile.DevProfile, question string) (string, string, string)

	// OnUserClick はユーザークリック時のセリフを生成する。
	OnUserClick(e monitor.MonitorEvent, cfg *config.Config, prof profile.DevProfile) (string, string, string)

	// OnUserQuestion はユーザーからの直接質問に応答するセリフを生成する。
	OnUserQuestion(e monitor.MonitorEvent, cfg *config.Config, prof profile.DevProfile, question string) (string, string, string)

	// GenerateQuestion は性格学習用の質問を生成する。
	GenerateQuestion(userName string, trait types.TraitID, progress types.TraitProgress, recentBehavior string, language string) (types.Question, error)

	// IsUsingFallback はフォールバックセリフを使用中かを返す。
	IsUsingFallback() bool

	// UpdateLLMConfig はLLM設定を更新する。
	UpdateLLMConfig(cfg *config.Config)

	// IsAvailable は指定バックエンドが利用可能かを返す。
	IsAvailable(backend string) bool
}

// SpeechGenerator が Speaker を実装していることをコンパイル時に検証する。
var _ Speaker = (*SpeechGenerator)(nil)
