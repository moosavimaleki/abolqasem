package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/gitservice"
	"abolqasem/internal/workspace/transcript"
)

const (
	workspaceCommitMessageAITimeout       = 45 * time.Second
	workspaceCommitMessagePatchCharBudget = 12000
	workspaceCommitMessagePatchFileBudget = 3000
)

type workspaceCommitMessageDraft struct {
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Message     string `json:"message"`
}

func workspaceGenerateCommitMessageAI(ctx context.Context, localPath string, paths []string) (string, string, error) {
	settings, err := state.LoadSettings()
	if err != nil {
		settings = state.DefaultAppSettings()
	}
	settings = state.NormalizeSettings(settings)
	generator := settings.CommitMessageGenerator
	if strings.TrimSpace(generator.Provider) == "" || strings.TrimSpace(generator.Model) == "" {
		return "", "", errors.New("commit message generator is not configured")
	}

	prompt, err := workspaceBuildCommitMessagePrompt(ctx, localPath, paths)
	if err != nil {
		return "", "", err
	}
	text, err := workspaceRunTransientCommitMessageTurn(ctx, localPath, generator.Provider, generator.Model, prompt)
	if err != nil {
		return "", "", err
	}
	return workspaceParseCommitMessageJSON(text)
}

func workspaceRunTransientCommitMessageTurn(ctx context.Context, localPath string, provider string, model string, prompt string) (string, error) {
	provider = strings.TrimSpace(provider)
	chatID := "commit-message-" + randomID()
	env, cleanup, err := workspaceTransientProviderEnv(provider)
	if err != nil {
		return "", err
	}
	defer cleanup()
	request := agent.TurnRequest{
		ChatID:    chatID,
		LocalPath: localPath,
		Provider:  provider,
		Model:     strings.TrimSpace(model),
		Content:   prompt,
		PlanMode:  true,
		Env:       env,
	}

	var turn agent.Turn
	switch provider {
	case "claude":
		turn = startWorkspaceClaudeTurn(ctx, request)
	case "codex":
		turn = startWorkspaceCodexTurn(ctx, request)
		defer workspaceCodexSessions.close(chatID)
	case "gemini":
		turn = startWorkspaceGeminiTurn(ctx, request)
	default:
		return "", fmt.Errorf("unsupported commit message provider: %s", provider)
	}
	return workspaceCollectTransientTurnText(ctx, turn)
}

func workspaceCollectTransientTurnText(ctx context.Context, turn agent.Turn) (string, error) {
	source, ok := turn.(agent.TurnEventSource)
	if !ok || source.Events() == nil {
		return "", errors.New("provider turn does not expose events")
	}
	responder, _ := turn.(agent.ToolResponder)
	var text strings.Builder
	for {
		select {
		case <-ctx.Done():
			_ = turn.Cancel()
			return "", ctx.Err()
		case event, ok := <-source.Events():
			if !ok {
				output := strings.TrimSpace(text.String())
				if output == "" {
					return "", errors.New("provider completed without text")
				}
				return output, nil
			}
			switch event.Type {
			case agent.TurnEventTranscript:
				if transcript.Kind(event.Entry) != transcript.KindAssistantText {
					continue
				}
				if chunk, ok := event.Entry["text"].(string); ok && strings.TrimSpace(chunk) != "" {
					if text.Len() > 0 {
						text.WriteString("\n")
					}
					text.WriteString(chunk)
				}
			case agent.TurnEventPendingTool:
				if responder == nil || event.PendingTool == nil {
					continue
				}
				responseCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				_ = responder.RespondTool(responseCtx, agent.ToolResponse{
					ToolUseID: event.PendingTool.ToolUseID,
					Result:    map[string]any{"answers": map[string]any{}},
				})
				cancel()
			case agent.TurnEventFinished:
				output := strings.TrimSpace(text.String())
				if output == "" {
					return "", errors.New("provider returned an empty commit message")
				}
				return output, nil
			case agent.TurnEventFailed:
				if event.Error != nil {
					return "", event.Error
				}
				return "", errors.New(firstNonEmptyWorkspaceString(event.Message, "provider failed"))
			case agent.TurnEventCancelled:
				return "", errors.New("provider turn was cancelled")
			}
		}
	}
}

func workspaceBuildCommitMessagePrompt(ctx context.Context, localPath string, paths []string) (string, error) {
	cleaned := workspaceCleanCommitMessagePaths(paths)
	if len(cleaned) == 0 {
		return "", errors.New("no diff paths selected")
	}
	snapshot, err := gitservice.Detect(ctx, localPath)
	if err != nil {
		return "", err
	}
	if snapshot.Status != gitservice.StatusReady {
		return "", errors.New("project is not a git repository")
	}

	filesByPath := make(map[string]gitservice.DiffFile, len(snapshot.Files))
	for _, file := range snapshot.Files {
		filesByPath[workspaceCleanCommitMessagePath(file.Path)] = file
	}

	type selectedDiffFile struct {
		path  string
		file  gitservice.DiffFile
		patch string
	}
	selected := make([]selectedDiffFile, 0, len(cleaned))
	for _, path := range cleaned {
		file, ok := filesByPath[path]
		if !ok {
			continue
		}
		patch, err := gitservice.ReadPatch(ctx, localPath, path)
		if err != nil {
			return "", err
		}
		selected = append(selected, selectedDiffFile{path: path, file: file, patch: patch})
	}
	if len(selected) == 0 {
		return "", errors.New("no selected git diff files found")
	}

	var builder strings.Builder
	builder.WriteString("Draft a git commit message for the selected changes.\n")
	builder.WriteString("Do not run commands, modify files, or ask follow-up questions.\n")
	builder.WriteString("Return only valid JSON with this exact shape: {\"subject\":\"...\",\"body\":\"...\"}\n")
	builder.WriteString("Subject rules: imperative mood, no trailing period, 72 characters or less.\n")
	builder.WriteString("Body rules: optional plain text explaining notable context; use an empty string when unnecessary.\n\n")
	if strings.TrimSpace(snapshot.BranchName) != "" {
		fmt.Fprintf(&builder, "Branch: %s\n\n", snapshot.BranchName)
	}
	builder.WriteString("Selected files:\n")
	for _, item := range selected {
		fmt.Fprintf(
			&builder,
			"- %s (%s, +%d -%d, untracked=%t)\n",
			item.path,
			item.file.ChangeType,
			item.file.Additions,
			item.file.Deletions,
			item.file.IsUntracked,
		)
	}
	builder.WriteString("\nDiff excerpts:\n")

	remaining := workspaceCommitMessagePatchCharBudget
	for _, item := range selected {
		if remaining <= 0 {
			fmt.Fprintf(&builder, "\n--- %s ---\n(diff omitted: prompt budget reached)\n", item.path)
			continue
		}
		limit := workspaceCommitMessagePatchFileBudget
		if remaining < limit {
			limit = remaining
		}
		patch := strings.TrimSpace(item.patch)
		if patch == "" {
			patch = "(no textual patch available)"
		}
		excerpt, truncated := workspaceTrimForPrompt(patch, limit)
		remaining -= len(excerpt)
		fmt.Fprintf(&builder, "\n--- %s ---\n", item.path)
		builder.WriteString(excerpt)
		if truncated {
			builder.WriteString("\n[diff truncated]\n")
		}
		if !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteString("\n")
		}
	}
	return builder.String(), nil
}

func workspaceParseCommitMessageJSON(text string) (string, string, error) {
	for _, candidate := range workspaceCommitMessageJSONCandidates(text) {
		var draft workspaceCommitMessageDraft
		if err := json.Unmarshal([]byte(candidate), &draft); err != nil {
			continue
		}
		subject := strings.TrimSpace(firstNonEmptyWorkspaceString(draft.Subject, draft.Summary))
		body := strings.TrimSpace(firstNonEmptyWorkspaceString(draft.Body, draft.Description))
		if subject == "" && strings.TrimSpace(draft.Message) != "" {
			subject, body = workspaceSplitCommitMessage(draft.Message)
		}
		subject = workspaceCommitMessageSubjectLine(subject)
		body = workspaceNormalizeCommitMessageBody(body)
		if subject != "" {
			return subject, body, nil
		}
	}
	return "", "", errors.New("provider did not return a valid commit message JSON object")
}

func workspaceCommitMessageJSONCandidates(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}
	fenced := workspaceStripMarkdownCodeFence(trimmed)
	if fenced != trimmed {
		candidates = append(candidates, fenced)
	}
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			candidates = append(candidates, trimmed[start:end+1])
		}
	}

	seen := map[string]bool{}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}
	return unique
}

func workspaceStripMarkdownCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	lines = lines[1:]
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func workspaceSplitCommitMessage(message string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return "", ""
	}
	subject := strings.TrimSpace(lines[0])
	body := strings.TrimSpace(strings.Join(lines[1:], "\n"))
	return subject, body
}

func workspaceCommitMessageSubjectLine(subject string) string {
	subject = strings.TrimSpace(strings.ReplaceAll(subject, "\r\n", "\n"))
	if index := strings.Index(subject, "\n"); index >= 0 {
		subject = strings.TrimSpace(subject[:index])
	}
	return strings.TrimSuffix(subject, ".")
}

func workspaceNormalizeCommitMessageBody(body string) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	return strings.Trim(body, "`")
}

func workspaceCleanCommitMessagePaths(paths []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		path = workspaceCleanCommitMessagePath(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		cleaned = append(cleaned, path)
	}
	return cleaned
}

func workspaceCleanCommitMessagePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return strings.Trim(path, "/")
}

func workspaceTrimForPrompt(value string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimSpace(value) != ""
	}
	if len(value) <= limit {
		return value, false
	}
	if limit > 200 {
		cut := strings.LastIndex(value[:limit], "\n")
		if cut > 0 {
			return strings.TrimSpace(value[:cut]), true
		}
	}
	return strings.TrimSpace(value[:limit]), true
}
