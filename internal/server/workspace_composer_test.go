package server

import (
	"context"
	"testing"

	"ai-agent-manager/internal/providers/catalog"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/agent"
	"ai-agent-manager/internal/workspace/eventstore"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func withWorkspaceComposerStore(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	previousTurnStarterFactory := workspaceTurnStarterFactory
	previousLegacyState := workspaceLoadLegacyState
	workspaceDataDir = func() string { return dir }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	workspaceTurnStarterFactory = func(*eventstore.Store) agent.TurnStarter {
		return agent.TurnStarterFunc(func(context.Context, agent.TurnRequest) (agent.Turn, error) {
			return &workspaceComposerTestTurn{}, nil
		})
	}
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return &state.AppState{Sessions: map[string]state.SessionMeta{}}, nil
	}
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
		workspaceTurnStarterFactory = previousTurnStarterFactory
		workspaceLoadLegacyState = previousLegacyState
	})
}

type workspaceComposerTestTurn struct{}

func (workspaceComposerTestTurn) Cancel() error {
	return nil
}

func (workspaceComposerTestTurn) RespondTool(context.Context, agent.ToolResponse) error {
	return nil
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
	requestedModel := "gpt-5.5"
	expectedModel := catalog.NormalizeServerModel("codex", requestedModel)
	result, err := workspaceAgentCoordinator().Send(context.Background(), agent.SendCommand{
		ChatID:   chat.ID,
		Content:  "hello",
		Provider: "codex",
		Model:    requestedModel,
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
	if len(snapshot.Messages) != 2 {
		t.Fatalf("expected system init and user prompt, got %#v", snapshot.Messages)
	}
	if transcript.Kind(snapshot.Messages[0]) != transcript.KindSystemInit {
		t.Fatalf("expected system init first, got %#v", snapshot.Messages[0])
	}
	if snapshot.Messages[0]["provider"] != "codex" || snapshot.Messages[0]["model"] != expectedModel {
		t.Fatalf("unexpected system init metadata: %#v", snapshot.Messages[0])
	}
	if transcript.Kind(snapshot.Messages[1]) != transcript.KindUserPrompt {
		t.Fatalf("expected user prompt second, got %#v", snapshot.Messages[1])
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

func TestWorkspaceRuntimeEventsUpdateSnapshots(t *testing.T) {
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
	if err := workspaceAppendAssistantText(chat.ID, "working"); err != nil {
		t.Fatalf("workspaceAppendAssistantText returned error: %v", err)
	}
	if err := workspaceAgentCoordinator().SetPendingTool(chat.ID, agent.PendingToolRequest{
		ToolUseID: "tool-1",
		ToolKind:  "ask_user_question",
		Input: map[string]any{
			"questions": []any{},
		},
	}); err != nil {
		t.Fatalf("SetPendingTool returned error: %v", err)
	}

	snapshot := workspaceChatSnapshot(chat.ID, 20).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.Status != readmodels.StatusWaitingForUser {
		t.Fatalf("expected waiting_for_user status, got %q", snapshot.Runtime.Status)
	}
	if !hasTranscriptKind(snapshot.Messages, transcript.KindAssistantText) {
		t.Fatalf("expected assistant text in snapshot, got %#v", snapshot.Messages)
	}
	if !hasTranscriptKind(snapshot.Messages, transcript.KindToolCall) {
		t.Fatalf("expected pending tool call in snapshot, got %#v", snapshot.Messages)
	}

	if err := workspaceAgentCoordinator().RespondTool(context.Background(), agent.ToolResponseCommand{
		ChatID:    chat.ID,
		ToolUseID: "tool-1",
		Result:    map[string]any{"answers": map[string]any{}},
	}); err != nil {
		t.Fatalf("RespondTool returned error: %v", err)
	}
	snapshot = workspaceChatSnapshot(chat.ID, 20).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.Status != readmodels.StatusRunning {
		t.Fatalf("expected running status after tool response, got %q", snapshot.Runtime.Status)
	}
	if !hasTranscriptKind(snapshot.Messages, transcript.KindToolResult) {
		t.Fatalf("expected tool result in snapshot, got %#v", snapshot.Messages)
	}
}

func TestWorkspaceSidebarReflectsRuntimeStatus(t *testing.T) {
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
	sidebar := workspaceSidebarSnapshot().(readmodels.SidebarData)
	if got := sidebar.ProjectGroups[0].Chats[0].Status; got != string(readmodels.StatusStarting) && got != string(readmodels.StatusRunning) {
		t.Fatalf("expected active sidebar status, got %q", got)
	}
	if err := workspaceAgentCoordinator().SetPendingTool(chat.ID, agent.PendingToolRequest{ToolUseID: "tool-1", ToolKind: "exit_plan_mode"}); err != nil {
		t.Fatalf("SetPendingTool returned error: %v", err)
	}
	sidebar = workspaceSidebarSnapshot().(readmodels.SidebarData)
	if got := sidebar.ProjectGroups[0].Chats[0].Status; got != string(readmodels.StatusWaitingForUser) {
		t.Fatalf("expected waiting sidebar status, got %q", got)
	}
	if err := workspaceAgentCoordinator().Cancel(chat.ID); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if err := (&workspaceEventStore{store: workspaceStore()}).RecordTurnFailed(chat.ID, "failed"); err != nil {
		t.Fatalf("RecordTurnFailed returned error: %v", err)
	}
	sidebar = workspaceSidebarSnapshot().(readmodels.SidebarData)
	if got := sidebar.ProjectGroups[0].Chats[0].Status; got != string(readmodels.StatusFailed) {
		t.Fatalf("expected failed sidebar status, got %q", got)
	}
}

func hasTranscriptKind(messages []readmodels.TranscriptEntry, kind string) bool {
	for _, message := range messages {
		if transcript.Kind(message) == kind {
			return true
		}
	}
	return false
}
