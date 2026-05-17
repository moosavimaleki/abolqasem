package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/protocol"
	"ai-agent-manager/internal/workspace/transcript"
)

func TestWorkspaceCommandRoutingHandlesSystemPing(t *testing.T) {
	conn := newTestWorkspaceConnection(nil)

	response := conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "cmd-1",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandSystemPing}),
	})

	if response == nil || response.Type != protocol.EnvelopeAck || response.ID != "cmd-1" {
		t.Fatalf("unexpected response: %#v", response)
	}
	result, ok := response.Result.(map[string]any)
	if !ok || result["ok"] != true {
		t.Fatalf("unexpected ping result: %#v", response.Result)
	}
}

func TestWorkspaceCommandRoutingCreatesProjectAndChat(t *testing.T) {
	withWorkspaceComposerStore(t)
	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()

	projectResponse := conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "project-open",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":      protocol.CommandProjectOpen,
			"localPath": projectDir,
			"title":     "Project",
		}),
	})
	if projectResponse == nil || projectResponse.Type != protocol.EnvelopeAck {
		t.Fatalf("unexpected project response: %#v", projectResponse)
	}
	projectResult, ok := projectResponse.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected project result: %#v", projectResponse.Result)
	}
	projectID, ok := projectResult["projectId"].(string)
	if !ok || projectID == "" {
		t.Fatalf("expected project id in result, got %#v", projectResult)
	}

	chatResponse := conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "chat-create",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":      protocol.CommandChatCreate,
			"projectId": projectID,
		}),
	})
	if chatResponse == nil || chatResponse.Type != protocol.EnvelopeAck {
		t.Fatalf("unexpected chat response: %#v", chatResponse)
	}
	chatResult, ok := chatResponse.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected chat result: %#v", chatResponse.Result)
	}
	chatID, ok := chatResult["chatId"].(string)
	if !ok || chatID == "" {
		t.Fatalf("expected chat id in result, got %#v", chatResult)
	}
}

func TestWorkspaceCommandRoutingHandlesProjectAndChatMutations(t *testing.T) {
	withWorkspaceComposerStore(t)
	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()

	projectID := mustCreateWorkspaceProject(t, conn, projectDir)
	assertWorkspaceAck(t, conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "project-rename",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":      protocol.CommandProjectRename,
			"projectId": projectID,
			"title":     "Sidebar Name",
		}),
	}), "project-rename")

	chatID := mustCreateWorkspaceChat(t, conn, projectID)
	assertWorkspaceAck(t, conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "chat-rename",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":   protocol.CommandChatRename,
			"chatId": chatID,
			"title":  "Renamed Chat",
		}),
	}), "chat-rename")
	forkResponse := conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "chat-fork",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandChatFork, "chatId": chatID}),
	})
	assertWorkspaceAck(t, forkResponse, "chat-fork")
	forkResult, ok := forkResponse.Result.(map[string]any)
	if !ok || forkResult["chatId"] == "" {
		t.Fatalf("expected fork chat id, got %#v", forkResponse.Result)
	}
	assertWorkspaceAck(t, conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "chat-archive",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandChatArchive, "chatId": chatID}),
	}), "chat-archive")
	assertWorkspaceAck(t, conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "chat-unarchive",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandChatUnarchive, "chatId": chatID}),
	}), "chat-unarchive")
	assertWorkspaceAck(t, conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "chat-delete",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandChatDelete, "chatId": chatID}),
	}), "chat-delete")

	state, err := workspaceStore().LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	project := state.ProjectsByID[projectID]
	if project.SidebarTitle == nil || *project.SidebarTitle != "Sidebar Name" {
		t.Fatalf("expected renamed project sidebar title, got %#v", project)
	}
	chat := state.ChatsByID[chatID]
	if chat.Title != "Renamed Chat" || chat.DeletedAt == 0 {
		t.Fatalf("expected renamed deleted chat, got %#v", chat)
	}
}

func TestWorkspaceCommandRoutingHandlesGitAndHistoryCommands(t *testing.T) {
	withWorkspaceComposerStore(t)
	conn := newTestWorkspaceConnection(nil)
	projectID := mustCreateWorkspaceProject(t, conn, t.TempDir())
	chatID := mustCreateWorkspaceChat(t, conn, projectID)

	gitResponse := conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "git-init",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandChatInitGit, "chatId": chatID}),
	})
	assertWorkspaceAck(t, gitResponse, "git-init")

	messageResponse := conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "commit-message",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":   protocol.CommandChatGenerateCommitMessage,
			"chatId": chatID,
			"paths":  []string{"internal/server/workspace_ws.go"},
		}),
	})
	assertWorkspaceAck(t, messageResponse, "commit-message")
	result, ok := messageResponse.Result.(map[string]any)
	if !ok || result["subject"] == "" {
		t.Fatalf("expected generated commit message, got %#v", messageResponse.Result)
	}

	historyResponse := conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "history",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":         protocol.CommandChatLoadHistory,
			"chatId":       chatID,
			"beforeCursor": "",
			"limit":        25,
		}),
	})
	assertWorkspaceAck(t, historyResponse, "history")
}

func TestWorkspaceCommandRoutingForksGeminiChatIntoNativeSession(t *testing.T) {
	withWorkspaceComposerStore(t)
	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(homeDir, ".gemini"))

	projectID := mustCreateWorkspaceProject(t, conn, projectDir)
	chatID := mustCreateWorkspaceChat(t, conn, projectID)
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   chatID,
		"provider": "gemini",
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 102, map[string]any{
		"chatId": chatID,
		"entry":  transcript.New(transcript.KindUserPrompt, map[string]any{"content": "hello from gemini"}),
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 103, map[string]any{
		"chatId": chatID,
		"entry":  transcript.New(transcript.KindAssistantText, map[string]any{"text": "assistant reply"}),
	})

	forkResponse := conn.handle(protocol.ClientEnvelope{
		V:       protocol.ProtocolVersion,
		Type:    protocol.EnvelopeCommand,
		ID:      "chat-fork-gemini",
		Command: mustWorkspaceRawCommand(t, map[string]any{"type": protocol.CommandChatFork, "chatId": chatID}),
	})
	assertWorkspaceAck(t, forkResponse, "chat-fork-gemini")
	forkResult, ok := forkResponse.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected fork result map, got %#v", forkResponse.Result)
	}
	forkChatID, _ := forkResult["chatId"].(string)
	if forkChatID == "" {
		t.Fatalf("expected fork chat id, got %#v", forkResult)
	}

	state, err := workspaceStore().LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	forkChat := state.ChatsByID[forkChatID]
	if forkChat.SessionToken == nil || *forkChat.SessionToken == "" {
		t.Fatalf("expected native gemini session token on fork, got %#v", forkChat)
	}
	if forkChat.PendingForkSessionToken != nil {
		t.Fatalf("expected no pending fork token on gemini native fork, got %#v", forkChat.PendingForkSessionToken)
	}
	registryPath := filepath.Join(homeDir, ".gemini", "projects.json")
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("expected gemini registry after fork export: %v", err)
	}
}

func TestWorkspaceSubscriptionRegistryBroadcastsOnlyRelatedTopics(t *testing.T) {
	withWorkspaceComposerStore(t)
	withWorkspaceConnectionRegistry(t)

	sidebarEnvelopes := []protocol.ServerEnvelope{}
	updateEnvelopes := []protocol.ServerEnvelope{}
	sidebarConn := newTestWorkspaceConnection(func(envelope protocol.ServerEnvelope) error {
		sidebarEnvelopes = append(sidebarEnvelopes, envelope)
		return nil
	})
	updateConn := newTestWorkspaceConnection(func(envelope protocol.ServerEnvelope) error {
		updateEnvelopes = append(updateEnvelopes, envelope)
		return nil
	})

	sidebarResponse := sidebarConn.handle(protocol.ClientEnvelope{
		V:     protocol.ProtocolVersion,
		Type:  protocol.EnvelopeSubscribe,
		ID:    "sub-sidebar",
		Topic: &protocol.SubscriptionTopic{Type: protocol.TopicSidebar},
	})
	if sidebarResponse == nil || sidebarResponse.Snapshot == nil || sidebarResponse.Snapshot.Type != protocol.SnapshotSidebar {
		t.Fatalf("unexpected sidebar subscribe response: %#v", sidebarResponse)
	}
	updateResponse := updateConn.handle(protocol.ClientEnvelope{
		V:     protocol.ProtocolVersion,
		Type:  protocol.EnvelopeSubscribe,
		ID:    "sub-update",
		Topic: &protocol.SubscriptionTopic{Type: protocol.TopicUpdate},
	})
	if updateResponse == nil || updateResponse.Snapshot == nil || updateResponse.Snapshot.Type != protocol.SnapshotUpdate {
		t.Fatalf("unexpected update subscribe response: %#v", updateResponse)
	}

	workspaceConnections.broadcast("")
	if len(sidebarEnvelopes) != 1 || sidebarEnvelopes[0].Snapshot == nil || sidebarEnvelopes[0].Snapshot.Type != protocol.SnapshotSidebar {
		t.Fatalf("expected sidebar broadcast only for sidebar subscriber, got %#v", sidebarEnvelopes)
	}
	if len(updateEnvelopes) != 0 {
		t.Fatalf("update subscriber received unrelated sidebar/local-project broadcast: %#v", updateEnvelopes)
	}

	workspaceConnections.broadcastUpdate(map[string]any{"status": "idle"})
	if len(updateEnvelopes) != 1 || updateEnvelopes[0].Snapshot == nil || updateEnvelopes[0].Snapshot.Type != protocol.SnapshotUpdate {
		t.Fatalf("expected update broadcast for update subscriber, got %#v", updateEnvelopes)
	}
	if len(sidebarEnvelopes) != 1 {
		t.Fatalf("sidebar subscriber received unrelated update broadcast: %#v", sidebarEnvelopes)
	}
}

func TestWorkspaceSubscriptionUnsubscribeStopsBroadcast(t *testing.T) {
	withWorkspaceComposerStore(t)
	withWorkspaceConnectionRegistry(t)

	envelopes := []protocol.ServerEnvelope{}
	conn := newTestWorkspaceConnection(func(envelope protocol.ServerEnvelope) error {
		envelopes = append(envelopes, envelope)
		return nil
	})
	conn.handle(protocol.ClientEnvelope{
		V:     protocol.ProtocolVersion,
		Type:  protocol.EnvelopeSubscribe,
		ID:    "sub-sidebar",
		Topic: &protocol.SubscriptionTopic{Type: protocol.TopicSidebar},
	})
	conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeUnsubscribe,
		ID:   "sub-sidebar",
	})

	workspaceConnections.broadcast("")
	if len(envelopes) != 0 {
		t.Fatalf("unsubscribed connection received broadcast: %#v", envelopes)
	}
}

func TestWorkspaceAppSettingsSubscriptionBroadcastsToSubscribers(t *testing.T) {
	withWorkspaceComposerStore(t)
	withWorkspaceConnectionRegistry(t)

	envelopes := []protocol.ServerEnvelope{}
	conn := newTestWorkspaceConnection(func(envelope protocol.ServerEnvelope) error {
		envelopes = append(envelopes, envelope)
		return nil
	})
	subscribeResponse := conn.handle(protocol.ClientEnvelope{
		V:     protocol.ProtocolVersion,
		Type:  protocol.EnvelopeSubscribe,
		ID:    "sub-app-settings",
		Topic: &protocol.SubscriptionTopic{Type: protocol.TopicAppSettings},
	})
	if subscribeResponse == nil || subscribeResponse.Snapshot == nil || subscribeResponse.Snapshot.Type != protocol.SnapshotAppSettings {
		t.Fatalf("unexpected app-settings subscribe response: %#v", subscribeResponse)
	}

	workspaceConnections.broadcastAppSettings(map[string]any{"locale": "fa"})
	if len(envelopes) != 1 || envelopes[0].Snapshot == nil || envelopes[0].Snapshot.Type != protocol.SnapshotAppSettings {
		t.Fatalf("expected app-settings broadcast, got %#v", envelopes)
	}
}

func newTestWorkspaceConnection(writeFn func(protocol.ServerEnvelope) error) *workspaceConnection {
	return &workspaceConnection{
		hub:           newWorkspaceTerminalHub(),
		writeFn:       writeFn,
		subscriptions: map[string]workspaceSubscription{},
	}
}

func mustCreateWorkspaceProject(t *testing.T, conn *workspaceConnection, localPath string) string {
	t.Helper()
	response := conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "project-create",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":      protocol.CommandProjectCreate,
			"localPath": localPath,
			"title":     "Project",
		}),
	})
	if response == nil || response.Type != protocol.EnvelopeAck {
		t.Fatalf("unexpected project create response: %#v", response)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected project create result: %#v", response.Result)
	}
	projectID, ok := result["projectId"].(string)
	if !ok || projectID == "" {
		t.Fatalf("expected project id in result, got %#v", result)
	}
	return projectID
}

func mustCreateWorkspaceChat(t *testing.T, conn *workspaceConnection, projectID string) string {
	t.Helper()
	response := conn.handle(protocol.ClientEnvelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.EnvelopeCommand,
		ID:   "chat-create",
		Command: mustWorkspaceRawCommand(t, map[string]any{
			"type":      protocol.CommandChatCreate,
			"projectId": projectID,
		}),
	})
	if response == nil || response.Type != protocol.EnvelopeAck {
		t.Fatalf("unexpected chat create response: %#v", response)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected chat create result: %#v", response.Result)
	}
	chatID, ok := result["chatId"].(string)
	if !ok || chatID == "" {
		t.Fatalf("expected chat id in result, got %#v", result)
	}
	return chatID
}

func assertWorkspaceAck(t *testing.T, response *protocol.ServerEnvelope, id string) {
	t.Helper()
	if response == nil || response.Type != protocol.EnvelopeAck || response.ID != id {
		t.Fatalf("unexpected ack response for %s: %#v", id, response)
	}
}

func withWorkspaceConnectionRegistry(t *testing.T) {
	t.Helper()
	previous := workspaceConnections
	workspaceConnections = newWorkspaceConnectionRegistry()
	t.Cleanup(func() {
		workspaceConnections = previous
	})
}

func mustWorkspaceRawCommand(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	return data
}
