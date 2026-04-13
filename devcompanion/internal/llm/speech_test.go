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
		_, _ = w.Write([]byte(`{"message":{"content":"ちゃんと進んでますね、いい感じですよ"},"done":true}`))
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

func TestFrequencyController_ActiveEdit_UsesRoutineInterval(t *testing.T) {
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	cfg := testConfig()
	cfg.SpeechFrequency = 2 // 中: routineInterval = 3分

	fc.RecordSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime)

	// 1分後 → インターバル未達 → false
	if fc.ShouldSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime.Add(1*time.Minute)) {
		t.Error("want false within routineInterval, got true")
	}
	// 3分1秒後 → インターバル超過 → true
	if !fc.ShouldSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime.Add(3*time.Minute+1*time.Second)) {
		t.Error("want true after routineInterval, got false")
	}
}

func TestFrequencyController_ActiveEdit_SuppressedAfterImportant(t *testing.T) {
	fc := NewFrequencyController()
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	cfg := testConfig()
	cfg.SpeechFrequency = 3 // 高: routineInterval = 90秒

	// GitCommit → alwaysSpeak
	fc.RecordSpeak(ReasonGitCommit, types.StateCoding, cfg, baseTime)

	// 2分以内の active_edit は抑制される
	if fc.ShouldSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime.Add(100*time.Second)) {
		t.Error("want false: active_edit suppressed within 2min after important event")
	}
	// 2分1秒後はルーティンインターバルで判定（90s < 121s → true）
	if !fc.ShouldSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime.Add(2*time.Minute+1*time.Second)) {
		t.Error("want true: active_edit allowed after post-important suppression")
	}
}

func TestFrequencyController_ActiveEdit_HighFreqShorterInterval(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		freq     int
		after    time.Duration
		wantTrue bool
	}{
		{1, 4 * time.Minute, false},  // freq=1: 5min interval, 4min → false
		{1, 5*time.Minute + 1, true}, // freq=1: 5min interval, 5min+ → true
		{2, 2 * time.Minute, false},  // freq=2: 3min interval, 2min → false
		{2, 3*time.Minute + 1, true}, // freq=2: 3min interval, 3min+ → true
		{3, 60 * time.Second, false}, // freq=3: 90s interval, 60s → false
		{3, 91 * time.Second, true},  // freq=3: 90s interval, 91s → true
	}
	for _, tt := range tests {
		fc2 := NewFrequencyController()
		cfg := testConfig()
		cfg.SpeechFrequency = tt.freq
		fc2.RecordSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime)
		got := fc2.ShouldSpeak(ReasonActiveEdit, types.StateCoding, cfg, baseTime.Add(tt.after))
		if got != tt.wantTrue {
			t.Errorf("freq=%d after=%v: want %v, got %v", tt.freq, tt.after, tt.wantTrue, got)
		}
	}
}

func TestPostProcess_TrimsLongSpeech(t *testing.T) {
	input := "これは非常に長いセリフです。80文字を超える場合は適切にカットされる必要があります。あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよらりるれろわをん"
	got := postProcess(input, "ja")
	
	if len([]rune(got)) > 120 {
		t.Errorf("postProcess should trim to 120 chars, got %d", len([]rune(got)))
	}
}
