package codex

import (
	"encoding/json"
	"testing"

	codexrpc "ai-agent-manager/internal/providers/codex/rpc"
)

func TestStreamNormalizerMapsCoreTurnTranscriptEvents(t *testing.T) {
	normalizer := NewStreamNormalizer()
	notifications := []codexrpc.Notification{
		notification("thread/started", map[string]any{
			"thread": map[string]any{"id": "thread-1"},
		}),
		notification("item/started", map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"item": map[string]any{
				"type":    "commandExecution",
				"id":      "call-1",
				"command": "/bin/zsh -lc pwd",
				"status":  "inProgress",
			},
		}),
		notification("item/completed", map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"item": map[string]any{
				"type":             "commandExecution",
				"id":               "call-1",
				"command":          "/bin/zsh -lc pwd",
				"status":           "completed",
				"aggregatedOutput": "/tmp/project\n",
				"exitCode":         0,
			},
		}),
		notification("item/completed", map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"item": map[string]any{
				"type":  "agentMessage",
				"id":    "msg-1",
				"text":  "/tmp/project",
				"phase": "final_answer",
			},
		}),
		notification("turn/completed", map[string]any{
			"threadId": "thread-1",
			"turn": map[string]any{
				"id":     "turn-1",
				"status": "completed",
				"error":  nil,
			},
		}),
	}

	var events []HarnessEvent
	for _, item := range notifications {
		events = append(events, normalizer.HandleNotification(item)...)
	}
	if events[0].Type != "session_token" || events[0].SessionToken != "thread-1" {
		t.Fatalf("unexpected session token event: %#v", events[0])
	}
	var kinds []string
	for _, event := range events {
		if event.Type == "transcript" {
			kinds = append(kinds, event.Entry["kind"].(string))
		}
	}
	expected := []string{"tool_call", "tool_result", "assistant_text", "result"}
	if !equalStringSlices(kinds, expected) {
		t.Fatalf("expected transcript kinds %#v, got %#v", expected, kinds)
	}
}

func TestStreamNormalizerMapsTokenUsage(t *testing.T) {
	events := NewStreamNormalizer().HandleNotification(notification("thread/tokenUsage/updated", map[string]any{
		"threadId": "thread-usage",
		"turnId":   "turn-usage",
		"tokenUsage": map[string]any{
			"total": map[string]any{
				"inputTokens":           11833,
				"cachedInputTokens":     3456,
				"outputTokens":          6,
				"reasoningOutputTokens": 0,
				"totalTokens":           11839,
			},
			"last": map[string]any{
				"inputTokens":           120,
				"cachedInputTokens":     0,
				"outputTokens":          6,
				"reasoningOutputTokens": 0,
				"totalTokens":           126,
			},
			"modelContextWindow": 258400,
		},
	}))
	if len(events) != 1 || events[0].Type != "transcript" {
		t.Fatalf("unexpected events: %#v", events)
	}
	if events[0].Entry["kind"] != "context_window_updated" {
		t.Fatalf("unexpected entry: %#v", events[0].Entry)
	}
	usage := events[0].Entry["usage"].(map[string]any)
	if usage["usedTokens"] != float64(126) {
		t.Fatalf("expected used tokens 126, got %#v", usage["usedTokens"])
	}
	if usage["totalProcessedTokens"] != float64(11839) {
		t.Fatalf("expected total processed tokens, got %#v", usage["totalProcessedTokens"])
	}
	if usage["maxTokens"] != float64(258400) {
		t.Fatalf("expected max tokens, got %#v", usage["maxTokens"])
	}
	if usage["compactsAutomatically"] != true {
		t.Fatalf("expected automatic compaction flag, got %#v", usage["compactsAutomatically"])
	}
}

func TestStreamNormalizerMapsCompaction(t *testing.T) {
	events := NewStreamNormalizer().HandleNotification(notification("thread/compacted", map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
	}))
	if len(events) != 1 || events[0].Entry["kind"] != "compact_boundary" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func notification(method string, params any) codexrpc.Notification {
	data, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return codexrpc.Notification{Method: method, Params: data}
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
