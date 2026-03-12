package engine

import (
	"sakura-kodama/internal/llm"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/types"
)

// SpeechDispatcher はサブエンジン（LearningEngine, ProactiveEngine）が
// セリフを生成・通知するための最小インターフェース。
//
// このインターフェースにより、LearningEngine と ProactiveEngine は
// Engine 全体への参照を持たず、必要な操作のみを通じてエンジンと協調できる。
// Engine 側で循環参照なく実装される。
//
// 事前条件:
//   - reason は空文字列であってはならない
//   - event.Type は空文字列であってはならない
type SpeechDispatcher interface {
	// DispatchSpeech はセリフを生成し、指定タイプのイベントとして通知する。
	// セリフが生成されなかった場合（クールダウン中など）は通知しない。
	// 事前条件: reason は空文字列であってはならない。
	DispatchSpeech(eventType string, ev monitor.MonitorEvent, reason llm.Reason, question string)

	// DispatchEvent はセリフ生成なしにイベントを直接通知する。
	// 事前条件: event.Type は空文字列であってはならない。
	DispatchEvent(event types.Event)

	// GenerateQuestion は性格学習用の質問を生成する。
	GenerateQuestion(userName string, trait types.TraitID, progress types.TraitProgress, behavior, lang string) (types.Question, error)

	// LastEvent は最後に処理したモニタリングイベントを返す。
	LastEvent() monitor.MonitorEvent

	// WorldState は現在の世界モデルと感情状態を返す。
	WorldState() (types.WorldModel, types.EmotionState)
}
