package sessioninterop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func exportCodexSession(args ExportArgs) (ExportResult, error) {
	threadID := codexThreadID()
	now := time.Now().UTC()
	projectDir := filepath.Join(codexRootDir(), "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	transcriptPath := filepath.Join(projectDir, fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), threadID))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return ExportResult{}, err
	}
	file, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return ExportResult{}, err
	}
	defer file.Close()
	meta := map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"type":      "session_meta",
		"payload": map[string]any{
			"id":          threadID,
			"timestamp":   now.Format(time.RFC3339),
			"cwd":         args.LocalPath,
			"originator":  "ai-agent-manager",
			"cli_version": "converted",
			"source":      "abolqasem-convert",
		},
	}
	if err := writeJSONLRecord(file, meta); err != nil {
		return ExportResult{}, err
	}
	for index, entry := range args.Entries {
		records := codexRecordsFromEntry(entry, index)
		for _, record := range records {
			if err := writeJSONLRecord(file, record); err != nil {
				return ExportResult{}, err
			}
		}
	}
	return ExportResult{SessionToken: threadID, TranscriptPath: transcriptPath, ProjectPath: projectDir}, nil
}

func codexRecordsFromEntry(entry readmodels.TranscriptEntry, index int) []map[string]any {
	timestamp := entryTimestampRFC3339(entry)
	switch transcript.Kind(entry) {
	case transcript.KindUserPrompt:
		text := stringValueAny(entry["content"])
		return []map[string]any{{
			"timestamp": timestamp,
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": text,
				}},
			},
		}, {
			"timestamp": timestamp,
			"type":      "event_msg",
			"payload": map[string]any{
				"type":          "user_message",
				"message":       text,
				"images":        []string{},
				"local_images":  []string{},
				"text_elements": []any{},
			},
		}}
	case transcript.KindAssistantText:
		text := stringValueAny(entry["text"])
		return []map[string]any{{
			"timestamp": timestamp,
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": text,
				}},
			},
		}, {
			"timestamp": timestamp,
			"type":      "event_msg",
			"payload": map[string]any{
				"type":    "agent_message",
				"message": text,
			},
		}}
	case transcript.KindToolCall:
		tool, _ := entry["tool"].(map[string]any)
		arguments, _ := json.Marshal(tool["input"])
		return []map[string]any{{
			"timestamp": timestamp,
			"type":      "response_item",
			"payload": map[string]any{
				"type":      "function_call",
				"call_id":   stringValueAny(tool["toolId"]),
				"name":      stringValueAny(tool["toolName"]),
				"arguments": string(arguments),
			},
		}}
	case transcript.KindToolResult:
		return []map[string]any{{
			"timestamp": timestamp,
			"type":      "response_item",
			"payload": map[string]any{
				"type":    "function_call_output",
				"call_id": stringValueAny(entry["toolId"]),
				"output":  stringValueAny(entry["content"]),
			},
		}}
	case transcript.KindCompactBoundary:
		return nil
	case transcript.KindCompactSummary:
		return []map[string]any{{
			"timestamp": timestamp,
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": "Summary of earlier conversation:\n" + stringValueAny(entry["summary"]),
				}},
			},
		}}
	default:
		_ = index
		return nil
	}
}

func writeJSONLRecord(file *os.File, record map[string]any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}
