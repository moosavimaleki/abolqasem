// Package opencode adapts the installed OpenCode CLI to Abolqasem turns.
package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

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
	if strings.TrimSpace(executable) == "" {
		executable = "opencode"
	}
	return &Adapter{Executable: executable}
}

func (a *Adapter) BuildArgs(request PromptRequest) []string {
	args := []string{"run", "--format", "json"}
	if cwd := strings.TrimSpace(request.CWD); cwd != "" {
		args = append(args, "--dir", cwd)
	}
	if model := strings.TrimSpace(request.Model); model != "" {
		args = append(args, "--model", model)
	}
	if variant := strings.TrimSpace(request.Effort); variant != "" {
		args = append(args, "--variant", variant)
	}
	if session := strings.TrimSpace(request.SessionToken); session != "" {
		args = append(args, "--session", session)
	}
	if request.ForkSession {
		args = append(args, "--fork")
	}
	return append(args, request.Prompt)
}

func (a *Adapter) RunPromptResult(ctx context.Context, request PromptRequest) (PromptResult, error) {
	cmd := exec.CommandContext(ctx, a.Executable, a.BuildArgs(request)...)
	cmd.Env = state.CurrentProviderProxyEnvWithOverrides(request.Env)
	if strings.TrimSpace(request.CWD) != "" {
		cmd.Dir = request.CWD
	}
	output, err := cmd.CombinedOutput()
	result, parseErr := ParseStreamResult(strings.NewReader(string(output)))
	if result.SessionToken == "" {
		result.SessionToken = sessionTokenInText(string(output))
	}
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			return result, err
		}
		return result, fmt.Errorf("opencode: %s", message)
	}
	if parseErr != nil {
		return result, parseErr
	}
	return result, nil
}

func ParseStreamResult(reader io.Reader) (PromptResult, error) {
	var result PromptResult
	seenText := map[string]bool{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return result, err
		}
		if result.SessionToken == "" {
			result.SessionToken = sessionTokenInValue(event)
		}
		for _, text := range assistantTexts(event) {
			if text == "" || seenText[text] {
				continue
			}
			seenText[text] = true
			result.Entries = append(result.Entries, transcript.New(transcript.KindAssistantText, map[string]any{"text": text}))
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func sessionTokenInText(value string) string {
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) {
		if strings.HasPrefix(field, "ses_") {
			return field
		}
	}
	return ""
}

func sessionTokenInValue(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"sessionID", "sessionId", "session_id"} {
			if token, ok := typed[key].(string); ok && strings.HasPrefix(token, "ses_") {
				return token
			}
		}
		if id, ok := typed["id"].(string); ok && strings.HasPrefix(id, "ses_") {
			return id
		}
		for _, child := range typed {
			if token := sessionTokenInValue(child); token != "" {
				return token
			}
		}
	case []any:
		for _, child := range typed {
			if token := sessionTokenInValue(child); token != "" {
				return token
			}
		}
	}
	return ""
}

func assistantTexts(value any) []string {
	var out []string
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			kind, _ := typed["type"].(string)
			if kind == "text" {
				if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, strings.TrimSpace(text))
				}
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return out
}
