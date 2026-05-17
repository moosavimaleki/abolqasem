package server

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/legacyimport"
)

func TestWorkspaceTerminalCreateRequestUsesProjectRoot(t *testing.T) {
	withWorkspaceComposerStore(t)

	projectDir := t.TempDir()
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	raw, err := json.Marshal(map[string]any{
		"projectId":  project.ID,
		"terminalId": "term-1",
		"cols":       100,
		"rows":       30,
		"scrollback": 2000,
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	request, err := workspaceTerminalCreateRequest(raw)
	if err != nil {
		t.Fatalf("workspaceTerminalCreateRequest returned error: %v", err)
	}
	expectedCWD, err := filepath.Abs(projectDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if request.CWD != expectedCWD {
		t.Fatalf("expected terminal cwd %q, got %q", expectedCWD, request.CWD)
	}
	if request.ProjectID != project.ID || request.TerminalID != "term-1" || request.Cols != 100 || request.Rows != 30 || request.Scrollback != 2000 {
		t.Fatalf("unexpected create request: %#v", request)
	}
}

func TestWorkspaceTerminalCreateRequestRejectsUnknownProject(t *testing.T) {
	withWorkspaceComposerStore(t)

	raw, err := json.Marshal(map[string]any{
		"projectId":  "missing-project",
		"terminalId": "term-1",
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	if _, err := workspaceTerminalCreateRequest(raw); err == nil {
		t.Fatal("expected unknown project to be rejected")
	}
}

func TestWorkspaceTerminalCreateRequestUsesLegacyProjectRoot(t *testing.T) {
	withWorkspaceComposerStore(t)

	projectDir := t.TempDir()
	meta := state.SessionMeta{
		Key:            "codex:legacy-terminal",
		Agent:          "codex",
		SessionID:      "legacy-terminal",
		TranscriptPath: filepath.Join(projectDir, "rollout.jsonl"),
		Cwd:            projectDir,
		ProjectName:    "Legacy Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})
	projectID := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project.ID

	raw, err := json.Marshal(map[string]any{
		"projectId":  projectID,
		"terminalId": "term-legacy",
		"cols":       80,
		"rows":       24,
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	request, err := workspaceTerminalCreateRequest(raw)
	if err != nil {
		t.Fatalf("workspaceTerminalCreateRequest returned error: %v", err)
	}
	expectedCWD, err := filepath.Abs(projectDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if request.CWD != expectedCWD {
		t.Fatalf("expected legacy terminal cwd %q, got %q", expectedCWD, request.CWD)
	}
}
