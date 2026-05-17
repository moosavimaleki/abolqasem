package sessioninterop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func exportClaudeSession(args ExportArgs) (ExportResult, error) {
	token := generateSessionToken()
	projectDir := filepath.Join(claudeRootDir(), "projects", claudeProjectSlug(args.LocalPath))
	transcriptPath := filepath.Join(projectDir, token+".jsonl")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return ExportResult{}, err
	}
	file, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return ExportResult{}, err
	}
	defer file.Close()
	var parentUUID any
	for index, entry := range args.Entries {
		records := claudeRecordsFromEntry(token, args.LocalPath, entry, index, &parentUUID)
		for _, record := range records {
			data, err := json.Marshal(record)
			if err != nil {
				return ExportResult{}, err
			}
			if _, err := file.Write(append(data, '\n')); err != nil {
				return ExportResult{}, err
			}
		}
	}
	return ExportResult{SessionToken: token, TranscriptPath: transcriptPath, ProjectPath: projectDir}, nil
}

func claudeRecordsFromEntry(sessionToken string, cwd string, entry readmodels.TranscriptEntry, index int, parentUUID *any) []map[string]any {
	createdAt := entryTimestampRFC3339(entry)
	uuid := generateSessionToken()
	base := map[string]any{
		"parentUuid":  *parentUUID,
		"isSidechain": false,
		"timestamp":   createdAt,
		"cwd":         cwd,
		"sessionId":   sessionToken,
		"uuid":        uuid,
	}
	emit := func(records []map[string]any) []map[string]any {
		if len(records) > 0 {
			*parentUUID = uuid
		}
		return records
	}
	switch transcript.Kind(entry) {
	case transcript.KindUserPrompt:
		return emit([]map[string]any{mergeMap(base, map[string]any{
			"type": "user",
			"message": map[string]any{
				"role":    "user",
				"content": stringValueAny(entry["content"]),
			},
		})})
	case transcript.KindAssistantText:
		return emit([]map[string]any{mergeMap(base, map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role":  "assistant",
				"model": "converted",
				"content": []map[string]any{{
					"type": "text",
					"text": stringValueAny(entry["text"]),
				}},
			},
		})})
	case transcript.KindToolCall:
		tool, _ := entry["tool"].(map[string]any)
		return emit([]map[string]any{mergeMap(base, map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role":  "assistant",
				"model": "converted",
				"content": []map[string]any{{
					"type":  "tool_use",
					"id":    stringValueAny(tool["toolId"]),
					"name":  stringValueAny(tool["toolName"]),
					"input": tool["input"],
				}},
			},
		})})
	case transcript.KindToolResult:
		return emit([]map[string]any{mergeMap(base, map[string]any{
			"type": "user",
			"message": map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": stringValueAny(entry["toolId"]),
					"content":     stringValueAny(entry["content"]),
					"is_error":    boolValueAny(entry["isError"]),
				}},
			},
		})})
	case transcript.KindCompactBoundary:
		return emit([]map[string]any{mergeMap(base, map[string]any{
			"type":    "system",
			"subtype": "context_cleared",
			"content": "conversation compacted",
		})})
	case transcript.KindCompactSummary:
		return emit([]map[string]any{mergeMap(base, map[string]any{
			"type":    "system",
			"subtype": "compact_summary",
			"content": stringValueAny(entry["summary"]),
		})})
	default:
		return nil
	}
}

func entryTimestampRFC3339(entry readmodels.TranscriptEntry) string {
	if createdAt, ok := entry["createdAt"].(float64); ok && createdAt > 0 {
		return time.UnixMilli(int64(createdAt)).UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func entryID(entry readmodels.TranscriptEntry, prefix string, index int) string {
	if value, ok := entry["_id"].(string); ok && value != "" {
		return value
	}
	return prefix + "-" + generateSessionToken()
}

func mergeMap(base map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func stringValueAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		data, _ := json.Marshal(typed)
		return string(data)
	}
}

func boolValueAny(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}
