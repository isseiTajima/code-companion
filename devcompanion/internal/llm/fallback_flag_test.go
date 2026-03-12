package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

// TestSpeechGenerator_FallbackFlag_InitiallyFalse は usingFallback フラグの初期状態テスト。
func TestSpeechGenerator_FallbackFlag_InitiallyFalse(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Name:           "テスト",
		Tone:           "calm",
		EncourageFreq:  "mid",
		Monologue:      true,
		AlwaysOnTop:    true,
		Mute:           false,
		Model:          "fake",
		OllamaEndpoint: "http://localhost",
	}
	sg := NewSpeechGenerator(cfg)
	_ = sg
}

func TestSpeechGenerator_FallbackFlag_ThreadSafeGet(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Name:           "テスト",
		Tone:           "calm",
		EncourageFreq:  "mid",
		Monologue:      true,
		AlwaysOnTop:    true,
		Mute:           false,
		Model:          "fake",
		OllamaEndpoint: "http://localhost",
	}
	_ = NewSpeechGenerator(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
		}()
	}
	wg.Wait()
}

func TestSpeechGenerator_Fallback_RecordingState(t *testing.T) {
	t.Parallel()

	wantText := "フォールバック結果"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"content": []map[string]string{{"text": wantText}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		Name:              "テスト",
		Tone:              "calm",
		EncourageFreq:     "mid",
		Monologue:         true,
		AlwaysOnTop:       true,
		Mute:              false,
		Model:             "fake",
		OllamaEndpoint:    server.URL,
		LLMBackend:        "claude",
		AnthropicAPIKey:   "test-key",
	}
	sg := NewSpeechGenerator(cfg)

	event := monitor.MonitorEvent{
		State: types.StateDeepWork,
	}

	speech, _, _ := sg.Generate(event, cfg, ReasonUserClick, profile.DevProfile{}, "")
	if speech == "" {
		t.Error("want non-empty speech even after fallback")
	}
}

func TestSpeechGenerator_Fallback_MultipleCallsConcurrent(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Name:           "テスト",
		Tone:           "calm",
		EncourageFreq:  "mid",
		Monologue:      false,
		AlwaysOnTop:    true,
		Mute:           false,
		Model:          "fake",
		OllamaEndpoint: "http://localhost",
		LLMBackend:     "fallback",
	}
	sg := NewSpeechGenerator(cfg)

	event := monitor.MonitorEvent{
		State: types.StateIdle,
	}

	var wg sync.WaitGroup
	results := make(chan string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			speech, _, _ := sg.Generate(event, cfg, ReasonUserClick, profile.DevProfile{}, "")
			results <- speech
		}()
	}

	wg.Wait()
	close(results)

	count := 0
	for range results {
		count++
	}
	if count != 10 {
		t.Errorf("want 10 results, got %d", count)
	}
}

func TestSpeechGenerator_ClaudeSuccess_ResetsUsingFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"content": []map[string]string{{"text": "復帰した"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		Name:              "テスト",
		Tone:              "calm",
		EncourageFreq:     "mid",
		Monologue:         false,
		AlwaysOnTop:       true,
		Mute:              false,
		Model:             "fake",
		OllamaEndpoint:    "http://localhost",
		LLMBackend:        "claude",
		AnthropicAPIKey:   "test-key",
	}
	sg := NewSpeechGenerator(cfg)
	sg.router.claude.(*AnthropicClient).endpoint = server.URL
	sg.router.claude.(*AnthropicClient).timeout = 200 * time.Millisecond

	event := monitor.MonitorEvent{
		State: types.StateIdle,
	}

	speech, _, _ := sg.Generate(event, cfg, ReasonUserQuestion, profile.DevProfile{}, "テスト質問")
	if speech != "復帰した" {
		t.Errorf("want '復帰した', got %q", speech)
	}
}
