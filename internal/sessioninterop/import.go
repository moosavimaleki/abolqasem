package sessioninterop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

type LegacyImportResult struct {
	Entries        []readmodels.TranscriptEntry
	SessionToken   string
	Provider       string
	LocalPath      string
	ProjectName    string
	SessionName    string
	SourceChatID   string
	TranscriptPath string
}

func ImportLegacySession(meta state.SessionMeta) (LegacyImportResult, error) {
	meta.Agent = strings.ToLower(strings.TrimSpace(meta.Agent))
	meta.SessionID = strings.TrimSpace(meta.SessionID)
	meta.TranscriptPath = strings.TrimSpace(meta.TranscriptPath)
	meta.Cwd = strings.TrimSpace(meta.Cwd)
	meta.ProjectName = strings.TrimSpace(meta.ProjectName)
	entries, err := importEntries(meta.Agent, meta.SessionID, meta.TranscriptPath)
	if err != nil {
		return LegacyImportResult{}, err
	}
	meta = enrichSessionMetaFromEntries(meta, entries)
	return LegacyImportResult{
		Entries:        entries,
		SessionToken:   strings.TrimSpace(meta.SessionID),
		Provider:       strings.TrimSpace(meta.Agent),
		LocalPath:      strings.TrimSpace(meta.Cwd),
		ProjectName:    strings.TrimSpace(meta.ProjectName),
		SessionName:    state.ResolveSessionName(meta),
		TranscriptPath: strings.TrimSpace(meta.TranscriptPath),
	}, nil
}

func importEntries(agent string, sessionID string, transcriptPath string) ([]readmodels.TranscriptEntry, error) {
	agent = strings.ToLower(strings.TrimSpace(agent))
	if strings.EqualFold(filepath.Ext(transcriptPath), ".json") {
		data, err := os.ReadFile(transcriptPath)
		if err != nil {
			return nil, err
		}
		var raw any
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		return nil, nil
	}
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	entries := make([]readmodels.TranscriptEntry, 0, 128)
	codexCustomCommands := make(map[string]struct{})
	index := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		index++
		var chunk []readmodels.TranscriptEntry
		switch agent {
		case "claude":
			chunk = importClaudeLine(sessionID, raw, index)
		case "codex":
			chunk = importCodexLine(sessionID, raw, index, codexCustomCommands)
		}
		entries = append(entries, chunk...)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return entries, err
	}
	return dedupeAdjacentTranscriptEntries(entries), nil
}

func importClaudeLine(sessionID string, raw map[string]any, index int) []readmodels.TranscriptEntry {
	typeName := stringValue(raw["type"])
	message := mapValue(raw["message"])
	role := stringValue(message["role"])
	createdAt := parseUnixMilli(raw["timestamp"])
	switch typeName {
	case "user":
		return importClaudeUserContent(sessionID, role, message["content"], createdAt, index)
	case "assistant":
		return importClaudeAssistantContent(sessionID, role, message["content"], createdAt, index)
	default:
		return nil
	}
}

func importClaudeUserContent(sessionID string, role string, content any, createdAt int64, index int) []readmodels.TranscriptEntry {
	if strings.ToLower(strings.TrimSpace(role)) != "user" {
		return nil
	}
	if text, ok := content.(string); ok {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || strings.Contains(trimmed, "<local-command-caveat>") {
			return nil
		}
		return []readmodels.TranscriptEntry{newEntryAt(transcript.KindUserPrompt, map[string]any{
			"_id":       fmt.Sprintf("claude-user-%s-%d", sessionID, index),
			"createdAt": float64(createdAt),
			"content":   trimmed,
		})}
	}
	blocks, _ := content.([]any)
	entries := make([]readmodels.TranscriptEntry, 0, len(blocks))
	for blockIndex, rawBlock := range blocks {
		block := mapValue(rawBlock)
		switch stringValue(block["type"]) {
		case "text":
			text := strings.TrimSpace(stringValue(block["text"]))
			if text == "" {
				continue
			}
			entries = append(entries, newEntryAt(transcript.KindUserPrompt, map[string]any{
				"_id":       fmt.Sprintf("claude-user-%s-%d-%d", sessionID, index, blockIndex),
				"createdAt": float64(createdAt),
				"content":   text,
			}))
		case "tool_result":
			entries = append(entries, newEntryAt(transcript.KindToolResult, map[string]any{
				"_id":       fmt.Sprintf("claude-tool-result-%s-%d-%d", sessionID, index, blockIndex),
				"createdAt": float64(createdAt),
				"toolId":    stringValue(block["tool_use_id"]),
				"content":   flattenUnknown(block["content"]),
				"isError":   boolValue(block["is_error"]),
				"debugRaw":  mustJSONString(block),
			}))
		}
	}
	return entries
}

func importClaudeAssistantContent(sessionID string, role string, content any, createdAt int64, index int) []readmodels.TranscriptEntry {
	if strings.ToLower(strings.TrimSpace(role)) != "assistant" {
		return nil
	}
	blocks, _ := content.([]any)
	entries := make([]readmodels.TranscriptEntry, 0, len(blocks))
	for blockIndex, rawBlock := range blocks {
		block := mapValue(rawBlock)
		switch stringValue(block["type"]) {
		case "text":
			text := strings.TrimSpace(stringValue(block["text"]))
			if text == "" {
				continue
			}
			entries = append(entries, newEntryAt(transcript.KindAssistantText, map[string]any{
				"_id":       fmt.Sprintf("claude-assistant-%s-%d-%d", sessionID, index, blockIndex),
				"createdAt": float64(createdAt),
				"text":      text,
			}))
		case "tool_use":
			toolName := stringValue(block["name"])
			entries = append(entries, newEntryAt(transcript.KindToolCall, map[string]any{
				"_id":       fmt.Sprintf("claude-tool-call-%s-%d-%d", sessionID, index, blockIndex),
				"createdAt": float64(createdAt),
				"tool": map[string]any{
					"kind":     "tool",
					"toolKind": inferToolKind(toolName),
					"toolName": toolName,
					"toolId":   stringValue(block["id"]),
					"input":    block["input"],
				},
			}))
		case "thinking":
			continue
		}
	}
	return entries
}

func importCodexLine(sessionID string, raw map[string]any, index int, customCommands map[string]struct{}) []readmodels.TranscriptEntry {
	typeName := stringValue(raw["type"])
	payload := mapValue(raw["payload"])
	createdAt := parseUnixMilli(raw["timestamp"])
	switch typeName {
	case "event_msg":
		switch stringValue(payload["type"]) {
		case "user_message":
			text := strings.TrimSpace(stringValue(payload["message"]))
			if text == "" {
				return nil
			}
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindUserPrompt, map[string]any{
				"_id":       fmt.Sprintf("codex-user-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
				"content":   text,
			})}
		case "agent_message":
			text := strings.TrimSpace(stringValue(payload["message"]))
			if text == "" {
				return nil
			}
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindAssistantText, map[string]any{
				"_id":       fmt.Sprintf("codex-assistant-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
				"text":      text,
			})}
		}
	case "response_item":
		switch stringValue(payload["type"]) {
		case "custom_tool_call":
			if !strings.EqualFold(strings.TrimSpace(stringValue(payload["name"])), "exec") {
				return nil
			}
			command, cwd, ok := extractCodexExecCommand(stringValue(payload["input"]))
			if !ok {
				return nil
			}
			itemID := firstNonEmptyString(stringValue(payload["call_id"]), stringValue(payload["id"]), fmt.Sprintf("codex-custom-command-%s-%d", sessionID, index))
			customCommands[itemID] = struct{}{}
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindCommandExecution, map[string]any{
				"_id":       fmt.Sprintf("codex-command-start-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
				"itemId":    itemID,
				"command":   command,
				"cwd":       cwd,
				"status":    "inProgress",
			})}
		case "custom_tool_call_output":
			itemID := firstNonEmptyString(stringValue(payload["call_id"]), stringValue(payload["id"]))
			if _, ok := customCommands[itemID]; !ok {
				return nil
			}
			output := flattenUnknown(firstNonNil(payload["output"], payload["content"]))
			status, exitCode, hasExitCode := codexCommandCompletion(output)
			fields := map[string]any{
				"_id":              fmt.Sprintf("codex-command-result-%s-%d", sessionID, index),
				"createdAt":        float64(createdAt),
				"itemId":           itemID,
				"status":           status,
				"aggregatedOutput": output,
			}
			if hasExitCode {
				fields["exitCode"] = float64(exitCode)
			}
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindCommandExecution, fields)}
		case "function_call":
			toolName := stringValue(payload["name"])
			input := parseJSONStringOrRaw(stringValue(payload["arguments"]))
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindToolCall, map[string]any{
				"_id":       fmt.Sprintf("codex-tool-call-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
				"tool": map[string]any{
					"kind":     "tool",
					"toolKind": inferToolKind(toolName),
					"toolName": toolName,
					"toolId":   stringValue(payload["call_id"]),
					"input":    input,
				},
			})}
		case "function_call_output":
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindToolResult, map[string]any{
				"_id":       fmt.Sprintf("codex-tool-result-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
				"toolId":    stringValue(payload["call_id"]),
				"content":   flattenUnknown(firstNonNil(payload["output"], payload["content"])),
				"isError":   false,
				"debugRaw":  mustJSONString(payload),
			})}
		case "message":
			role := strings.ToLower(strings.TrimSpace(stringValue(payload["role"])))
			text := strings.TrimSpace(codexContentText(payload["content"]))
			if text == "" {
				return nil
			}
			if role == "user" {
				return []readmodels.TranscriptEntry{newEntryAt(transcript.KindUserPrompt, map[string]any{
					"_id":       fmt.Sprintf("codex-message-user-%s-%d", sessionID, index),
					"createdAt": float64(createdAt),
					"content":   text,
				})}
			}
			if role != "assistant" {
				return nil
			}
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindAssistantText, map[string]any{
				"_id":       fmt.Sprintf("codex-message-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
				"text":      text,
			})}
		}
	case "turn_context":
		if boolValue(payload["compacted"]) {
			return []readmodels.TranscriptEntry{newEntryAt(transcript.KindCompactBoundary, map[string]any{
				"_id":       fmt.Sprintf("codex-compact-%s-%d", sessionID, index),
				"createdAt": float64(createdAt),
			})}
		}
	}
	return nil
}

var codexExecCommandFieldPattern = regexp.MustCompile(`(?:^|[,\{]\s*)["']?cmd["']?\s*:\s*("(?:\\.|[^"\\])*")`)
var codexExecWorkdirFieldPattern = regexp.MustCompile(`(?:^|[,\{]\s*)["']?workdir["']?\s*:\s*("(?:\\.|[^"\\])*")`)
var codexExitCodePattern = regexp.MustCompile(`(?i)(?:exited with code|exit(?:ed)? code\s*[:=]?)\s*(-?[0-9]+)`)

func extractCodexExecCommand(input string) (string, string, bool) {
	const marker = "tools.exec_command("
	commands := make([]string, 0, 1)
	workdirs := make([]string, 0, 1)
	for remaining := input; ; {
		markerIndex := strings.Index(remaining, marker)
		if markerIndex < 0 {
			break
		}
		remaining = remaining[markerIndex+len(marker):]
		segment := remaining
		if nextIndex := strings.Index(segment, marker); nextIndex >= 0 {
			segment = segment[:nextIndex]
		}
		command := extractCodexJSONStringField(segment, codexExecCommandFieldPattern)
		if strings.TrimSpace(command) != "" {
			commands = append(commands, command)
			workdirs = append(workdirs, extractCodexJSONStringField(segment, codexExecWorkdirFieldPattern))
		}
	}
	if len(commands) == 0 {
		return "", "", false
	}
	cwd := workdirs[0]
	for _, workdir := range workdirs[1:] {
		if workdir != cwd {
			cwd = ""
			break
		}
	}
	return strings.Join(commands, "\n"), cwd, true
}

func extractCodexJSONStringField(input string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(input)
	if len(match) != 2 {
		return ""
	}
	var value string
	if err := json.Unmarshal([]byte(match[1]), &value); err != nil {
		return ""
	}
	return value
}

func codexCommandCompletion(output string) (string, int, bool) {
	if match := codexExitCodePattern.FindStringSubmatch(output); len(match) == 2 {
		if exitCode, err := strconv.Atoi(match[1]); err == nil {
			if exitCode == 0 {
				return "completed", exitCode, true
			}
			return "failed", exitCode, true
		}
	}
	if strings.Contains(output, "Script completed") {
		return "completed", 0, true
	}
	return "completed", 0, false
}

func dedupeAdjacentTranscriptEntries(entries []readmodels.TranscriptEntry) []readmodels.TranscriptEntry {
	if len(entries) < 2 {
		return entries
	}
	out := make([]readmodels.TranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		if len(out) > 0 && transcriptEntriesHaveSameVisibleText(out[len(out)-1], entry) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func transcriptEntriesHaveSameVisibleText(left readmodels.TranscriptEntry, right readmodels.TranscriptEntry) bool {
	leftKind := transcript.Kind(left)
	rightKind := transcript.Kind(right)
	if leftKind != rightKind {
		return false
	}
	switch leftKind {
	case transcript.KindUserPrompt:
		return stringValue(left["content"]) != "" && stringValue(left["content"]) == stringValue(right["content"])
	case transcript.KindAssistantText:
		return stringValue(left["text"]) != "" && stringValue(left["text"]) == stringValue(right["text"])
	default:
		return false
	}
}

func newEntryAt(kind string, fields map[string]any) readmodels.TranscriptEntry {
	return transcript.New(kind, fields)
}

func codexContentText(value any) string {
	blocks, _ := value.([]any)
	parts := make([]string, 0, len(blocks))
	for _, rawBlock := range blocks {
		block := mapValue(rawBlock)
		switch stringValue(block["type"]) {
		case "output_text", "text", "input_text":
			if text := strings.TrimSpace(stringValue(block["text"])); text != "" && !isEmbeddedDataURL(text) {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func isEmbeddedDataURL(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(text)), "data:")
}

func inferToolKind(toolName string) string {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	switch normalized {
	case "bash", "exec_command":
		return "bash"
	case "read", "read_file":
		return "read_file"
	case "write", "write_file":
		return "write_file"
	case "edit", "edit_file":
		return "edit_file"
	case "grep":
		return "grep"
	case "glob":
		return "glob"
	case "websearch", "web_search":
		return "web_search"
	case "askuserquestion", "ask_user_question":
		return "ask_user_question"
	case "exitplanmode", "exit_plan_mode":
		return "exit_plan_mode"
	case "skill":
		return "skill"
	default:
		if normalized == "" {
			return "mcp_generic"
		}
		return normalized
	}
}

func flattenUnknown(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(flattenUnknown(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "message", "output", "result"} {
			if text := strings.TrimSpace(flattenUnknown(typed[key])); text != "" {
				return text
			}
		}
		body, _ := json.MarshalIndent(typed, "", "  ")
		return string(body)
	default:
		body, _ := json.Marshal(typed)
		return string(body)
	}
}

func mapValue(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func boolValue(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func parseUnixMilli(value any) int64 {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return time.Now().UnixMilli()
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.UnixMilli()
	}
	return time.Now().UnixMilli()
}

func parseJSONStringOrRaw(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[string]any{}
	}
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) == nil {
		return parsed
	}
	return map[string]any{"raw": value}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mustJSONString(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(body)
}

func enrichSessionMetaFromEntries(meta state.SessionMeta, entries []readmodels.TranscriptEntry) state.SessionMeta {
	if isGeneratedImportedSessionTitle(meta.SessionName, meta) {
		meta.SessionName = ""
	}
	if meta.FirstPreview != "" && meta.LastPreview != "" && meta.MessageCountEstimate > 0 {
		return meta
	}

	meta.MessageCountEstimate = len(entries)
	firstAnyPreview := ""
	firstUserPreview := ""
	lastPreview := ""
	for _, entry := range entries {
		text, isUser := transcriptEntryPreview(entry)
		if strings.TrimSpace(text) == "" {
			continue
		}
		isBootstrap := state.IsAgentBootstrapPrompt(text)
		if firstAnyPreview == "" && !isBootstrap {
			firstAnyPreview = text
		}
		if firstUserPreview == "" && isUser && !isBootstrap {
			firstUserPreview = text
		}
		lastPreview = text
	}
	if meta.FirstPreview == "" {
		meta.FirstPreview = firstNonEmptyString(firstUserPreview, firstAnyPreview)
	}
	if meta.LastPreview == "" {
		meta.LastPreview = lastPreview
	}
	return meta
}

func transcriptEntryPreview(entry readmodels.TranscriptEntry) (string, bool) {
	switch transcript.Kind(entry) {
	case transcript.KindUserPrompt:
		return strings.TrimSpace(flattenUnknown(entry["content"])), true
	case transcript.KindAssistantText:
		return strings.TrimSpace(flattenUnknown(entry["text"])), false
	case transcript.KindCompactSummary:
		return strings.TrimSpace(flattenUnknown(entry["summary"])), false
	case transcript.KindStatus:
		return strings.TrimSpace(flattenUnknown(entry["status"])), false
	case transcript.KindToolResult:
		return strings.TrimSpace(flattenUnknown(entry["content"])), false
	default:
		return "", false
	}
}

func isGeneratedImportedSessionTitle(title string, meta state.SessionMeta) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if strings.EqualFold(title, strings.TrimSpace(meta.SessionID)) {
		return true
	}
	transcriptBase := strings.TrimSuffix(filepath.Base(strings.TrimSpace(meta.TranscriptPath)), filepath.Ext(strings.TrimSpace(meta.TranscriptPath)))
	if transcriptBase != "" && strings.EqualFold(title, transcriptBase) {
		return true
	}
	return looksLikeGeneratedImportedSessionID(title)
}

func looksLikeGeneratedImportedSessionID(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "rollout-") {
		return true
	}
	if strings.Count(value, "-") < 3 {
		return false
	}
	hexLike := 0
	for _, r := range value {
		if r == '-' {
			continue
		}
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			hexLike++
			continue
		}
		return false
	}
	return hexLike >= 16
}
