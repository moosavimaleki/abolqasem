package claude

import (
	"context"
	"testing"
)

func TestSessionManagerReusesSessionAndUpdatesModelAndPermissionMode(t *testing.T) {
	handle := &fakeClaudeSession{}
	starts := 0
	manager := NewSessionManager(SessionStarterFunc(func(context.Context, StartSessionArgs) (SessionHandle, error) {
		starts++
		return handle, nil
	}))

	if _, err := manager.StartTurn(context.Background(), StartSessionArgs{
		ChatID:    "chat-1",
		LocalPath: "/tmp/project",
		Model:     "claude-opus-4-7",
		Effort:    "high",
		PlanMode:  false,
	}, "first prompt"); err != nil {
		t.Fatalf("StartTurn returned error: %v", err)
	}
	if _, err := manager.StartTurn(context.Background(), StartSessionArgs{
		ChatID:    "chat-1",
		LocalPath: "/tmp/project",
		Model:     "claude-sonnet-4-6",
		Effort:    "high",
		PlanMode:  true,
	}, "second prompt"); err != nil {
		t.Fatalf("second StartTurn returned error: %v", err)
	}

	if starts != 1 {
		t.Fatalf("expected one session start, got %d", starts)
	}
	if handle.modelSet != "claude-sonnet-4-6" {
		t.Fatalf("expected model update, got %q", handle.modelSet)
	}
	if !handle.permissionModeSet {
		t.Fatal("expected permission mode update")
	}
	if len(handle.prompts) != 2 || handle.prompts[1] != "second prompt" {
		t.Fatalf("unexpected prompts: %#v", handle.prompts)
	}
}

func TestSessionManagerRestartsForForkSession(t *testing.T) {
	var handles []*fakeClaudeSession
	manager := NewSessionManager(SessionStarterFunc(func(context.Context, StartSessionArgs) (SessionHandle, error) {
		handle := &fakeClaudeSession{}
		handles = append(handles, handle)
		return handle, nil
	}))

	if _, err := manager.StartTurn(context.Background(), StartSessionArgs{
		ChatID:    "chat-1",
		LocalPath: "/tmp/project",
		Model:     "claude-opus-4-7",
		Effort:    "high",
	}, "first prompt"); err != nil {
		t.Fatalf("StartTurn returned error: %v", err)
	}
	if _, err := manager.StartTurn(context.Background(), StartSessionArgs{
		ChatID:      "chat-1",
		LocalPath:   "/tmp/project",
		Model:       "claude-opus-4-7",
		Effort:      "high",
		ForkSession: true,
	}, "fork prompt"); err != nil {
		t.Fatalf("fork StartTurn returned error: %v", err)
	}

	if len(handles) != 2 {
		t.Fatalf("expected two sessions, got %d", len(handles))
	}
	if !handles[0].closed {
		t.Fatal("expected first session to be closed")
	}
}

type fakeClaudeSession struct {
	prompts           []string
	modelSet          string
	permissionModeSet bool
	interrupted       bool
	closed            bool
}

func (s *fakeClaudeSession) SendPrompt(_ context.Context, content string) error {
	s.prompts = append(s.prompts, content)
	return nil
}

func (s *fakeClaudeSession) SetModel(_ context.Context, model string) error {
	s.modelSet = model
	return nil
}

func (s *fakeClaudeSession) SetPermissionMode(_ context.Context, planMode bool) error {
	s.permissionModeSet = planMode
	return nil
}

func (s *fakeClaudeSession) Interrupt(context.Context) error {
	s.interrupted = true
	return nil
}

func (s *fakeClaudeSession) Close() error {
	s.closed = true
	return nil
}
