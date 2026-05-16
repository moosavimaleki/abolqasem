package codex

import (
	"encoding/json"
	"time"

	codexrpc "ai-agent-manager/internal/providers/codex/rpc"
	"ai-agent-manager/internal/workspace/readmodels"
)

type HarnessEvent struct {
	Type         string                     `json:"type"`
	SessionToken string                     `json:"sessionToken,omitempty"`
	Entry        readmodels.TranscriptEntry `json:"entry,omitempty"`
}

type StreamNormalizer struct{}

func NewStreamNormalizer() *StreamNormalizer {
	return &StreamNormalizer{}
}

func (n *StreamNormalizer) HandleNotification(notification codexrpc.Notification) []HarnessEvent {
	switch notification.Method {
	case "thread/started":
		var params struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if decodeParams(notification.Params, &params) != nil || params.Thread.ID == "" {
			return nil
		}
		return []HarnessEvent{{Type: "session_token", SessionToken: params.Thread.ID}}
	case "thread/tokenUsage/updated":
		entry := contextWindowEntry(notification.Params)
		if entry == nil {
			return nil
		}
		return []HarnessEvent{{Type: "transcript", Entry: entry}}
	case "thread/compacted":
		return []HarnessEvent{{Type: "transcript", Entry: transcriptEntry("compact_boundary", nil)}}
	case "item/started":
		return itemStartedEvents(notification.Params)
	case "item/completed":
		return itemCompletedEvents(notification.Params)
	case "turn/completed":
		return []HarnessEvent{{Type: "transcript", Entry: turnCompletedEntry(notification.Params)}}
	default:
		return nil
	}
}

func itemStartedEvents(raw json.RawMessage) []HarnessEvent {
	var params struct {
		Item map[string]any `json:"item"`
	}
	if decodeParams(raw, &params) != nil || params.Item == nil {
		return nil
	}
	switch asString(params.Item["type"]) {
	case "commandExecution":
		command := asString(params.Item["command"])
		if command == "" {
			return nil
		}
		return []HarnessEvent{{
			Type: "transcript",
			Entry: transcriptEntry("tool_call", map[string]any{
				"tool": map[string]any{
					"kind":     "tool",
					"toolKind": "bash",
					"toolName": "Bash",
					"toolId":   asString(params.Item["id"]),
					"input": map[string]any{
						"command": command,
					},
				},
			}),
		}}
	default:
		return nil
	}
}

func itemCompletedEvents(raw json.RawMessage) []HarnessEvent {
	var params struct {
		Item map[string]any `json:"item"`
	}
	if decodeParams(raw, &params) != nil || params.Item == nil {
		return nil
	}
	switch asString(params.Item["type"]) {
	case "agentMessage":
		text := asString(params.Item["text"])
		if text == "" {
			return nil
		}
		return []HarnessEvent{{
			Type:  "transcript",
			Entry: transcriptEntry("assistant_text", map[string]any{"text": text}),
		}}
	case "commandExecution":
		return []HarnessEvent{{
			Type: "transcript",
			Entry: transcriptEntry("tool_result", map[string]any{
				"toolId":  asString(params.Item["id"]),
				"content": asString(params.Item["aggregatedOutput"]),
				"isError": asFloat(params.Item["exitCode"]) != 0,
			}),
		}}
	default:
		return nil
	}
}

func turnCompletedEntry(raw json.RawMessage) readmodels.TranscriptEntry {
	var params struct {
		Turn struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	_ = decodeParams(raw, &params)

	isCancelled := params.Turn.Status == "interrupted"
	isError := params.Turn.Status == "failed"
	result := ""
	if params.Turn.Error != nil {
		result = params.Turn.Error.Message
	}
	subtype := "success"
	if isCancelled {
		subtype = "cancelled"
	} else if isError {
		subtype = "error"
	}
	return transcriptEntry("result", map[string]any{
		"subtype":    subtype,
		"isError":    isError,
		"durationMs": float64(0),
		"result":     result,
	})
}

func contextWindowEntry(raw json.RawMessage) readmodels.TranscriptEntry {
	var params struct {
		TokenUsage map[string]any `json:"tokenUsage"`
	}
	if decodeParams(raw, &params) != nil || params.TokenUsage == nil {
		return nil
	}
	total := asMap(firstNonNil(params.TokenUsage["total"], params.TokenUsage["total_token_usage"]))
	last := asMap(firstNonNil(params.TokenUsage["last"], params.TokenUsage["last_token_usage"]))
	usedTokens := tokenValue(last, "totalTokens", "total_tokens")
	return transcriptEntry("context_window_updated", map[string]any{
		"usage": map[string]any{
			"usedTokens":            usedTokens,
			"totalProcessedTokens":  tokenValue(total, "totalTokens", "total_tokens"),
			"maxTokens":             tokenValue(params.TokenUsage, "modelContextWindow", "model_context_window"),
			"inputTokens":           tokenValue(last, "inputTokens", "input_tokens"),
			"cachedInputTokens":     tokenValue(last, "cachedInputTokens", "cached_input_tokens"),
			"outputTokens":          tokenValue(last, "outputTokens", "output_tokens"),
			"reasoningOutputTokens": tokenValue(last, "reasoningOutputTokens", "reasoning_output_tokens"),
			"lastUsedTokens":        usedTokens,
			"compactsAutomatically": true,
		},
	})
}

func transcriptEntry(kind string, fields map[string]any) readmodels.TranscriptEntry {
	entry := readmodels.TranscriptEntry{
		"_id":       kind + "-" + time.Now().UTC().Format("20060102150405.000000000"),
		"createdAt": float64(time.Now().UnixMilli()),
		"kind":      kind,
	}
	for key, value := range fields {
		entry[key] = value
	}
	return entry
}

func decodeParams(raw json.RawMessage, target any) error {
	return json.Unmarshal(raw, target)
}

func asString(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func asFloat(value any) float64 {
	if typed, ok := value.(float64); ok {
		return typed
	}
	return 0
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func tokenValue(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return asFloat(value)
		}
	}
	return 0
}
