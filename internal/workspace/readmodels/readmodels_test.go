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

func TestDeriveChatSnapshotIncludesProviders(t *testing.T) {
	provider := "claude"
	planMode := true
	sessionToken := "session-1"
	state := EmptyState()
	state.ProjectsByID["project-1"] = ProjectRecord{
		ID:        "project-1",
		LocalPath: "/tmp/project",
		Title:     "Project",
		CreatedAt: 1,
		UpdatedAt: 1,
	}
	state.ProjectIDsByPath["/tmp/project"] = "project-1"
	state.ChatsByID["chat-1"] = ChatRecord{
		ID:              "chat-1",
		ProjectID:       "project-1",
		Title:           "Chat",
		CreatedAt:       1,
		UpdatedAt:       1,
		Provider:        &provider,
		PlanMode:        true,
		SessionToken:    &sessionToken,
		LastTurnOutcome: nil,
	}
	state.QueuedMessagesByChatID["chat-1"] = []QueuedChatMessage{{
		ID:          "queued-1",
		Content:     "follow up",
		Attachments: []ChatAttachment{},
		CreatedAt:   2,
		Provider:    &provider,
		Model:       "claude-sonnet-4-6",
		PlanMode:    &planMode,
	}}

	chat := DeriveChatSnapshot(
		state,
		map[string]KannaStatus{},
		map[string]bool{},
		"chat-1",
		ChatTranscriptSnapshot{
			Messages: []TranscriptEntry{},
			History: ChatHistorySnapshot{
				HasOlder:    false,
				OlderCursor: nil,
				RecentLimit: 200,
			},
		},
	)

	if chat == nil {
		t.Fatal("expected chat snapshot")
	}
	if chat.Runtime.Provider == nil || *chat.Runtime.Provider != "claude" {
		t.Fatalf("expected claude provider, got %#v", chat.Runtime.Provider)
	}
	if len(chat.QueuedMessages) != 1 || chat.QueuedMessages[0].Content != "follow up" {
		t.Fatalf("unexpected queued messages: %#v", chat.QueuedMessages)
	}
	if chat.History.RecentLimit != 200 {
		t.Fatalf("expected recent limit 200, got %d", chat.History.RecentLimit)
	}
	if len(chat.AvailableProviders) <= 1 {
		t.Fatalf("expected multiple providers, got %#v", chat.AvailableProviders)
	}
	var codexModels []string
	for _, provider := range chat.AvailableProviders {
		if provider.ID != "codex" {
			continue
		}
		for _, model := range provider.Models {
			codexModels = append(codexModels, model.ID)
		}
	}
	expected := []string{"gpt-5.5", "gpt-5.4", "gpt-5.3-codex", "gpt-5.3-codex-spark"}
	if len(codexModels) != len(expected) {
		t.Fatalf("expected codex models %#v, got %#v", expected, codexModels)
	}
	for index := range expected {
		if codexModels[index] != expected[index] {
			t.Fatalf("expected codex models %#v, got %#v", expected, codexModels)
		}
	}
}

func TestDeriveLocalProjectsSnapshotPrefersSavedProject(t *testing.T) {
	state := EmptyState()
	state.ProjectsByID["project-1"] = ProjectRecord{
		ID:        "project-1",
		LocalPath: "/tmp/project",
		Title:     "Saved Project",
		CreatedAt: 10,
		UpdatedAt: 20,
	}
	state.ChatsByID["chat-1"] = ChatRecord{
		ID:        "chat-1",
		ProjectID: "project-1",
		Title:     "Chat",
		CreatedAt: 30,
		UpdatedAt: 40,
	}

	snapshot := DeriveLocalProjectsSnapshot(
		state,
		[]DiscoveredProject{{
			LocalPath:  "/tmp/project",
			Title:      "Discovered Project",
			ModifiedAt: 5,
		}},
		"machine",
		"linux",
	)

	if snapshot.Machine.ID != "local" || snapshot.Machine.DisplayName != "machine" || snapshot.Machine.Platform != "linux" {
		t.Fatalf("unexpected machine: %#v", snapshot.Machine)
	}
	if len(snapshot.Projects) != 1 {
		t.Fatalf("expected one project, got %#v", snapshot.Projects)
	}
	project := snapshot.Projects[0]
	if project.Title != "Saved Project" {
		t.Fatalf("expected saved title, got %q", project.Title)
	}
	if project.Source != "saved" {
		t.Fatalf("expected saved source, got %q", project.Source)
	}
	if project.ChatCount != 1 {
		t.Fatalf("expected chat count 1, got %d", project.ChatCount)
	}
	if project.LastOpenedAt != 30 {
		t.Fatalf("expected last opened at 30, got %d", project.LastOpenedAt)
	}
}
