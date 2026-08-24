package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

type Adapter struct {
	Executable string
}

type PromptRequest struct {
	CWD          string
	Model        string
	Effort       string
	PlanMode     bool
	SessionToken string
	ForkSession  bool
	Prompt       string
	Env          []string
}

type PromptResult struct {
	Entries      []readmodels.TranscriptEntry
	SessionToken string
}

func NewAdapter(executable string) *Adapter {
	if executable == "" {
		executable = "claude"
	}
	return &Adapter{Executable: executable}
}

func (a *Adapter) BuildArgs(request PromptRequest) []string {
	args := []string{"--print", "--output-format", "stream-json", "--include-partial-messages"}
	if request.Model != "" {
		args = append(args, "--model", request.Model)
	}
	if request.Effort != "" {
		args = append(args, "--effort", request.Effort)
	}
	if request.PlanMode {
		args = append(args, "--permission-mode", "plan")
	} else {
		args = append(args, "--permission-mode", "acceptEdits")
	}
	if request.SessionToken != "" {
		args = append(args, "--resume", request.SessionToken)
	}
	if request.ForkSession {
		args = append(args, "--fork-session")
	}
	args = append(args, request.Prompt)
	return args
}

func (a *Adapter) RunPrompt(ctx context.Context, request PromptRequest) ([]readmodels.TranscriptEntry, error) {
	result, err := a.RunPromptResult(ctx, request)
	return result.Entries, err
}

func (a *Adapter) RunPromptResult(ctx context.Context, request PromptRequest) (PromptResult, error) {
	cmd := exec.CommandContext(ctx, a.Executable, a.BuildArgs(request)...)
	cmd.Env = state.CurrentProviderProxyEnvWithOverrides(request.Env)
	if request.CWD != "" {
		cmd.Dir = request.CWD
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return PromptResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return PromptResult{}, err
	}
	result, parseErr := ParseStreamResult(stdout)
	waitErr := cmd.Wait()
	if parseErr != nil {
		return result, parseErr
	}
	return result, waitErr
}

func ParseStream(reader io.Reader) ([]readmodels.TranscriptEntry, error) {
	result, err := ParseStreamResult(reader)
	return result.Entries, err
}

func ParseStreamResult(reader io.Reader) (PromptResult, error) {
	var result PromptResult
	var entries []readmodels.TranscriptEntry
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			result.Entries = entries
			return result, err
		}
		if token := eventSessionToken(event); token != "" {
			result.SessionToken = token
		}
		entries = append(entries, entriesFromEvent(event)...)
	}
	result.Entries = entries
	if err := scanner.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func entriesFromEvent(event map[string]any) []readmodels.TranscriptEntry {
	switch eventType(event) {
	case "assistant":
		return assistantEntries(event)
	case "user":
		return toolResultEntries(event)
	case "result":
		return []readmodels.TranscriptEntry{transcript.New(transcript.KindResult, map[string]any{
			"subtype":    "success",
			"isError":    false,
			"durationMs": number(event["duration_ms"]),
			"result":     stringValue(event["result"]),
		})}
	default:
		return nil
	}
}

func assistantEntries(event map[string]any) []readmodels.TranscriptEntry {
	message, _ := event["message"].(map[string]any)
	content, _ := message["content"].([]any)
	if len(content) == 0 {
		if text := assistantText(event); text != "" {
			return []readmodels.TranscriptEntry{transcript.New(transcript.KindAssistantText, map[string]any{"text": text})}
		}
		return nil
	}
	entries := make([]readmodels.TranscriptEntry, 0, len(content))
	for _, raw := range content {
		block, _ := raw.(map[string]any)
		switch stringValue(block["type"]) {
		case "text":
			if text := stringValue(block["text"]); text != "" {
				entries = append(entries, transcript.New(transcript.KindAssistantText, map[string]any{"text": text}))
			}
		case "tool_use":
			toolID := stringValue(block["id"])
			if toolID == "" {
				continue
			}
			name := stringValue(block["name"])
			entries = append(entries, transcript.New(transcript.KindToolCall, map[string]any{"tool": map[string]any{
				"kind":     "tool",
				"toolKind": claudeToolKind(name),
				"toolName": name,
				"toolId":   toolID,
				"input":    block["input"],
			}}))
		}
	}
	return entries
}

func toolResultEntries(event map[string]any) []readmodels.TranscriptEntry {
	message, _ := event["message"].(map[string]any)
	content, _ := message["content"].([]any)
	entries := make([]readmodels.TranscriptEntry, 0, len(content))
	for _, raw := range content {
		block, _ := raw.(map[string]any)
		if stringValue(block["type"]) != "tool_result" {
			continue
		}
		toolID := stringValue(block["tool_use_id"])
		if toolID == "" {
			continue
		}
		entries = append(entries, transcript.New(transcript.KindToolResult, map[string]any{
			"toolId":  toolID,
			"content": block["content"],
			"isError": block["is_error"] == true,
		}))
	}
	return entries
}

func claudeToolKind(name string) string {
	switch name {
	case "Bash":
		return "bash"
	case "Read":
		return "read_file"
	case "Write":
		return "write_file"
	case "Edit", "MultiEdit":
		return "edit_file"
	case "Glob":
		return "glob"
	case "Grep":
		return "grep"
	default:
		return "unknown_tool"
	}
}

func eventType(event map[string]any) string {
	if value := stringValue(event["type"]); value != "" {
		return value
	}
	return stringValue(event["event"])
}

func eventSessionToken(event map[string]any) string {
	for _, key := range []string{"session_id", "sessionId", "sessionToken"} {
		if value := stringValue(event[key]); value != "" {
			return value
		}
	}
	return ""
}

func assistantText(event map[string]any) string {
	if text := stringValue(event["text"]); text != "" {
		return text
	}
	message, _ := event["message"].(map[string]any)
	content, _ := message["content"].([]any)
	text := ""
	for _, item := range content {
		record, _ := item.(map[string]any)
		if stringValue(record["type"]) == "text" {
			text += stringValue(record["text"])
		}
	}
	return text
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func number(value any) float64 {
	if typed, ok := value.(float64); ok {
		return typed
	}
	return 0
}
