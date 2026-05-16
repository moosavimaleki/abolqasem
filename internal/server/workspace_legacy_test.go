package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/legacyimport"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func withLegacyState(t *testing.T, appState *state.AppState) {
	t.Helper()
	previous := workspaceLoadLegacyState
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return appState, nil
	}
	t.Cleanup(func() { workspaceLoadLegacyState = previous })
}

func TestMergeLegacySidebarDataAddsReadOnlySessionsUnderProjects(t *testing.T) {
	updatedAt := time.Unix(1700000000, 0)
	appState := &state.AppState{Sessions: map[string]state.SessionMeta{
		"claude:legacy-1": {
			Key:            "claude:legacy-1",
			Agent:          "claude",
			SessionID:      "legacy-1",
			SessionName:    "Legacy Session",
			TranscriptPath: "/tmp/project/transcript.jsonl",
			Cwd:            "/tmp/project",
			ProjectName:    "Project",
			UpdatedAt:      updatedAt,
			MetadataOnly:   true,
		},
	}}
	withLegacyState(t, appState)

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 1 {
		t.Fatalf("expected one project group, got %#v", sidebar.ProjectGroups)
	}
	chats := sidebar.ProjectGroups[0].Chats
	if len(chats) != 1 {
		t.Fatalf("expected one legacy chat, got %#v", chats)
	}
	if !chats[0].ReadOnly || chats[0].CanResume {
		t.Fatalf("expected non-resumable read-only legacy chat, got %#v", chats[0])
	}
	if chats[0].LegacySessionKey != "claude:legacy-1" {
		t.Fatalf("expected legacy session key, got %q", chats[0].LegacySessionKey)
	}
}

func TestWorkspaceLegacyChatSnapshotImportsTranscript(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"hello"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"hi"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	snapshot := workspaceLegacyChatSnapshot(legacyimport.LegacyChatID(meta), 10).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.LegacySessionKey != meta.Key || !snapshot.Runtime.ReadOnly || !snapshot.Runtime.CanResume {
		t.Fatalf("unexpected legacy runtime: %#v", snapshot.Runtime)
	}
	if len(snapshot.Messages) != 2 {
		t.Fatalf("expected imported transcript messages, got %#v", snapshot.Messages)
	}
	if transcript.Kind(snapshot.Messages[0]) != transcript.KindUserPrompt {
		t.Fatalf("expected user prompt first, got %#v", snapshot.Messages[0])
	}
}
