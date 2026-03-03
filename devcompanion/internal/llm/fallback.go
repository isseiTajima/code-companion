package llm

// Reason はセリフ生成の理由を表す。
type Reason string

const (
	ReasonSuccess      Reason = "success"
	ReasonFail         Reason = "fail"
	ReasonThinkingTick Reason = "thinking_tick"
	ReasonUserClick    Reason = "user_click"
)

var fallbackTexts = map[Reason]string{
	ReasonThinkingTick: "考え中…",
	ReasonSuccess:      "よし、できた！",
	ReasonFail:         "うっ…でも大丈夫",
	ReasonUserClick:    "なに？",
}

// FallbackSpeech はLLM呼び出し失敗時のテンプレートセリフを返す。
func FallbackSpeech(r Reason) string {
	if text, ok := fallbackTexts[r]; ok {
		return text
	}
	return "…"
}
