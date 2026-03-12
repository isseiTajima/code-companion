package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"sakura-kodama/internal/config"
	"sakura-kodama/internal/monitor"
	"sakura-kodama/internal/profile"
	"sakura-kodama/internal/types"
)

func TestSpeechGenerator_EventContext_BuildSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"test response","done":true}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Name:           "TestBot",
		Tone:           "calm",
		OllamaEndpoint: server.URL,
		Model:          "test-model",
	}

	sg := NewSpeechGenerator(cfg)

	event := monitor.MonitorEvent{
		State: types.StateCoding,
	}

	_, _, _ = sg.Generate(event, cfg, ReasonSuccess, profile.DevProfile{}, "")
}

func TestSpeechGenerator_EventContext_BuildFailed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"hang in there","done":true}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		Name:           "TestBot",
		Tone:           "calm",
		OllamaEndpoint: server.URL,
		Model:          "test-model",
	}

	sg := NewSpeechGenerator(cfg)

	event := monitor.MonitorEvent{
		State: types.StateStuck,
	}

	_, _, _ = sg.Generate(event, cfg, ReasonFail, profile.DevProfile{}, "")
}
