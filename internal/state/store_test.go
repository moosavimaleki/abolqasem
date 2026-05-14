package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStateRecoversCorruptFile(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	path := GetStateFilePath()
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	appState, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if len(appState.Sessions) != 0 {
		t.Fatalf("expected recovered empty state, got %+v", appState)
	}

	matches, err := filepath.Glob(path + ".corrupt.*")
	if err != nil {
		t.Fatalf("glob corrupt files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one corrupt backup, got %v", matches)
	}
}

func TestNormalizeAndValidateEventBuildsFallbackAndMetadataOnly(t *testing.T) {
	event := NormalizeAndValidateEvent(HookEvent{
		Agent: "gemini",
		Cwd:   "/tmp/project",
	})

	if event.SessionID == "" {
		t.Fatal("expected fallback session id")
	}
	if !event.MetadataOnly {
		t.Fatal("expected metadata only when transcript path is missing")
	}
	if !strings.Contains(event.InvalidReason, "transcript") {
		t.Fatalf("expected transcript reason, got %q", event.InvalidReason)
	}
}

func TestUpsertSessionUsesCompositeKey(t *testing.T) {
	appState := newAppState()
	meta := UpsertSession(appState, HookEvent{
		Agent:        "codex",
		SessionID:    "same-id",
		Cwd:          "/tmp/project-a",
		MetadataOnly: true,
	})
	other := UpsertSession(appState, HookEvent{
		Agent:        "claude",
		SessionID:    "same-id",
		Cwd:          "/tmp/project-b",
		MetadataOnly: true,
	})

	if meta.Key == other.Key {
		t.Fatalf("expected different keys, got %s", meta.Key)
	}
	if len(appState.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(appState.Sessions))
	}
}
