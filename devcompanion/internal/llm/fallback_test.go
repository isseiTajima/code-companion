package llm

import "testing"

func TestFallbackSpeech_UserClickIsUpdatedPhrase(t *testing.T) {
	SetSeed(42)
	speech := FallbackSpeech(ReasonUserClick, "ja")

	if speech == "" || speech == "…" {
		t.Errorf("want specific text for user click, got %q", speech)
	}
}

func TestFallbackSpeech_UnknownReasonUsesEllipsis(t *testing.T) {
	speech := FallbackSpeech(Reason("unknown"), "ja")

	if speech != "…" {
		t.Errorf("want ellipsis fallback for unknown reason, got %q", speech)
	}
}

func TestFallbackSpeech_GitCommitIsNonEmpty(t *testing.T) {
	speech := FallbackSpeech(ReasonGitCommit, "ja")
	if speech == "" || speech == "…" {
		t.Errorf("want specific text for %s, got %q", ReasonGitCommit, speech)
	}
}
