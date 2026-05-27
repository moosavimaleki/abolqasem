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

func TestLoadSettingsUsesDefaultsAndPersistsFalseValues(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	defaults, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}
	if !defaults.HookUpdates || defaults.HookFollowMode != HookFollowAuto || !defaults.FilesystemDiscovery {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}

	next := defaults
	next.HookUpdates = false
	next.HookFollowMode = HookFollowNotice
	next.IgnoreHookNavigationWhileTyping = false
	next.FilesystemDiscovery = false
	if err := SaveSettings(next); err != nil {
		t.Fatalf("SaveSettings returned error: %v", err)
	}

	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}
	if loaded.HookUpdates || loaded.HookFollowMode != HookFollowNotice || loaded.IgnoreHookNavigationWhileTyping || loaded.FilesystemDiscovery {
		t.Fatalf("expected persisted false values, got %+v", loaded)
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

func TestUpsertSessionCanonicalizesTranscriptDuplicatesAndPreservesCustomName(t *testing.T) {
	appState := newAppState()
	cwd := t.TempDir()
	transcriptPath := filepath.Join(cwd, "rollout-2026-05-15T09-23-21-019e2a32-513d-7c02-a78c-ab1b0130635c.jsonl")
	if err := os.WriteFile(transcriptPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	alias := SessionMeta{
		Key:            "codex:rollout-2026-05-15T09-23-21-019e2a32-513d-7c02-a78c-ab1b0130635c",
		Agent:          "codex",
		SessionID:      "rollout-2026-05-15T09-23-21-019e2a32-513d-7c02-a78c-ab1b0130635c",
		SessionName:    "تست",
		TranscriptPath: transcriptPath,
		Cwd:            cwd,
		ProjectName:    "codex",
	}
	appState.Sessions[alias.Key] = alias

	meta := UpsertSession(appState, HookEvent{
		Agent:          "codex",
		SessionID:      "019e2a32-513d-7c02-a78c-ab1b0130635c",
		TranscriptPath: transcriptPath,
		Cwd:            cwd,
		ProjectName:    "codex",
		MetadataOnly:   true,
	})

	if meta.Key != "codex:019e2a32-513d-7c02-a78c-ab1b0130635c" {
		t.Fatalf("expected canonical key, got %q", meta.Key)
	}
	if meta.SessionName != "تست" {
		t.Fatalf("expected custom session name to survive, got %q", meta.SessionName)
	}
	if len(appState.Sessions) != 1 {
		t.Fatalf("expected duplicate sessions to collapse, got %d", len(appState.Sessions))
	}
	if _, ok := appState.Sessions[alias.Key]; ok {
		t.Fatalf("expected alias key %q to be removed", alias.Key)
	}
}
