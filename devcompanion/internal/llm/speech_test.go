package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

func testConfig() *config.Config {
	return config.DefaultConfig()
}

func TestFrequencyController_ThinkingTick_SpeaksAfterMinInterval(t *testing.T) {
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := testConfig()

	fc.RecordSpeak(ReasonThinkingTick, types.StateDeepWork, cfg, baseTime)

	// ThinkingTick interval depends on SpeechFrequency
	now := baseTime.Add(16 * time.Minute) 
	can := fc.ShouldSpeak(ReasonThinkingTick, types.StateDeepWork, cfg, now)

	if !can {
		t.Error("want true after long interval, got false")
	}
}

func TestFrequencyController_UserClick_AlwaysSpeaks(t *testing.T) {
	fc := NewFrequencyController()
	cfg := testConfig()

	can := fc.ShouldSpeak(ReasonUserClick, types.StateIdle, cfg, time.Now())

	if !can {
		t.Error("want true for user click, got false")
	}
}

func TestSpeechGenerator_Generate_ContainsDetailsAndQuestion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"了解しました！"},"done":true}`))
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.OllamaEndpoint = server.URL
	sg := NewSpeechGenerator(cfg)
	prof := profile.DevProfile{}

	event := monitor.MonitorEvent{
		State:   types.StateCoding,
		Details: "main.go",
	}
	question := "今日は何をすればいい？"

	_, prompt, _ := sg.Generate(event, cfg, ReasonUserQuestion, prof, question)

	if !strings.Contains(prompt, "main.go") {
		t.Errorf("prompt should contain details 'main.go', but got: %s", prompt)
	}
	if !strings.Contains(prompt, question) {
		t.Errorf("prompt should contain question '%s', but got: %s", question, prompt)
	}
}

func TestPostProcess_TrimsLongSpeech(t *testing.T) {
	input := "これは非常に長いセリフです。80文字を超える場合は適切にカットされる必要があります。あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよらりるれろわをん"
	got := postProcess(input)
	
	if len([]rune(got)) > 120 {
		t.Errorf("postProcess should trim to 120 chars, got %d", len([]rune(got)))
	}
}
