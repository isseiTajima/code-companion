package persona

import (
	"testing"
	"sakura-kodama/internal/types"
)

func TestPersonaEngine_GetPromptModifiers(t *testing.T) {
	tests := []struct {
		style    types.PersonaStyle
		contains string
	}{
		{types.StyleSoft, "優しく"},
		{types.StyleEnergetic, "元気"},
		{types.StyleStrict, "直接的"},
	}

	for _, tt := range tests {
		p := NewPersonaEngine(tt.style)
		mod := p.GetPromptModifiers()
		if !contains(mod, tt.contains) {
			t.Errorf("style %v: expected to contain %q, got %q", tt.style, tt.contains, mod)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s[:len(sub)] == sub || contains(s[1:], sub))
}
