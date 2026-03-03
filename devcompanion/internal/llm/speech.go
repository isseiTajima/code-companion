package llm

import (
	"context"
	"math/rand"
	"time"
	"unicode"

	"devcompanion/internal/config"
	"devcompanion/internal/monitor"
)

// FrequencyController は発話頻度を制御する。
type FrequencyController struct {
	lastSpeechAt     time.Time
	consecutiveCount int
	cooldownUntil    time.Time
	lastState        monitor.StateType
}

// NewFrequencyController は FrequencyController を初期化する。
func NewFrequencyController() *FrequencyController {
	return &FrequencyController{}
}

// ShouldSpeak は now 時点で発話すべきかを返す。
func (fc *FrequencyController) ShouldSpeak(reason Reason, state monitor.StateType, now time.Time) bool {
	switch reason {
	case ReasonUserClick:
		return true

	case ReasonSuccess, ReasonFail:
		// State遷移時に1回のみ発話
		return state != fc.lastState

	case ReasonThinkingTick:
		if now.Before(fc.cooldownUntil) {
			return false
		}
		// 7〜18秒のランダムインターバル
		interval := time.Duration(7+rand.Intn(12)) * time.Second
		return now.Sub(fc.lastSpeechAt) >= interval
	}

	return false
}

// RecordSpeak は発話を記録し、Thinking連続発話のカウントを更新する。
// 3回連続でThinkingTick発話した場合はクールダウンをセットする。
func (fc *FrequencyController) RecordSpeak(reason Reason, state monitor.StateType, now time.Time) {
	fc.lastSpeechAt = now
	fc.lastState = state

	if reason == ReasonThinkingTick {
		fc.consecutiveCount++
		if fc.consecutiveCount >= 3 {
			fc.cooldownUntil = now.Add(30 * time.Second)
			fc.consecutiveCount = 0
		}
	} else {
		fc.consecutiveCount = 0
	}
}

// SpeechGenerator はセリフを生成する。
type SpeechGenerator struct {
	ollama *OllamaClient
	freq   *FrequencyController
}

// NewSpeechGenerator は SpeechGenerator を初期化する。
func NewSpeechGenerator(model string) *SpeechGenerator {
	return &SpeechGenerator{
		ollama: NewOllamaClient(model),
		freq:   NewFrequencyController(),
	}
}

// Generate はMonitorEventとConfigからセリフを生成する。
// Mute=true のとき空文字を返す。発話条件を満たさない場合も空文字を返す。
func (sg *SpeechGenerator) Generate(e monitor.MonitorEvent, cfg *config.Config, reason Reason) string {
	if cfg.Mute {
		return ""
	}

	now := time.Now()
	if !sg.freq.ShouldSpeak(reason, e.State, now) {
		return ""
	}

	speech := sg.generateText(e, cfg, reason)
	sg.freq.RecordSpeak(reason, e.State, now)
	return postProcess(speech)
}

// OnUserClick はユーザークリック時のセリフを生成する。
func (sg *SpeechGenerator) OnUserClick(cfg *config.Config) string {
	if cfg.Mute {
		return ""
	}

	now := time.Now()
	e := monitor.MonitorEvent{State: monitor.StateIdle, Task: monitor.TaskPlan, Mood: monitor.MoodCalm}
	speech := sg.generateText(e, cfg, ReasonUserClick)
	sg.freq.RecordSpeak(ReasonUserClick, e.State, now)
	return postProcess(speech)
}

// generateText はOllamaまたはフォールバックからテキストを取得する。
func (sg *SpeechGenerator) generateText(e monitor.MonitorEvent, cfg *config.Config, reason Reason) string {
	input := OllamaInput{
		State:  string(e.State),
		Task:   string(e.Task),
		Mood:   string(e.Mood),
		Name:   cfg.Name,
		Tone:   cfg.Tone,
		Reason: string(reason),
	}
	text, err := sg.ollama.Generate(context.Background(), input)
	if err != nil {
		return FallbackSpeech(reason)
	}
	return text
}

// postProcess はセリフの後処理を行う（40文字切り捨て・絵文字削減）。
func postProcess(s string) string {
	runes := []rune(s)
	if len(runes) > 40 {
		runes = runes[:40]
	}

	emojiCount := 0
	result := make([]rune, 0, len(runes))
	for _, r := range runes {
		if isEmoji(r) {
			emojiCount++
			if emojiCount > 1 {
				// 2個目以降の絵文字を除去
				continue
			}
		}
		result = append(result, r)
	}

	return string(result)
}

// isEmoji は rune が絵文字かを判定する。
func isEmoji(r rune) bool {
	return unicode.Is(unicode.So, r)
}
