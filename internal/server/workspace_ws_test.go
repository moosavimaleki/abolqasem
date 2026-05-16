package server

import (
	"encoding/json"
	"testing"

	"ai-agent-manager/internal/workspace/protocol"
)

func TestWorkspaceCommandRoutingHandlesSystemPing(t *testing.T) {
	conn := &workspaceConnection{subscriptions: map[string]string{}, hub: newWorkspaceTerminalHub()}

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
	conn := &workspaceConnection{subscriptions: map[string]string{}, hub: newWorkspaceTerminalHub()}
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

func mustWorkspaceRawCommand(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	return data
}
