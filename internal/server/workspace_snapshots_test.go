package server

import (
	"testing"

	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/eventstore"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func withWorkspaceSnapshotStore(t *testing.T) *eventstore.Store {
	t.Helper()
	dir := t.TempDir()
	previous := workspaceDataDir
	workspaceDataDir = func() string { return dir }
	t.Cleanup(func() { workspaceDataDir = previous })
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
