package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/eventstore"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func withWorkspaceSnapshotStore(t *testing.T) *eventstore.Store {
	t.Helper()
	dir := t.TempDir()
	previous := workspaceDataDir
	previousLegacyState := workspaceLoadLegacyState
	workspaceDataDir = func() string { return dir }
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return &state.AppState{Sessions: map[string]state.SessionMeta{}}, nil
	}
	t.Cleanup(func() {
		workspaceDataDir = previous
		workspaceLoadLegacyState = previousLegacyState
	})
	return eventstore.New(dir)
}

func TestWorkspaceSidebarAndLocalProjectsSnapshotsComeFromEventStore(t *testing.T) {
	store := withWorkspaceSnapshotStore(t)
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "Chat",
	})

	sidebar := workspaceSidebarSnapshot().(readmodels.SidebarData)
	if len(sidebar.ProjectGroups) != 1 {
		t.Fatalf("expected one project group, got %#v", sidebar.ProjectGroups)
	}
	if sidebar.ProjectGroups[0].Chats[0].ChatID != "chat-1" {
		t.Fatalf("expected chat-1 in sidebar, got %#v", sidebar.ProjectGroups[0].Chats)
	}

	localProjects := workspaceLocalProjectsSnapshot().(readmodels.LocalProjectsSnapshot)
	if len(localProjects.Projects) != 1 || localProjects.Projects[0].LocalPath != "/tmp/project" {
		t.Fatalf("expected local project snapshot from event store, got %#v", localProjects.Projects)
	}
}

func TestWorkspaceChatSnapshotIncludesRecentTranscript(t *testing.T) {
	store := withWorkspaceSnapshotStore(t)
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 300, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "m1",
			"kind":      transcript.KindUserPrompt,
			"createdAt": float64(300),
			"content":   "hello",
		},
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 400, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "m2",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(400),
			"text":      "hi",
		},
	})

	snapshot := workspaceChatSnapshot("chat-1", 1).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.ChatID != "chat-1" {
		t.Fatalf("expected chat runtime, got %#v", snapshot.Runtime)
	}
	if len(snapshot.Messages) != 1 || snapshot.Messages[0]["_id"] != "m2" {
		t.Fatalf("expected only newest transcript entry, got %#v", snapshot.Messages)
	}
	if !snapshot.History.HasOlder || snapshot.History.OlderCursor == nil || *snapshot.History.OlderCursor != "m1" {
		t.Fatalf("expected older history cursor m1, got %#v", snapshot.History)
	}
}

func TestWorkspaceChatSnapshotSkipsMessageReplayForEmptyTmuxChat(t *testing.T) {
	store := withWorkspaceSnapshotStore(t)
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":      "chat-1",
		"projectId":   "project-1",
		"title":       "Chat",
		"tmuxSession": "abolqasem-chat-1",
	})
	if err := os.WriteFile(filepath.Join(workspaceDataDir(), "messages.jsonl"), []byte("{bad json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	snapshot, ok := workspaceChatSnapshot("chat-1", 200).(*readmodels.ChatSnapshot)
	if !ok || snapshot == nil {
		t.Fatalf("expected tmux chat snapshot despite malformed messages stream, got %#v", snapshot)
	}
	if snapshot.Runtime.TmuxSession != "abolqasem-chat-1" {
		t.Fatalf("expected tmux runtime metadata, got %#v", snapshot.Runtime)
	}
}

func TestWorkspaceChatSnapshotIgnoresStoredMessagesForTmuxChat(t *testing.T) {
	store := withWorkspaceSnapshotStore(t)
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":      "chat-1",
		"projectId":   "project-1",
		"title":       "Chat",
		"tmuxSession": "abolqasem-chat-1",
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 300, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "old-message",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(300),
			"text":      "old eventstore text",
		},
	})

	snapshot := workspaceChatSnapshot("chat-1", 200).(*readmodels.ChatSnapshot)
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected tmux snapshot to ignore stored messages, got %#v", snapshot.Messages)
	}
}

func TestWorkspaceChatSnapshotTrimsRedundantToolResultDebugRaw(t *testing.T) {
	store := withWorkspaceSnapshotStore(t)
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 300, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "tool-call",
			"kind":      transcript.KindToolCall,
			"createdAt": float64(300),
			"tool": map[string]any{
				"toolId":   "call-1",
				"toolKind": "bash",
				"toolName": "exec_command",
			},
		},
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 301, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "tool-result",
			"kind":      transcript.KindToolResult,
			"createdAt": float64(301),
			"toolId":    "call-1",
			"content":   "small display content",
			"debugRaw":  strings.Repeat("large raw payload", 100),
		},
	})

	snapshot := workspaceChatSnapshot("chat-1", 10).(*readmodels.ChatSnapshot)
	if _, ok := snapshot.Messages[1]["debugRaw"]; ok {
		t.Fatalf("expected redundant debugRaw to be trimmed, got %#v", snapshot.Messages[1])
	}
}

func TestWorkspaceChatSnapshotBackfillsLegacySessionToken(t *testing.T) {
	store := withWorkspaceSnapshotStore(t)
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return &state.AppState{Sessions: map[string]state.SessionMeta{
			"codex:legacy-session-1": {
				Key:         "codex:legacy-session-1",
				Agent:       "codex",
				SessionID:   "legacy-session-1",
				Cwd:         "/tmp/project",
				ProjectName: "Project",
				UpdatedAt:   time.Unix(1700000000, 0),
			},
		}}, nil
	}
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 210, map[string]any{
		"chatId":   "chat-1",
		"provider": "codex",
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 300, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "codex-user-legacy-session-1-1",
			"kind":      transcript.KindUserPrompt,
			"createdAt": float64(300),
			"content":   "hello",
		},
	})

	snapshot := workspaceChatSnapshot("chat-1", 10).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.SessionToken == nil || *snapshot.Runtime.SessionToken != "legacy-session-1" {
		t.Fatalf("expected backfilled session token, got %#v", snapshot.Runtime.SessionToken)
	}
}

func appendWorkspaceEvent(t *testing.T, store *eventstore.Store, stream string, eventType string, timestamp int64, data map[string]any) {
	t.Helper()
	event, err := events.NewAt(eventType, timestamp, data)
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(stream, event); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
}
