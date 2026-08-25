package transcript

import "time"

type Entry map[string]any

const (
	KindUserPrompt           = "user_prompt"
	KindSystemInit           = "system_init"
	KindAccountInfo          = "account_info"
	KindAssistantText        = "assistant_text"
	KindToolCall             = "tool_call"
	KindToolResult           = "tool_result"
	KindResult               = "result"
	KindStatus               = "status"
	KindContextWindowUpdated = "context_window_updated"
	KindRateLimitUpdated     = "rate_limit_updated"
	KindCompactBoundary      = "compact_boundary"
	KindCompactSummary       = "compact_summary"
	KindContextCleared       = "context_cleared"
	KindInterrupted          = "interrupted"
	KindCommandExecution     = "command_execution"
	KindFileChange           = "file_change"
	KindTurnPlan             = "turn_plan"
	KindProposedPlan         = "proposed_plan"
	KindTurnActivity         = "turn_activity"
	KindUnknown              = "unknown"
)

func New(kind string, fields map[string]any) Entry {
	now := time.Now()
	entry := Entry{
		"_id":       kind + "-" + now.UTC().Format("20060102150405.000000000"),
		"createdAt": float64(now.UnixMilli()),
		"kind":      kind,
	}
	for key, value := range fields {
		entry[key] = value
	}
	return entry
}

func Kind(entry Entry) string {
	if value, ok := entry["kind"].(string); ok {
		return value
	}
	return ""
}
