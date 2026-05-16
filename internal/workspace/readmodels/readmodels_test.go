package readmodels

import (
	"testing"

	"ai-agent-manager/internal/workspace/events"
)

func TestDeriveSidebarData(t *testing.T) {
	projectEvent, err := events.NewAt(events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "project",
	})
	if err != nil {
		t.Fatalf("NewAt project returned error: %v", err)
	}
	chatEvent, err := events.NewAt(events.TypeChatCreated, 200, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "first chat",
	})
	if err != nil {
		t.Fatalf("NewAt chat returned error: %v", err)
	}

	state := Replay([]events.Event{projectEvent, chatEvent})
	sidebar := DeriveSidebarData(state)

	if len(sidebar.ProjectGroups) != 1 {
		t.Fatalf("expected 1 project group, got %d", len(sidebar.ProjectGroups))
	}
	group := sidebar.ProjectGroups[0]
	if group.GroupKey != "project-1" {
		t.Fatalf("expected project-1, got %q", group.GroupKey)
	}
	if len(group.Chats) != 1 {
		t.Fatalf("expected 1 chat, got %d", len(group.Chats))
	}
	if group.Chats[0].ChatID != "chat-1" {
		t.Fatalf("expected chat-1, got %q", group.Chats[0].ChatID)
	}
}

func TestDeriveSidebarDataSkipsDeletedProject(t *testing.T) {
	opened, err := events.NewAt(events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "project",
	})
	if err != nil {
		t.Fatalf("NewAt opened returned error: %v", err)
	}
	removed, err := events.NewAt(events.TypeProjectRemoved, 200, map[string]any{
		"projectId": "project-1",
	})
	if err != nil {
		t.Fatalf("NewAt removed returned error: %v", err)
	}

	sidebar := DeriveSidebarData(Replay([]events.Event{opened, removed}))
	if len(sidebar.ProjectGroups) != 0 {
		t.Fatalf("expected no project groups, got %d", len(sidebar.ProjectGroups))
	}
}

func TestDeriveSidebarDataSeparatesArchivedChats(t *testing.T) {
	opened, _ := events.NewAt(events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "project",
	})
	created, _ := events.NewAt(events.TypeChatCreated, 200, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "first chat",
	})
	archived, _ := events.NewAt(events.TypeChatArchived, 300, map[string]any{
		"chatId": "chat-1",
	})

	sidebar := DeriveSidebarData(Replay([]events.Event{opened, created, archived}))
	if len(sidebar.ProjectGroups) != 1 {
		t.Fatalf("expected 1 project group, got %d", len(sidebar.ProjectGroups))
	}
	group := sidebar.ProjectGroups[0]
	if len(group.Chats) != 0 {
		t.Fatalf("expected no active chats, got %d", len(group.Chats))
	}
	if len(group.ArchivedChats) != 1 {
		t.Fatalf("expected 1 archived chat, got %d", len(group.ArchivedChats))
	}
}
