package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"

	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
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
	cmd := exec.CommandContext(ctx, a.Executable, a.BuildArgs(request)...)
	if request.CWD != "" {
		cmd.Dir = request.CWD
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	entries, parseErr := ParseStream(stdout)
	waitErr := cmd.Wait()
	if parseErr != nil {
		return entries, parseErr
	}
	return entries, waitErr
}

func ParseStream(reader io.Reader) ([]readmodels.TranscriptEntry, error) {
	var entries []readmodels.TranscriptEntry
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			return entries, err
		}
		entries = append(entries, entriesFromEvent(event)...)
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

func entriesFromEvent(event map[string]any) []readmodels.TranscriptEntry {
	switch eventType(event) {
	case "assistant":
		text := assistantText(event)
		if text == "" {
			return nil
		}
		return []readmodels.TranscriptEntry{transcript.New(transcript.KindAssistantText, map[string]any{"text": text})}
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

func eventType(event map[string]any) string {
	if value := stringValue(event["type"]); value != "" {
		return value
	}
	return stringValue(event["event"])
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
