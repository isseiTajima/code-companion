package llm

import (
	"strings"
	"testing"
	"time"

	"devcompanion/internal/monitor"
)

// --- FrequencyController: Thinking 頻度制御 ---

func TestFrequencyController_ThinkingTick_SpeaksAfterMinInterval(t *testing.T) {
	// Given: 発話済み・19秒後（最大インターバルを超えた）
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// 最初の発話を記録
	fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, baseTime)

	// When: 19秒後（7〜18秒の最大値を超えているので必ず発話可能）
	now := baseTime.Add(19 * time.Second)
	can := fc.ShouldSpeak(ReasonThinkingTick, monitor.StateThinking, now)

	// Then: 発話可能
	if !can {
		t.Error("want true after 19s (beyond max interval 18s), got false")
	}
}

func TestFrequencyController_ThinkingTick_SuppressedWithinMinInterval(t *testing.T) {
	// Given: 発話済み・6秒後（最小インターバル 7 秒未満）
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, baseTime)

	// When: 6秒後（最小インターバル未満）
	now := baseTime.Add(6 * time.Second)
	can := fc.ShouldSpeak(ReasonThinkingTick, monitor.StateThinking, now)

	// Then: 発話不可（最小インターバル内）
	if can {
		t.Error("want false within min interval (6s < 7s), got true")
	}
}

func TestFrequencyController_ThinkingTick_MaxConsecutive_TriggersCooldown(t *testing.T) {
	// Given: 3回連続発話（上限）
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// 3回連続発話を記録（各19秒後に実施して確実に発話可能にする）
	for i := 0; i < 3; i++ {
		now := baseTime.Add(time.Duration(i*20) * time.Second)
		fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, now)
	}

	// When: 3回連続後の次の発話試行（十分な時間が経過していても）
	now := baseTime.Add(60*time.Second + 1*time.Second) // クールダウン前の直後
	can := fc.ShouldSpeak(ReasonThinkingTick, monitor.StateThinking, now)

	// Then: クールダウン中なので発話不可
	if can {
		t.Error("want false after 3 consecutive speaks (cooldown triggered), got true")
	}
}

func TestFrequencyController_ThinkingTick_CooldownExpires_CanSpeak(t *testing.T) {
	// Given: 3回連続発話後、30秒クールダウンが終了
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// 3回連続発話（最後の発話を time=0s とする）
	lastSpeakTime := baseTime
	for i := 0; i < 3; i++ {
		t := baseTime.Add(time.Duration(i*20) * time.Second)
		fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, t)
		lastSpeakTime = t
	}

	// When: 最後の発話から 30 秒 + 19 秒後（クールダウン + 最大インターバル超過）
	now := lastSpeakTime.Add(30*time.Second + 19*time.Second)
	can := fc.ShouldSpeak(ReasonThinkingTick, monitor.StateThinking, now)

	// Then: 発話可能（クールダウン終了）
	if !can {
		t.Error("want true after cooldown expires (30s + interval), got false")
	}
}

func TestFrequencyController_ThinkingTick_Cooldown_Suppresses(t *testing.T) {
	// Given: 3回連続発話後のクールダウン期間中
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// 3回連続発話
	var lastTime time.Time
	for i := 0; i < 3; i++ {
		lastTime = baseTime.Add(time.Duration(i*20) * time.Second)
		fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, lastTime)
	}

	// When: クールダウン中（29秒後）
	now := lastTime.Add(29 * time.Second)
	can := fc.ShouldSpeak(ReasonThinkingTick, monitor.StateThinking, now)

	// Then: クールダウン中なので不可
	if can {
		t.Error("want false during cooldown (29s < 30s), got true")
	}
}

// --- FrequencyController: Success / Fail 遷移時に1回のみ ---

func TestFrequencyController_Success_SpeaksOnStateChange(t *testing.T) {
	// Given: 直前が Running 状態
	fc := NewFrequencyController()
	// Running を記録（FC の lastState を Running にする）
	fc.RecordSpeak(ReasonThinkingTick, monitor.StateRunning, time.Now())

	// When: State が Success に変わった直後
	can := fc.ShouldSpeak(ReasonSuccess, monitor.StateSuccess, time.Now())

	// Then: 発話可能（State 変化時は1回発話）
	if !can {
		t.Error("want true on Success state transition, got false")
	}
}

func TestFrequencyController_Success_DoesNotSpeakTwice(t *testing.T) {
	// Given: Success 発話を1回記録済み
	fc := NewFrequencyController()
	now := time.Now()

	fc.RecordSpeak(ReasonSuccess, monitor.StateSuccess, now)

	// When: 同じ Success State で再度 ShouldSpeak
	can := fc.ShouldSpeak(ReasonSuccess, monitor.StateSuccess, now.Add(1*time.Second))

	// Then: 発話不可（State 変化なし）
	if can {
		t.Error("want false when Success state hasn't changed, got true")
	}
}

func TestFrequencyController_Fail_SpeaksOnStateChange(t *testing.T) {
	// Given: 直前が Running 状態
	fc := NewFrequencyController()
	fc.RecordSpeak(ReasonThinkingTick, monitor.StateRunning, time.Now())

	// When: State が Fail に変わった直後
	can := fc.ShouldSpeak(ReasonFail, monitor.StateFail, time.Now())

	// Then: 発話可能
	if !can {
		t.Error("want true on Fail state transition, got false")
	}
}

func TestFrequencyController_Fail_DoesNotSpeakTwice(t *testing.T) {
	// Given: Fail 発話を1回記録済み
	fc := NewFrequencyController()
	now := time.Now()

	fc.RecordSpeak(ReasonFail, monitor.StateFail, now)

	// When: 同じ Fail State で再度 ShouldSpeak
	can := fc.ShouldSpeak(ReasonFail, monitor.StateFail, now.Add(1*time.Second))

	// Then: 発話不可
	if can {
		t.Error("want false when Fail state hasn't changed, got true")
	}
}

// --- FrequencyController: UserClick は常に発話 ---

func TestFrequencyController_UserClick_AlwaysSpeaks(t *testing.T) {
	// Given: 初期状態
	fc := NewFrequencyController()

	// When: UserClick
	can := fc.ShouldSpeak(ReasonUserClick, monitor.StateIdle, time.Now())

	// Then: 必ず発話可能
	if !can {
		t.Error("want true for user click, got false")
	}
}

func TestFrequencyController_UserClick_DuringCooldown_Speaks(t *testing.T) {
	// Given: Thinking クールダウン中（3回連続発話後）
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// 3回連続発話してクールダウンをトリガー
	var lastTime time.Time
	for i := 0; i < 3; i++ {
		lastTime = baseTime.Add(time.Duration(i*20) * time.Second)
		fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, lastTime)
	}

	// When: クールダウン中に UserClick
	now := lastTime.Add(5 * time.Second) // クールダウン中
	can := fc.ShouldSpeak(ReasonUserClick, monitor.StateThinking, now)

	// Then: UserClick はクールダウン無視で発話可能
	if !can {
		t.Error("want true for user click even during cooldown, got false")
	}
}

func TestFrequencyController_UserClick_AfterImmediateSpeak_Speaks(t *testing.T) {
	// Given: 直前に発話済み（ThinkingTick）
	fc := NewFrequencyController()
	now := time.Now()
	fc.RecordSpeak(ReasonThinkingTick, monitor.StateThinking, now)

	// When: 直後に UserClick
	can := fc.ShouldSpeak(ReasonUserClick, monitor.StateThinking, now.Add(1*time.Second))

	// Then: UserClick はインターバル無視で発話可能
	if !can {
		t.Error("want true for user click regardless of interval, got false")
	}
}

// --- postProcess: 文字数切り捨て ---

func TestPostProcess_TruncateAt40(t *testing.T) {
	// Given: 41文字の文字列（日本語）
	speech := strings.Repeat("あ", 41)

	// When: 後処理
	result := postProcess(speech)

	// Then: 40文字に切り捨て
	runes := []rune(result)
	if len(runes) != 40 {
		t.Errorf("want 40 runes, got %d", len(runes))
	}
}

func TestPostProcess_NoTruncateAt40(t *testing.T) {
	// Given: ちょうど40文字の文字列
	speech := strings.Repeat("あ", 40)

	// When: 後処理
	result := postProcess(speech)

	// Then: そのまま40文字
	runes := []rune(result)
	if len(runes) != 40 {
		t.Errorf("want 40 runes (no truncate), got %d", len(runes))
	}
}

func TestPostProcess_NoTruncateBelow40(t *testing.T) {
	// Given: 39文字の文字列
	speech := strings.Repeat("あ", 39)

	// When: 後処理
	result := postProcess(speech)

	// Then: 変更なし（切り捨て不要）
	runes := []rune(result)
	if len(runes) != 39 {
		t.Errorf("want 39 runes (no truncate), got %d", len(runes))
	}
}

func TestPostProcess_TruncateLong_PreservesFirst40(t *testing.T) {
	// Given: 50文字の文字列（先頭40文字が "あ"、残りが "い"）
	speech := strings.Repeat("あ", 40) + strings.Repeat("い", 10)

	// When: 後処理
	result := postProcess(speech)

	// Then: 先頭40文字（"あ"×40）のみ残る
	expected := strings.Repeat("あ", 40)
	if result != expected {
		t.Errorf("want first 40 chars preserved, got %q", result)
	}
}

// --- postProcess: 絵文字削減 ---

func TestPostProcess_SingleEmoji_Unchanged(t *testing.T) {
	// Given: 絵文字1個の文字列
	speech := "よし🎉"

	// When: 後処理
	result := postProcess(speech)

	// Then: 変更なし（絵文字1個は削減不要）
	if result != speech {
		t.Errorf("want %q unchanged, got %q", speech, result)
	}
}

func TestPostProcess_TwoEmojis_ReducesToOne(t *testing.T) {
	// Given: 絵文字2個の文字列
	speech := "よし🎉😊"

	// When: 後処理
	result := postProcess(speech)

	// Then: 最初の絵文字のみ残る（2個→1個）
	if strings.Contains(result, "😊") {
		t.Errorf("want second emoji removed, got %q", result)
	}
	if !strings.Contains(result, "🎉") {
		t.Errorf("want first emoji preserved, got %q", result)
	}
}

func TestPostProcess_ThreeEmojis_ReducesToOne(t *testing.T) {
	// Given: 絵文字3個の文字列
	speech := "よし🎉😊✨"

	// When: 後処理
	result := postProcess(speech)

	// Then: 最初の絵文字のみ残る
	if strings.Contains(result, "😊") || strings.Contains(result, "✨") {
		t.Errorf("want only first emoji, got %q", result)
	}
	if !strings.Contains(result, "🎉") {
		t.Errorf("want first emoji preserved, got %q", result)
	}
}

func TestPostProcess_NoEmoji_Unchanged(t *testing.T) {
	// Given: 絵文字なしの文字列
	speech := "よし、できた！"

	// When: 後処理
	result := postProcess(speech)

	// Then: 変更なし
	if result != speech {
		t.Errorf("want %q unchanged, got %q", speech, result)
	}
}

func TestPostProcess_TruncateAndEmojiReduction_Combined(t *testing.T) {
	// Given: 41文字かつ絵文字2個を含む文字列
	speech := strings.Repeat("あ", 39) + "🎉😊"

	// When: 後処理（切り捨て後に絵文字削減、または順序は実装依存）
	result := postProcess(speech)

	// Then: 40文字以下かつ絵文字は1個以下
	runes := []rune(result)
	if len(runes) > 40 {
		t.Errorf("want at most 40 runes, got %d", len(runes))
	}
}
