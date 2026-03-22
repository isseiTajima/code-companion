package monitor

import (
	"math/rand"
	"sakura-kodama/internal/types"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// MoodType はキャラクターの気分を表す。
type MoodType string

const (
	MoodHappy     MoodType = "StrongJoy" // 強い喜び
	MoodPositive  MoodType = "Positive"  // 軽い喜び
	MoodNeutral   MoodType = "Neutral"   // 通常
	MoodQuiet     MoodType = "Quiet"     // 静かな見守り
	MoodConcerned MoodType = "Concerned" // 心配・困惑
	MoodFocus     MoodType = "Focus"     // 集中
	MoodNegative  MoodType = "Negative"  // 悲しみ・失敗
)

// InferMood はStateやセッション状況からMoodを決定する。
func InferMood(ev MonitorEvent) MoodType {
	// 1. 強力なポジティブイベント (100% 喜び)
	if ev.State == types.StateSuccess || ev.Event == types.EventGitActivity {
		return MoodHappy
	}

	// 2. 強力なネガティブイベント (100% 悲しみ)
	if ev.State == types.StateFail {
		return MoodNegative
	}

	// 3. 作業中の「ゆらぎ」判定
	// 常に同じ表情だと退屈なので、確率的に表情を変える
	r := rand.Float64()

	switch ev.State {
	case types.StateDeepWork:
		return MoodFocus

	case types.StateCoding:
		if ev.Task == TaskGenerateCode || ev.Task == TaskDebug {
			return MoodFocus // デバッグ・コード生成中は常に集中顔
		}
		if r < 0.25 { // 25%の確率で「作業が楽しい（Positive）」
			return MoodPositive
		}
		if r > 0.70 { // 30%の確率で「作業に没頭（Focus）」
			return MoodFocus
		}
		return MoodNeutral

	case types.StateIdle:
		if r < 0.15 { // 15%の確率で「ちょっと寂しい・退屈（Negative）」
			return MoodNegative
		}
		if r > 0.85 { // 15%の確率で「リラックス（Positive）」
			return MoodPositive
		}
		return MoodNeutral

	case types.StateThinking:
		return MoodFocus
	}

	// 4. セッション状態に基づく判定（フォールバック）
	switch ev.Session.Mode {
	case types.ModeDeepFocus:
		return MoodQuiet
	case types.ModeProductiveFlow:
		return MoodPositive
	case types.ModeStruggling:
		return MoodConcerned
	}

	return MoodNeutral
}
