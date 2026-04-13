package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

func TestSpeechGenerator_WithRouter_OllamaFail_ClaudeSuccess(t *testing.T) {
	t.Parallel()

	claudeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"content": []map[string]string{{"text": "大丈夫ですよ、ちゃんと頑張ってますね"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer claudeServer.Close()

	cfg := &config.Config{
		Name:           "TestBot",
		Tone:           "calm",
		EncourageFreq:  "mid",
		Monologue:      true,
		OllamaEndpoint: "http://localhost:9999",
		AnthropicAPIKey: "test-key",
	}

	sg := NewSpeechGenerator(cfg)
	sg.router.claude.(*AnthropicClient).endpoint = claudeServer.URL
	sg.router.claude.(*AnthropicClient).timeout = 100 * time.Millisecond

	event := monitor.MonitorEvent{
		State: types.StateDeepWork,
	}

	speech, _, _ := sg.Generate(event, cfg, ReasonUserQuestion, profile.DevProfile{}, "最近どんな感じ？")
	if speech != "大丈夫ですよ、ちゃんと頑張ってますね" {
		t.Fatalf("want Japanese speech from Claude, got %q", speech)
	}
}

func TestSpeechGenerator_WithRouter_AllLayersFail_ReturnsFallback(t *testing.T) {
	t.Parallel()
	SetSeed(42) // 決定論的シード

	cfg := &config.Config{
		Name:            "TestBot",
		Tone:            "calm",
		EncourageFreq:   "mid",
		Monologue:       true,
		OllamaEndpoint:  "http://localhost:9999",
		AnthropicAPIKey: "",
	}

	sg := NewSpeechGenerator(cfg)
	sg.router.aiCLI = nil

	event := monitor.MonitorEvent{
		State: types.StateDeepWork,
	}

	speech, _, _ := sg.Generate(event, cfg, ReasonThinkingTick, profile.DevProfile{}, "")
	if speech == "" {
		t.Fatalf("want non-empty fallback speech")
	}
}


func TestSpeechGenerator_NoNilPanicWithoutRouter(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Name:           "TestBot",
		Tone:           "calm",
		EncourageFreq:  "mid",
		Monologue:      true,
		OllamaEndpoint: "http://localhost:11434/api/generate",
	}

	sg := NewSpeechGenerator(cfg)

	event := monitor.MonitorEvent{
		State: types.StateIdle,
	}

	speech, _, _ := sg.Generate(event, cfg, ReasonUserClick, profile.DevProfile{}, "")
	if speech == "" {
		t.Fatal("want non-empty speech")
	}
}
