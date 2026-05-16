package agent

import (
	"context"
	"errors"
	"testing"

	"ai-agent-manager/internal/workspace/readmodels"
)

func TestSendStartsTurnAndBlocksConcurrentTurn(t *testing.T) {
	store := newFakeStore()
	starter := TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
		return &fakeTurn{}, nil
	})
	coordinator := NewCoordinator(store, starter, nil)

	result, err := coordinator.Send(context.Background(), SendCommand{
		ChatID:   "chat-1",
		Content:  "hello",
		Provider: "codex",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result.ChatID != "chat-1" || result.Queued {
		t.Fatalf("unexpected result: %#v", result)
	}
	if store.started != 1 {
		t.Fatalf("expected one started turn, got %d", store.started)
	}

	queued, err := coordinator.Send(context.Background(), SendCommand{
		ChatID:  "chat-1",
		Content: "follow up",
	})
	if err != nil {
		t.Fatalf("queued Send returned error: %v", err)
	}
	if !queued.Queued || queued.QueuedMessageID == "" {
		t.Fatalf("expected queued result, got %#v", queued)
	}
	if len(store.queued["chat-1"]) != 1 {
		t.Fatalf("expected one queued message, got %#v", store.queued["chat-1"])
	}
}

func TestCancelRemovesActiveTurn(t *testing.T) {
	store := newFakeStore()
	turn := &fakeTurn{}
	coordinator := NewCoordinator(store, TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
		return turn, nil
	}), nil)

	if _, err := coordinator.Send(context.Background(), SendCommand{ChatID: "chat-1", Content: "hello"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if err := coordinator.Cancel("chat-1"); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if !turn.cancelled {
		t.Fatal("expected turn to be cancelled")
	}
	if store.cancelled != 1 {
		t.Fatalf("expected one cancellation record, got %d", store.cancelled)
	}
	if len(coordinator.ActiveStatuses()) != 0 {
		t.Fatalf("expected no active statuses, got %#v", coordinator.ActiveStatuses())
	}
}

func TestProviderStartFailureRecordsFailure(t *testing.T) {
	store := newFakeStore()
	coordinator := NewCoordinator(store, TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
		return nil, errors.New("provider failed")
	}), nil)

	if _, err := coordinator.Send(context.Background(), SendCommand{ChatID: "chat-1", Content: "hello"}); err == nil {
		t.Fatal("expected send error")
	}
	if store.failed != 1 {
		t.Fatalf("expected one failure record, got %d", store.failed)
	}
	if len(coordinator.ActiveStatuses()) != 0 {
		t.Fatalf("expected no active statuses after failure, got %#v", coordinator.ActiveStatuses())
	}
}

func TestCancelCancelsTurnContext(t *testing.T) {
	store := newFakeStore()
	var turnContext context.Context
	coordinator := NewCoordinator(store, TurnStarterFunc(func(ctx context.Context, _ TurnRequest) (Turn, error) {
		turnContext = ctx
		return &fakeTurn{}, nil
	}), nil)

	if _, err := coordinator.Send(context.Background(), SendCommand{ChatID: "chat-1", Content: "hello"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if turnContext == nil {
		t.Fatal("expected turn context")
	}
	if err := coordinator.Cancel("chat-1"); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	select {
	case <-turnContext.Done():
	default:
		t.Fatal("expected turn context to be cancelled")
	}
}

func TestPendingToolSnapshot(t *testing.T) {
	store := newFakeStore()
	coordinator := NewCoordinator(store, TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
		return &fakeTurn{}, nil
	}), nil)

	if _, err := coordinator.Send(context.Background(), SendCommand{ChatID: "chat-1", Content: "hello"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if err := coordinator.SetPendingTool("chat-1", PendingToolRequest{
		ToolUseID: "tool-1",
		ToolKind:  "ask_user_question",
	}); err != nil {
		t.Fatalf("SetPendingTool returned error: %v", err)
	}
	pending := coordinator.PendingTool("chat-1")
	if pending == nil {
		t.Fatal("expected pending tool")
	}
	if pending.ToolUseID != "tool-1" || pending.ToolKind != "ask_user_question" {
		t.Fatalf("unexpected pending tool: %#v", pending)
	}
	if got := coordinator.ActiveStatuses()["chat-1"]; got != readmodels.StatusWaitingForUser {
		t.Fatalf("expected waiting_for_user status, got %q", got)
	}
}

func TestActiveTurnIncludesProjectAndStartedAt(t *testing.T) {
	store := newFakeStore()
	coordinator := NewCoordinator(store, TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
		return &fakeTurn{}, nil
	}), nil)

	if _, err := coordinator.Send(context.Background(), SendCommand{ChatID: "chat-1", Content: "hello"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	coordinator.mu.Lock()
	active := coordinator.active["chat-1"]
	coordinator.mu.Unlock()
	if active == nil {
		t.Fatal("expected active turn")
	}
	if active.ProjectID != "project-1" {
		t.Fatalf("expected project id, got %q", active.ProjectID)
	}
	if active.StartedAt.IsZero() {
		t.Fatal("expected startedAt to be set")
	}
}

type fakeStore struct {
	chats     map[string]readmodels.ChatRecord
	queued    map[string][]readmodels.QueuedChatMessage
	started   int
	cancelled int
	failed    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		chats: map[string]readmodels.ChatRecord{
			"chat-1": {
				ID:        "chat-1",
				ProjectID: "project-1",
				Title:     "New Chat",
				CreatedAt: 1,
				UpdatedAt: 1,
			},
		},
		queued: map[string][]readmodels.QueuedChatMessage{},
	}
}

func (s *fakeStore) CreateChat(projectID string) (readmodels.ChatRecord, error) {
	chat := readmodels.ChatRecord{
		ID:        "chat-created",
		ProjectID: projectID,
		Title:     "New Chat",
		CreatedAt: 1,
		UpdatedAt: 1,
	}
	s.chats[chat.ID] = chat
	return chat, nil
}

func (s *fakeStore) RequireChat(chatID string) (readmodels.ChatRecord, error) {
	chat, ok := s.chats[chatID]
	if !ok {
		return readmodels.ChatRecord{}, errors.New("chat not found")
	}
	return chat, nil
}

func (s *fakeStore) SetChatProvider(chatID string, provider string) error {
	chat := s.chats[chatID]
	chat.Provider = &provider
	s.chats[chatID] = chat
	return nil
}

func (s *fakeStore) SetPlanMode(chatID string, planMode bool) error {
	chat := s.chats[chatID]
	chat.PlanMode = planMode
	s.chats[chatID] = chat
	return nil
}

func (s *fakeStore) AppendUserPrompt(string, string, []readmodels.ChatAttachment, bool) error {
	return nil
}

func (s *fakeStore) RecordTurnStarted(string) error {
	s.started++
	return nil
}

func (s *fakeStore) RecordTurnFailed(string, string) error {
	s.failed++
	return nil
}

func (s *fakeStore) RecordTurnCancelled(string) error {
	s.cancelled++
	return nil
}

func (s *fakeStore) EnqueueMessage(chatID string, message QueueMessageInput) (readmodels.QueuedChatMessage, error) {
	queued := readmodels.QueuedChatMessage{
		ID:          "queued-1",
		Content:     message.Content,
		Attachments: message.Attachments,
		CreatedAt:   2,
	}
	s.queued[chatID] = append(s.queued[chatID], queued)
	return queued, nil
}

func (s *fakeStore) GetQueuedMessages(chatID string) []readmodels.QueuedChatMessage {
	return append([]readmodels.QueuedChatMessage(nil), s.queued[chatID]...)
}

func (s *fakeStore) GetQueuedMessage(chatID string, queuedMessageID string) (readmodels.QueuedChatMessage, bool) {
	for _, message := range s.queued[chatID] {
		if message.ID == queuedMessageID {
			return message, true
		}
	}
	return readmodels.QueuedChatMessage{}, false
}

func (s *fakeStore) RemoveQueuedMessage(chatID string, queuedMessageID string) error {
	existing := s.queued[chatID]
	next := existing[:0]
	for _, message := range existing {
		if message.ID != queuedMessageID {
			next = append(next, message)
		}
	}
	s.queued[chatID] = next
	return nil
}

type fakeTurn struct {
	cancelled bool
}

func (t *fakeTurn) Cancel() error {
	t.cancelled = true
	return nil
}
