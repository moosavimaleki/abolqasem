package server

import (
	"context"
	"testing"

	"ai-agent-manager/internal/workspace/agent"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func withWorkspaceComposerStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return dir }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})
}

func TestWorkspaceComposerCreatesChatAndSendsPrompt(t *testing.T) {
	withWorkspaceComposerStore(t)

	project, err := workspaceOpenProject("/tmp/project", "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatalf("workspaceCreateChat returned error: %v", err)
	}
	result, err := workspaceAgentCoordinator().Send(context.Background(), agent.SendCommand{
		ChatID:   chat.ID,
		Content:  "hello",
		Provider: "codex",
		Model:    "gpt-5.5",
		PlanMode: true,
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result.ChatID != chat.ID || result.Queued {
		t.Fatalf("unexpected send result: %#v", result)
	}

	snapshot := workspaceChatSnapshot(chat.ID, 10).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.Status != readmodels.StatusStarting && snapshot.Runtime.Status != readmodels.StatusRunning {
		t.Fatalf("expected active status, got %q", snapshot.Runtime.Status)
	}
	if snapshot.Runtime.Provider == nil || *snapshot.Runtime.Provider != "codex" {
		t.Fatalf("expected codex provider, got %#v", snapshot.Runtime.Provider)
	}
	if len(snapshot.Messages) != 1 || transcript.Kind(snapshot.Messages[0]) != transcript.KindUserPrompt {
		t.Fatalf("expected one user prompt, got %#v", snapshot.Messages)
	}
}

func TestWorkspaceComposerQueuesAndCancels(t *testing.T) {
	withWorkspaceComposerStore(t)

	project, err := workspaceOpenProject("/tmp/project", "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatalf("workspaceCreateChat returned error: %v", err)
	}
	if _, err := workspaceAgentCoordinator().Send(context.Background(), agent.SendCommand{ChatID: chat.ID, Content: "first"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	queued, err := workspaceAgentCoordinator().Send(context.Background(), agent.SendCommand{ChatID: chat.ID, Content: "second"})
	if err != nil {
		t.Fatalf("queued Send returned error: %v", err)
	}
	if !queued.Queued || queued.QueuedMessageID == "" {
		t.Fatalf("expected queued send result, got %#v", queued)
	}
	snapshot := workspaceChatSnapshot(chat.ID, 10).(*readmodels.ChatSnapshot)
	if len(snapshot.QueuedMessages) != 1 || snapshot.QueuedMessages[0].Content != "second" {
		t.Fatalf("expected queued message in snapshot, got %#v", snapshot.QueuedMessages)
	}

	if err := workspaceAgentCoordinator().Cancel(chat.ID); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	snapshot = workspaceChatSnapshot(chat.ID, 10).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.Status == readmodels.StatusRunning {
		t.Fatal("expected chat to stop running after cancel")
	}
}
