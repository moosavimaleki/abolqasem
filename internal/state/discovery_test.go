package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverSessionsInRootsFindsKnownAgentTranscripts(t *testing.T) {
	root := t.TempDir()
	codexRoot := filepath.Join(root, "codex", "sessions")
	claudeRoot := filepath.Join(root, "claude", "projects")
	geminiRoot := filepath.Join(root, "gemini", "tmp")

	codexPath := writeDiscoveryFile(t,
		filepath.Join(codexRoot, "2026", "05", "14", "rollout-2026-05-14T10-00-00-019e2a32-513d-7c02-a78c-ab1b0130635c.jsonl"),
		`{"payload":{"type":"user_message","message":"hi"},"cwd":"/work/codex-project"}`+"\n",
	)
	claudePath := writeDiscoveryFile(t,
		filepath.Join(claudeRoot, "-work-claude-project", "claude-session.jsonl"),
		`{"session_id":"claude-session","cwd":"/work/claude-project","message":{"role":"user","content":"hi"}}`+"\n",
	)
	geminiPath := writeDiscoveryFile(t,
		filepath.Join(geminiRoot, "projecthash", "chats", "session-2026-05-14T10-00.json"),
		`{"session_id":"gemini-session","cwd":"/work/gemini-project","history":[{"role":"user","parts":[{"text":"hi"}]}]}`,
	)

	baseTime := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	setModTime(t, codexPath, baseTime)
	setModTime(t, claudePath, baseTime.Add(time.Minute))
	setModTime(t, geminiPath, baseTime.Add(2*time.Minute))

	appState := newAppState()
	report, err := DiscoverSessionsInRoots(appState, []DiscoveryRoot{
		{Agent: "codex", Path: codexRoot},
		{Agent: "claude", Path: claudeRoot},
		{Agent: "gemini", Path: geminiRoot},
	})
	if err != nil {
		t.Fatalf("DiscoverSessionsInRoots returned error: %v", err)
	}
	if report.Found != 3 || report.Added != 3 {
		t.Fatalf("unexpected discovery report: %+v", report)
	}

	assertDiscoveredSession(t, appState, "codex:019e2a32-513d-7c02-a78c-ab1b0130635c", "codex-project", codexPath)
	assertDiscoveredSession(t, appState, "claude:claude-session", "claude-project", claudePath)
	assertDiscoveredSession(t, appState, "gemini:gemini-session", "gemini-project", geminiPath)
	if appState.LatestSessionKey != "gemini:gemini-session" {
		t.Fatalf("expected latest gemini session, got %q", appState.LatestSessionKey)
	}
}

func TestDiscoverSessionsDoesNotRegressNewerHookMetadata(t *testing.T) {
	root := t.TempDir()
	codexRoot := filepath.Join(root, "codex", "sessions")
	codexPath := writeDiscoveryFile(t,
		filepath.Join(codexRoot, "2026", "05", "14", "rollout-same.jsonl"),
		`{"cwd":"/old/project","payload":{"type":"user_message","message":"hi"}}`+"\n",
	)
	oldTime := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	setModTime(t, codexPath, oldTime)

	appState := newAppState()
	UpsertSession(appState, HookEvent{
		Agent:          "codex",
		SessionID:      "rollout-same",
		TranscriptPath: codexPath,
		Cwd:            "/new/project",
		UpdatedAt:      newTime.Format(time.RFC3339),
	})

	report, err := DiscoverSessionsInRoots(appState, []DiscoveryRoot{{Agent: "codex", Path: codexRoot}})
	if err != nil {
		t.Fatalf("DiscoverSessionsInRoots returned error: %v", err)
	}
	if report.Found != 1 || report.Added != 0 {
		t.Fatalf("unexpected discovery report: %+v", report)
	}

	meta := appState.Sessions["codex:rollout-same"]
	if meta.Cwd != "/new/project" {
		t.Fatalf("expected newer cwd to survive, got %q", meta.Cwd)
	}
	if !meta.UpdatedAt.Equal(newTime) {
		t.Fatalf("expected newer timestamp to survive, got %s", meta.UpdatedAt)
	}
}

func TestDiscoverSessionsPrefersExistingCanonicalSessionIDForCodexTranscript(t *testing.T) {
	root := t.TempDir()
	codexRoot := filepath.Join(root, "codex", "sessions")
	codexPath := writeDiscoveryFile(t,
		filepath.Join(codexRoot, "2026", "05", "15", "rollout-2026-05-15T09-23-21-019e2a32-513d-7c02-a78c-ab1b0130635c.jsonl"),
		`{"cwd":"/work/codex-project","payload":{"type":"user_message","message":"hi"}}`+"\n",
	)

	appState := newAppState()
	UpsertSession(appState, HookEvent{
		Agent:          "codex",
		SessionID:      "019e2a32-513d-7c02-a78c-ab1b0130635c",
		TranscriptPath: codexPath,
		Cwd:            "/work/codex-project",
		UpdatedAt:      time.Date(2026, 5, 15, 9, 30, 0, 0, time.UTC).Format(time.RFC3339),
	})

	report, err := DiscoverSessionsInRoots(appState, []DiscoveryRoot{{Agent: "codex", Path: codexRoot}})
	if err != nil {
		t.Fatalf("DiscoverSessionsInRoots returned error: %v", err)
	}
	if report.Added != 0 {
		t.Fatalf("expected no duplicate add, got %+v", report)
	}
	if _, ok := appState.Sessions["codex:rollout-2026-05-15T09-23-21-019e2a32-513d-7c02-a78c-ab1b0130635c"]; ok {
		t.Fatal("discovery should not create a rollout-derived duplicate key")
	}
	if _, ok := appState.Sessions["codex:019e2a32-513d-7c02-a78c-ab1b0130635c"]; !ok {
		t.Fatal("expected canonical codex key to remain")
	}
}

func writeDiscoveryFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write discovery file: %v", err)
	}
	return path
}

func setModTime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("set mod time: %v", err)
	}
}

func assertDiscoveredSession(t *testing.T, appState *AppState, key, projectName, transcriptPath string) {
	t.Helper()
	meta, ok := appState.Sessions[key]
	if !ok {
		t.Fatalf("expected session %s in state", key)
	}
	if meta.ProjectName != projectName {
		t.Fatalf("expected project %q, got %q", projectName, meta.ProjectName)
	}
	if meta.TranscriptPath != transcriptPath {
		t.Fatalf("expected transcript path %q, got %q", transcriptPath, meta.TranscriptPath)
	}
	if meta.MetadataOnly {
		t.Fatalf("expected readable transcript, got metadata_only: %s", meta.InvalidReason)
	}
}
