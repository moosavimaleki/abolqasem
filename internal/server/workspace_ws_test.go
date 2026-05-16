package server

import (
	"encoding/json"
	"testing"

	"ai-agent-manager/internal/workspace/protocol"
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
