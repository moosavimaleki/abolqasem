package codex

import (
	"encoding/json"

	codexrpc "abolqasem/internal/providers/codex/rpc"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
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
	case "account/rateLimits/updated":
		entry := rateLimitEntry(notification.Params)
		if entry == nil {
			return nil
		}
		return []HarnessEvent{{Type: "transcript", Entry: entry}}
	case "thread/compacted":
		return []HarnessEvent{{Type: "transcript", Entry: transcript.New(transcript.KindCompactBoundary, nil)}}
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
	case "plan":
		return planItemEvents(params.Item)
	case "commandExecution":
		command := asString(params.Item["command"])
		if command == "" {
			return nil
		}
		return []HarnessEvent{{
			Type: "transcript",
			Entry: transcript.New(transcript.KindToolCall, map[string]any{
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
	case "fileChange":
		return []HarnessEvent{{
			Type: "transcript",
			Entry: transcript.New(transcript.KindToolCall, map[string]any{
				"tool": map[string]any{
					"kind":     "tool",
					"toolKind": "unknown_tool",
					"toolName": "Codex file changes",
					"toolId":   asString(params.Item["id"]),
					"input": map[string]any{
						"changes": params.Item["changes"],
					},
				},
			}),
		}}
	default:
		return nil
	}
}

func planItemEvents(item map[string]any) []HarnessEvent {
	text := firstStringValue(item, "text", "plan", "content", "summary")
	if text == "" {
		return nil
	}
	return []HarnessEvent{{
		Type: "transcript",
		Entry: transcript.New(transcript.KindToolCall, map[string]any{"tool": map[string]any{
			"kind":     "tool",
			"toolKind": "exit_plan_mode",
			"toolName": "Plan",
			"toolId":   asString(item["id"]),
			"input":    map[string]any{"plan": text},
		}}),
	}}
}

func itemCompletedEvents(raw json.RawMessage) []HarnessEvent {
	var params struct {
		Item map[string]any `json:"item"`
	}
	if decodeParams(raw, &params) != nil || params.Item == nil {
		return nil
	}
	switch asString(params.Item["type"]) {
	case "plan":
		return planItemEvents(params.Item)
	case "agentMessage":
		text := asString(params.Item["text"])
		if text == "" {
			return nil
		}
		return []HarnessEvent{{
			Type:  "transcript",
			Entry: transcript.New(transcript.KindAssistantText, map[string]any{"text": text}),
		}}
	case "commandExecution":
		return []HarnessEvent{{
			Type: "transcript",
			Entry: transcript.New(transcript.KindToolResult, map[string]any{
				"toolId":  asString(params.Item["id"]),
				"content": asString(params.Item["aggregatedOutput"]),
				"isError": asFloat(params.Item["exitCode"]) != 0,
			}),
		}}
	case "fileChange":
		return []HarnessEvent{{
			Type: "transcript",
			Entry: transcript.New(transcript.KindToolResult, map[string]any{
				"toolId":  asString(params.Item["id"]),
				"content": map[string]any{"changes": params.Item["changes"], "status": asString(params.Item["status"])},
				"isError": asString(params.Item["status"]) == "failed",
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
	return transcript.New(transcript.KindResult, map[string]any{
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
	return transcript.New(transcript.KindContextWindowUpdated, map[string]any{
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

func rateLimitEntry(raw json.RawMessage) readmodels.TranscriptEntry {
	var params struct {
		RateLimits map[string]any `json:"rateLimits"`
	}
	if decodeParams(raw, &params) != nil || params.RateLimits == nil {
		return nil
	}
	snapshot := normalizeRateLimitSnapshot(params.RateLimits)
	if snapshot == nil {
		return nil
	}
	return transcript.New(transcript.KindRateLimitUpdated, map[string]any{
		"rateLimits": snapshot,
	})
}

func normalizeRateLimitSnapshot(raw map[string]any) map[string]any {
	primary := normalizeRateLimitWindow(asMap(firstNonNil(raw["primary"])))
	secondary := normalizeRateLimitWindow(asMap(firstNonNil(raw["secondary"])))
	if primary == nil && secondary == nil {
		return nil
	}
	return map[string]any{
		"limitId":              firstStringValue(raw, "limitId", "limit_id"),
		"limitName":            firstStringValue(raw, "limitName", "limit_name"),
		"primary":              primary,
		"secondary":            secondary,
		"credits":              normalizeRateLimitCredits(asMap(firstNonNil(raw["credits"]))),
		"planType":             firstStringValue(raw, "planType", "plan_type"),
		"rateLimitReachedType": firstStringValue(raw, "rateLimitReachedType", "rate_limit_reached_type"),
	}
}

func normalizeRateLimitCredits(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	result := map[string]any{}
	if value, ok := raw["hasCredits"].(bool); ok {
		result["hasCredits"] = value
	} else if value, ok := raw["has_credits"].(bool); ok {
		result["hasCredits"] = value
	}
	if value, ok := raw["unlimited"].(bool); ok {
		result["unlimited"] = value
	}
	balance := firstStringValue(raw, "balance")
	if balance != "" {
		result["balance"] = balance
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeRateLimitWindow(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	usedPercent, ok := firstNumberValue(raw, "usedPercent", "used_percent")
	if !ok {
		return nil
	}
	windowDurationMins, hasWindowDuration := firstNumberValue(raw, "windowDurationMins", "window_minutes")
	resetsAt, hasResetsAt := firstNumberValue(raw, "resetsAt", "resets_at")
	result := map[string]any{
		"usedPercent": usedPercent,
	}
	if hasWindowDuration {
		result["windowDurationMins"] = windowDurationMins
	}
	if hasResetsAt {
		result["resetsAt"] = resetsAt
	}
	return result
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

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := asString(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstNumberValue(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if parsed, ok := numberValue(value); ok {
				return parsed, true
			}
		}
	}
	return 0, false
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func tokenValue(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return asFloat(value)
		}
	}
	return 0
}
