package sessioninterop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

// exportOpenCodeSession writes OpenCode's portable export shape. OpenCode does
// not support importing that file as a native session, so the first OpenCode
// turn uses it as normalized context and creates a genuine ses_* session.
func exportOpenCodeSession(args ExportArgs) (ExportResult, error) {
	token := "converted-" + generateSessionToken()
	directory := filepath.Join(state.GetStateDir(), "opencode", "converted")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return ExportResult{}, err
	}
	path := filepath.Join(directory, token+".json")
	messages := make([]map[string]any, 0, len(args.Entries))
	for _, entry := range args.Entries {
		if message, ok := openCodeMessageFromEntry(entry); ok {
			messages = append(messages, message)
		}
	}
	payload := map[string]any{
		"info": map[string]any{
			"id":        token,
			"title":     args.Title,
			"directory": args.LocalPath,
			"time":      map[string]any{"created": time.Now().UnixMilli()},
		},
		"messages": messages,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ExportResult{}, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{SessionToken: token, TranscriptPath: path, ProjectPath: directory}, nil
}

func openCodeMessageFromEntry(entry readmodels.TranscriptEntry) (map[string]any, bool) {
	created := time.Now().UnixMilli()
	if value, ok := entry["createdAt"].(float64); ok && value > 0 {
		created = int64(value)
	}
	role, text := "", ""
	switch transcript.Kind(entry) {
	case transcript.KindUserPrompt:
		role, text = "user", stringValueAny(entry["content"])
	case transcript.KindAssistantText:
		role, text = "assistant", stringValueAny(entry["text"])
	case transcript.KindCompactSummary:
		role, text = "user", "Summary of earlier conversation:\n"+stringValueAny(entry["summary"])
	default:
		return nil, false
	}
	if text == "" {
		return nil, false
	}
	return map[string]any{
		"info":  map[string]any{"role": role, "time": map[string]any{"created": created}},
		"parts": []map[string]any{{"type": "text", "text": text}},
	}, true
}
