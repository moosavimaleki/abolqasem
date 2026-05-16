package protocol

import (
	"encoding/json"
	"testing"
)

func TestDecodeClientEnvelopeCommand(t *testing.T) {
	payload := []byte(`{
		"v": 1,
		"type": "command",
		"id": "cmd-1",
		"command": {
			"type": "chat.send",
			"chatId": "chat-1",
			"content": "hello",
			"provider": "codex",
			"model": "gpt-5.5"
		}
	}`)

	envelope, err := DecodeClientEnvelope(payload)
	if err != nil {
		t.Fatalf("DecodeClientEnvelope returned error: %v", err)
	}

	if envelope.Type != EnvelopeCommand {
		t.Fatalf("expected command envelope, got %q", envelope.Type)
	}
	commandType, err := CommandType(envelope.Command)
	if err != nil {
		t.Fatalf("CommandType returned error: %v", err)
	}
	if commandType != CommandChatSend {
		t.Fatalf("expected %q, got %q", CommandChatSend, commandType)
	}
}

func TestDecodeClientEnvelopeSubscribe(t *testing.T) {
	payload := []byte(`{
		"v": 1,
		"type": "subscribe",
		"id": "sub-1",
		"topic": {
			"type": "chat",
			"chatId": "chat-1",
			"recentLimit": 50
		}
	}`)

	envelope, err := DecodeClientEnvelope(payload)
	if err != nil {
		t.Fatalf("DecodeClientEnvelope returned error: %v", err)
	}
	if envelope.Topic == nil {
		t.Fatal("expected topic")
	}
	if envelope.Topic.Type != TopicChat {
		t.Fatalf("expected %q, got %q", TopicChat, envelope.Topic.Type)
	}
	if envelope.Topic.RecentLimit == nil || *envelope.Topic.RecentLimit != 50 {
		t.Fatalf("expected recentLimit 50, got %#v", envelope.Topic.RecentLimit)
	}
}

func TestServerEnvelopeMatchesKannaShape(t *testing.T) {
	envelope := SnapshotEnvelope("sub-1", SnapshotSidebar, map[string]any{
		"projectGroups": []any{},
	})

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if decoded["v"] != float64(1) {
		t.Fatalf("expected protocol version 1, got %#v", decoded["v"])
	}
	if decoded["type"] != EnvelopeSnapshot {
		t.Fatalf("expected snapshot envelope, got %#v", decoded["type"])
	}
	snapshot, ok := decoded["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("expected snapshot object, got %#v", decoded["snapshot"])
	}
	if snapshot["type"] != SnapshotSidebar {
		t.Fatalf("expected sidebar snapshot, got %#v", snapshot["type"])
	}
	if _, ok := snapshot["data"].(map[string]any); !ok {
		t.Fatalf("expected snapshot data object, got %#v", snapshot["data"])
	}
}

func TestDecodeClientEnvelopeRejectsInvalidCommand(t *testing.T) {
	_, err := DecodeClientEnvelope([]byte(`{"v":1,"type":"command","id":"cmd-1","command":{}}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
