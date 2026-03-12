package monitor

import (
	"sakura-kodama/internal/types"
)

// MoodType はキャラクターの気分を表す。
type MoodType string

const (
	MoodHappy     MoodType = "StrongJoy" // 強い喜び (以前のHappy)
	MoodPositive  MoodType = "Positive"  // 軽い喜び
	MoodNeutral   MoodType = "Neutral"   // 通常 (以前のCalm)
	MoodQuiet     MoodType = "Quiet"     // 静かな見守り
	MoodConcerned MoodType = "Concerned" // 心配・困惑 (以前のNervous)
	MoodFocus     MoodType = "Focus"     // 集中
	MoodNegative  MoodType = "Negative"  // 悲しみ・失敗
)

// InferMood はStateやセッション状況からMoodを決定する。
func InferMood(ev MonitorEvent) MoodType {
	// 1. 強力なポジティブイベント
	if ev.State == types.StateSuccess {
		return MoodHappy
	}
	// Git系も喜びとして扱う
	if ev.Event == types.EventGitActivity {
		return MoodHappy
	}

	// 2. ビルド失敗は「心配・困惑」として扱う。
	// Sakura は支え合う後輩エンジニアであり、失敗時は悲観するのではなく
	// ユーザーを心配するリアクション（MoodConcerned）が自然。
	if ev.State == types.StateFail {
		return MoodConcerned
	}

	// 3. セッション状態に基づく判定
	switch ev.Session.Mode {
	case types.ModeDeepFocus:
		return MoodQuiet
	case types.ModeProductiveFlow:
		return MoodPositive
	case types.ModeStruggling:
		return MoodConcerned
	}

	// 4. 開発状態に基づく判定
	switch ev.State {
	case types.StateCoding:
		// 通常のコーディング中は Neutral だが、タスクによって変化
		if ev.Task == TaskGenerateCode || ev.Task == TaskDebug {
			return MoodFocus
		}
		return MoodNeutral
	case types.StateThinking:
		return MoodFocus
	}

	return MoodNeutral
}
