package persona

import (
	"sakura-kodama/internal/types"
)

// CharacterCore は Sakura Kodama の基本人格（後輩）。
type CharacterCore struct {
	Name string
	Tone string
}

// PersonaEngine は Core と Style を組み合わせて最終的な表現を決定する。
type PersonaEngine struct {
	Core  CharacterCore
	Style types.PersonaStyle
}

func NewPersonaEngine(style types.PersonaStyle) *PersonaEngine {
	return &PersonaEngine{
		Core: CharacterCore{
			Name: "さくら",
			Tone: "フレンドリーな後輩",
		},
		Style: style,
	}
}

// GetPromptModifiers はスタイルに応じたプロンプト指示を返す。
func (p *PersonaEngine) GetPromptModifiers() string {
	switch p.Style {
	case types.StyleSoft, types.StyleCute:
		return "優しく見守る、控えめで可愛いトーンで話してください。"
	case types.StyleEnergetic, types.StyleGenki:
		return "元気いっぱいに、ポジティブで明るいトーンで話してください。"
	case types.StyleStrict, types.StyleTsukime:
		return "少し生意気で直接的な、きつめのトーンで話してください。"
	default:
		return "丁寧かつ明るい態度で接してください。"
	}
}
