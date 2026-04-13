package llm

import (
	"regexp"
	"strings"
)

// SpeechValidator defines the interface for speech validation rules.
type SpeechValidator interface {
	Validate(s, lang string) bool
}

// LengthValidator checks if the speech is too short.
type LengthValidator struct {
	MinLength int
}

func (v *LengthValidator) Validate(s, lang string) bool {
	runes := []rune(strings.TrimSpace(s))
	return len(runes) >= v.MinLength
}

// BannedWordValidator checks for prohibited words or phrases.
type BannedWordValidator struct {
	BannedWords map[string][]string // lang -> words
}

func (v *BannedWordValidator) Validate(s, lang string) bool {
	words, ok := v.BannedWords[lang]
	if !ok {
		// Default to Japanese if language not found, or skip if that's preferred
		words = v.BannedWords["ja"]
	}
	sl := strings.ToLower(s)
	for _, b := range words {
		if strings.Contains(sl, strings.ToLower(b)) {
			return false
		}
	}
	return true
}

// ScriptValidator checks if the speech contains characters from the wrong script.
type ScriptValidator struct{}

func (v *ScriptValidator) Validate(s, lang string) bool {
	return !wrongScriptRunes(s, lang)
}

// RegexValidator checks the speech against a list of regular expressions.
type RegexValidator struct {
	Patterns map[string][]*regexp.Regexp // lang -> patterns
}

func (v *RegexValidator) Validate(s, lang string) bool {
	patterns, ok := v.Patterns[lang]
	if !ok {
		return true // No patterns for this language
	}
	for _, re := range patterns {
		if re.MatchString(s) {
			return false
		}
	}
	return true
}

// LanguageConsistencyValidator ensures English speech doesn't have Japanese characters and vice versa.
type LanguageConsistencyValidator struct{}

func (v *LanguageConsistencyValidator) Validate(s, lang string) bool {
	if lang == "en" {
		// English mode: should not contain Hiragana, Katakana, or Han (Kanji)
		return !regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`).MatchString(s)
	}
	// Japanese mode: must contain at least some Japanese characters
	return regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`).MatchString(s)
}

// SuspectWordValidator checks for suspicious words (like code fragments).
type SuspectWordValidator struct {
	MaxWordLength int
}

func (v *SuspectWordValidator) Validate(s, lang string) bool {
	words := strings.Fields(s)
	for _, w := range words {
		if len(w) > v.MaxWordLength && regexp.MustCompile(`^[a-zA-Z]+$`).MatchString(w) {
			return false
		}
	}
	return true
}

var (
	defaultValidators = []SpeechValidator{
		&LengthValidator{MinLength: 18},
		&SuspectWordValidator{MaxWordLength: 15},
		&ScriptValidator{},
		&LanguageConsistencyValidator{},
		&BannedWordValidator{
			BannedWords: map[string][]string{
				"ja": {
					"魔法", "ダンス", "宝石", "芸術", "宝物",
					"お手伝いできること", "お力になれ", "サポートさせ", "かしこまりました",
					// キーボード（音・動作両方禁止）
					"キーボードを叩く音", "キーボードの音",
					"キーボードを叩", "キーボードを打", "キーボードが", "タイピングして",
					// 顔・表情（色や具合も含む）
					"難しい顔", "難しそうな顔", "真剣な顔", "いい顔してる", "顔色",
					"顔が赤", "顔が青", "顔が真", "少し顔", "顔が白", "顔がwarm", "肌がwarm",
					// 嗅覚・五感
					"コーヒーの香り", "コーヒーの匂い", "いい香り", "香りがし",
					// 中国語混入
					"的样子", "前辈", "前輩さん",
					// 顔の観察（拡張）
					"そんな顔", "こんな顔", "その顔",
					// 操作系
					"配置変えました", "確認してきます", "確認してみます", "見てきます",
					"調べてきます", "やってきます", "直しておきます", "開いておきます",
					"やっておきます", "しておきます",
					// 見てます系（現在形・過去形両方）
					"見ていました", "見てました",
					"見てますよ", "ちゃんと見てます", "ずっと見てます", "見守って",
					// 常在アピール（存在のごり押し）
					"ずっといますよ", "ちゃんといますよ", "いつもいます", "ずっとそばに",
					// 全然こっち見ない系
					"全然こっち", "こっちを見て", "こっち向いて",
					// その他
					"お疲れ様", "なるほど", "すごい時間",
					"休憩", "ストレッチ", "一休み",
					"コーヒー", "集中",
					"夏みたい", "春みたい", "秋みたい", "冬みたい",
					"夏っぽい", "春っぽい", "秋っぽい", "冬っぽい",
					// 体調確認・労い：繰り返し出すぎるため全面禁止
					"大丈夫ですか", "疲れてないですか", "お疲れではないですか",
					"無理してないですか", "無理しないでください",
					"なにかお手伝い", "何かお手伝い",
					// 観察できないものへの言及（コード内容・複数画面・音）
					"コードの論理", "コードを読ん", "コードの内容", "コードを見てみ",
					"もう一つの画面", "別の画面", "もう1つの画面",
					"音が聞こえ", "静かな音",
					// 観察できない過去の具体的参照
					"去年と同じ", "去年も同じ", "昨年と同じ", "去年の勢い", "去年の夏",
					// 観察できない感覚・コード動作への言及
					"空気でわかる", "空気で分か", "コードが動き出", "作業が丁寧",
					// 何が気になるか言わない不完全文
					"気になってたんですけど", "気になってたんですが",
					// プロンプト内容がセリフに漏れ出したもの
					"NGパターン", "NG パターン",
					// 過多使用フレーズ（バッチ多様性確保のため禁止）
					"ワクワク",
					"いい感じじゃないですか",
					"AI と組んで", "AIと組んで",
					// 技術的な能力評価（サクラには見えないので言わない）
					"プロンプト力", "指示が的確", "指示うまい", "指示の出し方",
					"仮説を立て", "仮説立て", "バグを絞", "バグ追",
					"レビューも立派", "立派な仕事",
					// 評価で判明した頻出低評価フレーズ
					"新しいツール", "ファイル切り替", "頭フル回転", "フル回転",
					"ちょっと待って", "ちゃんとわかってます", "ちゃんと分かってます",
					"迷ってるみたい", "迷っているみたい",
					"力を出せる", "流儀ですね",
					"頑張っていますか", "頑張ってますか",
					"大丈夫そうですね", "大丈夫そうじゃないですか",
					"正解が見えてくる",
					// 桜・花の比喩（プロンプトで禁止しているがバリデーターでも保険）
					"桜の花", "花が舞", "花びら", "花が咲", "桜が", "桜の季節", "桜色",
					// 天気ネタの不自然な使い方（コーディングと無関係に天気を絡める）
					"春の雨", "雨の日でも", "雨の中でも", "雨でも", "肌寒いですけど",
					// 技術評価追加パターン
					"仮説で絞", "バグを特定",
					// 観察できない操作への言及
					"どのファイル", "ファイルを見て", "ファイル開い",
					// overused追加
					"新しいライブラリ",
					// 繰り返し出すぎる汎用フレーズ（多様性確保のため全禁止）
					"絶対いけます", "このまま行きましょう", "このままいきましょう",
					"このまま進んでる", "のが素敵です",
					// 方言混入（genki設定でも関西弁が出るため禁止）
					"ですわな", "わな～",
					// LLMが obsess するフレーズ
					"未知のもの",
					// コードは見えないので書いているとは言えない
					"コードを書いている", "コードを書いてる",
				},
				"en": {
					"blossom", "spring breeze", "spring wind", "unfurl", "gentle stream", "petal", "senpai", "cherry",
					"lovely to see", "lovely to watch", "i feel calm", "i feel safe", "i feel peaceful", "watching you work", "observing your",
					"i've been watching", "i'm watching", "i was watching", "i keep watching",
					"i'm not going anywhere", "i'll always be here", "i'll stay", "i'll never leave",
					"code looks clean", "code looks nice", "code looks neat", "code looks readable", "looks organized",
					"colors changed", "color changed", "color of your code",
					"smell", "coffee aroma", "keyboard sound", "i can hear", "keep typing", "been typing", "at the keyboard",
					"good work", "great work", "i see,", "i see.", "i see!", "i understand", "take a break", "need a break",
					"that's a long time", "so much time", "working for so long",
					"like summer", "like spring", "like winter", "like autumn", "like fall",
					"are you okay", "you okay", "you doing okay",
					"i was wondering", "i've been meaning to",
					// prompt content leaking into speech
					"ng pattern", "bad example",
					// overused phrases
					"excited to see", "working with ai",
				},
			},
		},
		&RegexValidator{
			Patterns: map[string][]*regexp.Regexp{
				"ja": {
					// プロンプト内容がセリフ先頭に漏れ出したもの（"NG" / "OK" / "例：" 等）
					regexp.MustCompile(`^(NG|OK|例[：:]|【|（例）)`),
					// cute以外の性格の語尾（oneesan/kansai/男性語尾）混入を弾く
					regexp.MustCompile(`(わね|わよ|だよね|だろ[うっ]?|やろ[う]?|やん[なか]?)[。！?６？]?\s*$`),
					regexp.MustCompile(`コード.{0,8}(綺麗|きれい|見やす|読みやす|整理|整っ|揃|形に|伸び|の色|色が|形になって|見やすくなっ)|(綺麗|きれい|見やす|読みやす|整理).{0,8}コード|見やすい配置`),
					// JSONフラグメント混入検出: ", " や null/true/false/数値リテラルが含まれる
					regexp.MustCompile(`",\s*"|,\s*(null|true|false)|,\s*-?[0-9]+\.[0-9]`),
				},
			},
		},
	}
)

func isValidSpeechUnified(s, lang string) bool {
	ok, _ := isValidSpeechUnifiedDetailed(s, lang)
	return ok
}

// isValidSpeechUnifiedDetailed はバリデーション結果と、失敗したバリデーター名を返す。
func isValidSpeechUnifiedDetailed(s, lang string) (bool, string) {
	if s == "" {
		return false, "empty"
	}
	for _, v := range defaultValidators {
		if !v.Validate(s, lang) {
			return false, validatorName(v, s)
		}
	}
	return true, ""
}

func validatorName(v SpeechValidator, s string) string {
	switch v.(type) {
	case *LengthValidator:
		return "Length"
	case *SuspectWordValidator:
		return "SuspectWord"
	case *ScriptValidator:
		return "Script"
	case *LanguageConsistencyValidator:
		return "LangConsistency"
	case *BannedWordValidator:
		// どの単語で引っかかったかを付加
		if bv, ok := v.(*BannedWordValidator); ok {
			sl := strings.ToLower(s)
			for _, words := range bv.BannedWords {
				for _, w := range words {
					if strings.Contains(sl, strings.ToLower(w)) {
						return "BannedWord:" + w
					}
				}
			}
		}
		return "BannedWord"
	case *RegexValidator:
		return "Regex"
	default:
		return "Unknown"
	}
}
