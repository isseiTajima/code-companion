package llm

import (
	"log"
	"regexp"
	"strings"
)

// SpeechProcessor defines the interface for post-processing speech text.
type SpeechProcessor interface {
	Process(s, lang string) string
}

// CleanupProcessor removes common unwanted symbols and decorations.
type CleanupProcessor struct {
	Symbols []string
}

func (p *CleanupProcessor) Process(s, lang string) string {
	for _, sym := range p.Symbols {
		s = strings.ReplaceAll(s, sym, "")
	}
	// 段落区切り（ニュース2部構成の残骸）を句点+スペースに置換してフラットな1文にする
	s = regexp.MustCompile(`\n{2,}`).ReplaceAllString(s, "。")
	s = regexp.MustCompile(`\n`).ReplaceAllString(s, " ")
	return s
}

// BracketProcessor removes text within various types of brackets (often stage directions).
type BracketProcessor struct {
	Re *regexp.Regexp
}

func (p *BracketProcessor) Process(s, lang string) string {
	return p.Re.ReplaceAllString(s, "")
}

// EmojiProcessor removes emojis and special symbols.
type EmojiProcessor struct {
	EmojiRe   *regexp.Regexp
	SymbolsRe *regexp.Regexp
}

func (p *EmojiProcessor) Process(s, lang string) string {
	s = p.EmojiRe.ReplaceAllString(s, "")
	return p.SymbolsRe.ReplaceAllString(s, "")
}

// LengthLimitProcessor trims the speech to a maximum length.
type LengthLimitProcessor struct {
	DefaultLimit int
}

func (p *LengthLimitProcessor) Process(s, lang string) string {
	// Note: maxLen override logic is handled in the main PostProcess call if needed,
	// but here we implement the default behavior.
	limit := p.DefaultLimit
	runes := []rune(s)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

// DuplicateNameProcessor ensures a specific name (like "先輩") doesn't appear too many times.
type DuplicateNameProcessor struct {
	Name string
}

func (p *DuplicateNameProcessor) Process(s, lang string) string {
	if strings.Count(s, p.Name) > 1 {
		first := strings.Index(s, p.Name)
		// Assuming Name is "先輩" (6 bytes in UTF-8)
		nameLen := len(p.Name)
		s = s[:first+nameLen] + strings.ReplaceAll(s[first+nameLen:], p.Name, "")
	}
	return s
}

// ScriptCheckProcessor discards speech if it contains invalid characters for the language.
type ScriptCheckProcessor struct{}

func (p *ScriptCheckProcessor) Process(s, lang string) string {
	if wrongScriptRunes(s, lang) {
		log.Printf("[WARN] postProcess: wrong-script chars detected (lang=%s), discarding: %s", lang, s)
		return ""
	}
	return s
}

// PhraseReplaceProcessor はフレーズを削除ではなく別の文字列に置換する。
// PhraseScrubProcessor（削除）と異なり、削除すると文が壊れる場合に使う。
//
// 使い方:
//
//	&PhraseReplaceProcessor{
//	    Patterns: map[string][]struct{ Re *regexp.Regexp; Repl string }{
//	        "ja": {
//	            {regexp.MustCompile(`置換したいパターン`), "置換後文字列"},
//	        },
//	    },
//	}
type PhraseReplaceProcessor struct {
	Patterns map[string][]struct {
		Re   *regexp.Regexp
		Repl string
	}
}

func (p *PhraseReplaceProcessor) Process(s, lang string) string {
	patterns, ok := p.Patterns[lang]
	if !ok {
		return s
	}
	for _, entry := range patterns {
		s = entry.Re.ReplaceAllString(s, entry.Repl)
	}
	return s
}

// SentenceTrimProcessor は日本語テキストで文末記号が2つ以上ある場合に最初の文末で切る。
// バッチ生成時に複数文が連結して出力された場合の対策。
type SentenceTrimProcessor struct{}

var jaEOSRe = regexp.MustCompile(`[。！？!?]`)

func (p *SentenceTrimProcessor) Process(s, lang string) string {
	if lang != "ja" {
		return s
	}
	locs := jaEOSRe.FindAllStringIndex(s, -1)
	if len(locs) < 2 {
		return s // 1文以下ならそのまま
	}
	// 2文以上ある場合は最初の文末で切る
	return strings.TrimSpace(s[:locs[0][1]])
}

// PhraseScrubProcessor removes specific banned phrases inline (without discarding the whole speech).
// Unlike BannedWordValidator (which rejects the entire speech), this scrubs only the offending phrase.
type PhraseScrubProcessor struct {
	Patterns map[string][]*regexp.Regexp
}

func (p *PhraseScrubProcessor) Process(s, lang string) string {
	patterns, ok := p.Patterns[lang]
	if !ok {
		return s
	}
	for _, re := range patterns {
		s = re.ReplaceAllString(s, "")
	}
	// Normalize leftover punctuation after scrubbing
	s = regexp.MustCompile(`[。、…！？\s]+$`).ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

var defaultProcessors = []SpeechProcessor{
	&CleanupProcessor{
		Symbols: []string{"**", "「", "」", "『", "』", "“", "”", "〘", "〙", "【", "】"},
	},
	&BracketProcessor{
		Re: regexp.MustCompile(`[（\(\{<\[].*?[）\)\}>\]]`),
	},
	&EmojiProcessor{
		EmojiRe:   regexp.MustCompile(`[\x{1F000}-\x{1FFFF}\x{2300}-\x{27FF}\x{2B00}-\x{2BFF}\x{FE0F}]+`),
		SymbolsRe: regexp.MustCompile(`[φðıिच]`),
	},
	&PhraseScrubProcessor{
		Patterns: map[string][]*regexp.Regexp{
			"ja": {
				// 特定フレーズ除去（モデルが多用する口癖パターン）
				regexp.MustCompile(`[。、…]?\s*認めざるを得ないというか[、…。]?\s*`), // 文頭・文中・文末すべて除去
			regexp.MustCompile(`[。、…]?\s*認めるしかありません[、…。]?\s*`),   // 同上・別形
				regexp.MustCompile(`^悪くないですよ[、。…]?\s*`),                    // 文頭の「悪くないですよ、」を除去
				regexp.MustCompile(`[。！、…]?\s*まじか[。！]?\s*$`),               // 文末の「まじか」を除去
				regexp.MustCompile(`[、。…]?\s*えっと[、。]?\s*$`),                 // 文末の「えっと」フィラーを除去
				regexp.MustCompile(`[。…、]?ずっといますよ[。…！]?`),
				regexp.MustCompile(`[。…、]?ちゃんといますよ[。…！]?`),
				regexp.MustCompile(`[。…、]?いつもいますよ[。…！]?`),
				regexp.MustCompile(`[。…、]?ずっとそばにいます[よね。…！]?`),
				regexp.MustCompile(`[。…、]?見守って(います|いますから|いますね|いますよ)[。…！]?`),
				regexp.MustCompile(`[。…、]?ちゃんと見てます[よね。…！]?`),
				regexp.MustCompile(`[。…、]?ずっと見てます[よね。…！]?`),
				regexp.MustCompile(`[。…、]?応援してます[よね。…！]?`),
			// 口癖化したテンプレ opener の除去（文頭に現れる定型パターン）
			regexp.MustCompile(`^一応確認ですけど[、，,]?\s*`),
			regexp.MustCompile(`^一応チェックですけど[、，,]?\s*`),
			regexp.MustCompile(`^黙ってましたけど[、，,]?\s*`),
			regexp.MustCompile(`^まじか[、，,]?\s*`),
			// 文末の機械的 closer 除去
			regexp.MustCompile(`[、。…！]?\s*それは認めます[。！]?\s*$`),
			regexp.MustCompile(`[、。…！]?\s*そこは認めます[。！]?\s*$`),
			},
			"en": {
				regexp.MustCompile(`[,.]?\s*[Ii]'m not going anywhere[.!]?`),
				regexp.MustCompile(`[,.]?\s*[Ii]'ll always be here[.!]?`),
				regexp.MustCompile(`[,.]?\s*[Ii]'ll be right here[.!]?`),
				regexp.MustCompile(`[,.]?\s*[Ii]'m watching[.!]?`),
				regexp.MustCompile(`[,.]?\s*[Ii]'ve been watching[.!]?`),
				regexp.MustCompile(`[,.]?\s*[Ii]'m always here[.!]?`),
			},
		},
	},
	// 複数文が連結して生成された場合に最初の1文だけ残す
	&SentenceTrimProcessor{},
}

func postProcessUnified(s, lang string, maxLen ...int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	for _, p := range defaultProcessors {
		s = p.Process(s, lang)
	}

	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Length limit with override support
	// デフォルト90文字（自然な後輩の一言として適切な長さ）。
	// ニュース解説など長い発言が必要な場合は maxLen で上書きする。
	limit := 90
	if len(maxLen) > 0 && maxLen[0] > limit {
		limit = maxLen[0]
	}
	lp := &LengthLimitProcessor{DefaultLimit: limit}
	s = lp.Process(s, lang)

	// Language-specific or character-specific cleanups
	if lang == "ja" {
		dp := &DuplicateNameProcessor{Name: "先輩"}
		s = dp.Process(s, lang)
	}

	// Final script check
	sp := &ScriptCheckProcessor{}
	s = sp.Process(s, lang)

	return s
}

// stripCodeBlock はモデルが返すテキストからJSON部分を抽出し、整形する。
func stripCodeBlock(s string) string {
	// 1. マークダウンのコードブロック記法があれば中身を取り出す
	if idx := strings.Index(s, "```"); idx >= 0 {
		content := s[idx+3:]
		// json などの言語指定があればスキップ
		if endLine := strings.Index(content, "\n"); endLine >= 0 && endLine < 10 {
			content = content[endLine+1:]
		}
		if endIdx := strings.Index(content, "```"); endIdx >= 0 {
			s = content[:endIdx]
		} else {
			s = content
		}
	}

	// 2. 最初の { と 最後の } の間を抜き出す（説明文対策）
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}

	// 3. 改行や不要な空白を除去して1行にまとめる（小規模モデルの不正な改行対策）
	lines := strings.Split(s, "\n")
	var buffer strings.Builder
	for _, line := range lines {
		buffer.WriteString(strings.TrimSpace(line))
	}

	return buffer.String()
}

// wrongScriptRunes は lang の設定と合わない文字スクリプトが含まれているか判定する。
// 「言語設定外の文字が混入した = LLM が言語を間違えた」として破棄判定に使う。
func wrongScriptRunes(s, lang string) bool {
	for _, r := range s {
		// いずれの言語でもあってはならないスクリプト
		switch {
		case r >= 0xAC00 && r <= 0xD7A3: return true // ハングル音節
		case r >= 0x1100 && r <= 0x11FF: return true // ハングル字母
		case r >= 0x3130 && r <= 0x318F: return true // ハングル互換字母
		case r >= 0x0600 && r <= 0x06FF: return true // アラビア語
		case r >= 0x0900 && r <= 0x097F: return true // デーヴァナーガリー
		case r >= 0x0400 && r <= 0x04FF: return true // キリル文字
		case r >= 0x0590 && r <= 0x05FF: return true // ヘブライ語
		case r >= 0x0E00 && r <= 0x0E7F: return true // タイ語
		// 簡体字中国語（日本語では使わない簡略字体）
		case r == 0x6837: return true // 样（日本語は様 U+69D8）
		case r == 0x4EEC: return true // 们（日本語では不使用）
		case r == 0x8FD9: return true // 这（日本語では不使用）
		case r == 0x65F6: return true // 时（日本語は時 U+6642）
		case r == 0x4E3A: return true // 为（日本語は為 U+70BA）
		case r == 0x4E1C: return true // 东（日本語は東 U+6771）
		}
		// 英語設定では日本語・CJK 系も不正
		if lang == "en" {
			switch {
			case r >= 0x3040 && r <= 0x309F: return true // ひらがな
			case r >= 0x30A0 && r <= 0x30FF: return true // カタカナ
			case r >= 0x4E00 && r <= 0x9FFF: return true // CJK統合漢字
			case r >= 0x3400 && r <= 0x4DBF: return true // CJK拡張A
			}
		}
	}
	return false
}
