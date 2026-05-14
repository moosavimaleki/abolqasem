package parser

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-agent-manager/internal/render"
)

var ErrTranscriptUnavailable = errors.New("transcript is unavailable")

type Message struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Role      string     `json:"role"`
	Kind      string     `json:"kind"`
	Text      string     `json:"text"`
	HTML      string     `json:"html"`
	Direction string     `json:"direction"`
	Index     int        `json:"index"`
	CreatedAt *time.Time `json:"created_at"`
}

type ParseResult struct {
	SessionID     string    `json:"session_id"`
	Items         []Message `json:"items"`
	HasMoreBefore bool      `json:"has_more_before"`
	HasMoreAfter  bool      `json:"has_more_after"`
	OldestCursor  string    `json:"oldest_cursor"`
	NewestCursor  string    `json:"newest_cursor"`
	Status        string    `json:"status,omitempty"`
}

type ParseOptions struct {
	Limit  int
	Before string
	After  string
}

type SessionSummary struct {
	LastPreview          string
	MessageCountEstimate int
}

type cacheEntry struct {
	mtime    time.Time
	size     int64
	messages []Message
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

func ParseMessages(agent, sessionID, transcriptPath string, opts ParseOptions) (*ParseResult, error) {
	if strings.TrimSpace(transcriptPath) == "" {
		return &ParseResult{
			SessionID: sessionID,
			Items:     []Message{},
			Status:    "metadata_only",
		}, ErrTranscriptUnavailable
	}
	if opts.Limit <= 0 {
		opts.Limit = 30
	}

	messages, err := loadMessages(agent, sessionID, transcriptPath)
	if err != nil {
		return nil, err
	}

	result := &ParseResult{
		SessionID: sessionID,
		Items:     []Message{},
	}
	if len(messages) == 0 {
		return result, nil
	}

	start, end := sliceRange(messages, opts)
	if start < 0 {
		start = 0
	}
	if end > len(messages) {
		end = len(messages)
	}
	if start > end {
		start = end
	}

	result.Items = append(result.Items, messages[start:end]...)
	result.HasMoreBefore = start > 0
	result.HasMoreAfter = end < len(messages)
	if len(result.Items) > 0 {
		result.OldestCursor = strconv.Itoa(result.Items[0].Index)
		result.NewestCursor = strconv.Itoa(result.Items[len(result.Items)-1].Index)
	}
	return result, nil
}

func GetSessionSummary(agent, sessionID, transcriptPath string) (SessionSummary, error) {
	if strings.TrimSpace(transcriptPath) == "" {
		return SessionSummary{}, ErrTranscriptUnavailable
	}
	messages, err := loadMessages(agent, sessionID, transcriptPath)
	if err != nil {
		return SessionSummary{}, err
	}
	summary := SessionSummary{MessageCountEstimate: len(messages)}
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Text) == "" {
			continue
		}
		summary.LastPreview = trimPreview(messages[i].Text)
		break
	}
	return summary, nil
}

func loadMessages(agent, sessionID, transcriptPath string) ([]Message, error) {
	info, err := os.Stat(transcriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTranscriptUnavailable
		}
		return nil, err
	}

	cacheKey := agent + "|" + filepath.Clean(transcriptPath)
	cacheMu.Lock()
	if entry, ok := cache[cacheKey]; ok && entry.size == info.Size() && entry.mtime.Equal(info.ModTime()) {
		cached := append([]Message(nil), entry.messages...)
		cacheMu.Unlock()
		return cached, nil
	}
	cacheMu.Unlock()

	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	messages := make([]Message, 0, 64)
	lineIndex := 0
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineIndex++
			raw := map[string]any{}
			if decodeErr := json.Unmarshal(bytesTrimSpace(line), &raw); decodeErr == nil {
				if msg := extractMessage(agent, raw, sessionID, lineIndex); msg != nil {
					messages = append(messages, *msg)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}

	cacheMu.Lock()
	cache[cacheKey] = cacheEntry{
		mtime:    info.ModTime(),
		size:     info.Size(),
		messages: append([]Message(nil), messages...),
	}
	cacheMu.Unlock()
	return messages, nil
}

func sliceRange(messages []Message, opts ParseOptions) (int, int) {
	end := len(messages)
	if opts.Before != "" {
		index := findMessageIndex(messages, opts.Before)
		if index >= 0 {
			end = index
		}
	}

	start := end - opts.Limit
	if start < 0 {
		start = 0
	}

	if opts.After != "" {
		index := findMessageIndex(messages, opts.After)
		if index >= 0 {
			start = index + 1
			end = start + opts.Limit
			if end > len(messages) {
				end = len(messages)
			}
		}
	}
	return start, end
}

func findMessageIndex(messages []Message, cursor string) int {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return -1
	}
	if idx, err := strconv.Atoi(cursor); err == nil {
		for i, msg := range messages {
			if msg.Index == idx {
				return i
			}
		}
	}
	for i, msg := range messages {
		if msg.ID == cursor {
			return i
		}
	}
	return -1
}

func extractMessage(agent string, raw map[string]any, sessionID string, index int) *Message {
	switch strings.ToLower(agent) {
	case "codex":
		return extractCodexMessage(raw, sessionID, index)
	case "claude":
		return extractClaudeMessage(raw, sessionID, index)
	case "gemini":
		return extractGeminiMessage(raw, sessionID, index)
	default:
		return extractGenericMessage(raw, sessionID, index)
	}
}

func extractCodexMessage(raw map[string]any, sessionID string, index int) *Message {
	payload := asMap(raw["payload"])
	eventType := stringValue(payload["type"])
	if eventType == "" {
		eventType = stringValue(raw["type"])
	}

	switch eventType {
	case "user_message":
		return newMessage(sessionID, index, "user", "message", firstNonEmpty(
			flattenText(payload["message"]),
			flattenText(payload["text"]),
			flattenText(raw["message"]),
		), extractTimestamp(raw, payload))
	case "agent_message":
		return newMessage(sessionID, index, "assistant", "message", firstNonEmpty(
			flattenText(payload["message"]),
			flattenText(payload["text"]),
			flattenText(payload["content"]),
		), extractTimestamp(raw, payload))
	case "tool_call", "command_output", "tool_result":
		return newMessage(sessionID, index, "tool", "tool", firstNonEmpty(
			flattenText(payload["output"]),
			flattenText(payload["message"]),
			flattenText(raw["output"]),
			flattenText(raw["message"]),
		), extractTimestamp(raw, payload))
	}

	if text := firstNonEmpty(flattenText(payload["message"]), flattenText(raw["message"]), flattenText(raw["text"]), flattenText(raw["content"])); text != "" {
		role := firstNonEmpty(stringValue(payload["role"]), stringValue(raw["role"]), "assistant")
		return newMessage(sessionID, index, role, "message", text, extractTimestamp(raw, payload))
	}
	return nil
}

func extractClaudeMessage(raw map[string]any, sessionID string, index int) *Message {
	message := asMap(raw["message"])
	role := firstNonEmpty(stringValue(message["role"]), stringValue(raw["role"]))
	kind := "message"
	if role == "" {
		role = "assistant"
	}

	text := flattenText(message["content"])
	if text == "" {
		text = flattenText(raw["content"])
	}
	if text == "" {
		text = flattenText(raw["message"])
	}
	if text == "" {
		return nil
	}

	if strings.Contains(strings.ToLower(text), "tool_use") || strings.Contains(strings.ToLower(text), "tool_result") {
		role = "tool"
		kind = "tool"
	}
	return newMessage(sessionID, index, role, kind, text, extractTimestamp(raw, message))
}

func extractGeminiMessage(raw map[string]any, sessionID string, index int) *Message {
	role := firstNonEmpty(stringValue(raw["role"]), stringValue(raw["speaker"]))
	kind := "message"
	text := firstNonEmpty(
		flattenText(raw["content"]),
		flattenText(raw["response"]),
		flattenText(raw["prompt"]),
		flattenText(raw["text"]),
	)
	if text == "" {
		return nil
	}
	if role == "" {
		role = "assistant"
	}
	if role == "tool" {
		kind = "tool"
	}
	return newMessage(sessionID, index, role, kind, text, extractTimestamp(raw))
}

func extractGenericMessage(raw map[string]any, sessionID string, index int) *Message {
	role := firstNonEmpty(stringValue(raw["role"]), stringValue(raw["event_type"]), "assistant")
	kind := "message"
	if role == "command_output" || role == "tool_call" || role == "tool" {
		role = "tool"
		kind = "tool"
	}
	text := firstNonEmpty(
		flattenText(raw["content"]),
		flattenText(raw["text"]),
		flattenText(raw["output"]),
		flattenText(raw["message"]),
		flattenText(raw["user"]),
		flattenText(raw["assistant"]),
	)
	if text == "" {
		return nil
	}
	return newMessage(sessionID, index, role, kind, text, extractTimestamp(raw))
}

func newMessage(sessionID string, index int, role, kind, text string, createdAt *time.Time) *Message {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return &Message{
		ID:        fmt.Sprintf("evt_%s_%d", sessionID, index),
		SessionID: sessionID,
		Role:      role,
		Kind:      kind,
		Text:      text,
		HTML:      render.MarkdownToHTML(text),
		Direction: DetectDirection(text),
		Index:     index,
		CreatedAt: createdAt,
	}
}

func extractTimestamp(values ...map[string]any) *time.Time {
	for _, value := range values {
		for _, key := range []string{"created_at", "timestamp", "time"} {
			if raw := stringValue(value[key]); raw != "" {
				if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
					return &parsed
				}
			}
		}
	}
	return nil
}

func flattenText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := flattenText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]any:
		if text := firstNonEmpty(
			flattenText(typed["text"]),
			flattenText(typed["message"]),
			flattenText(typed["content"]),
			flattenText(typed["output"]),
			flattenText(typed["input"]),
		); text != "" {
			return text
		}
		if name := stringValue(typed["name"]); name != "" {
			args := flattenText(typed["arguments"])
			if args != "" {
				return name + "\n" + args
			}
			return name
		}
		if typed["type"] != nil {
			if body, err := json.MarshalIndent(typed, "", "  "); err == nil {
				return string(body)
			}
		}
	default:
		if body, err := json.Marshal(typed); err == nil {
			return string(body)
		}
	}
	return ""
}

func asMap(value any) map[string]any {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func trimPreview(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 160 {
		return strings.TrimSpace(value[:160]) + "..."
	}
	return value
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}
