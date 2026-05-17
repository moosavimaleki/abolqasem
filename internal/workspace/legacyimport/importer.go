package legacyimport

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"ai-agent-manager/internal/parser"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

type ImportOptions struct {
	RecentLimit int
}

type ImportedSession struct {
	Project          readmodels.ProjectRecord          `json:"project"`
	Chat             readmodels.ChatRecord             `json:"chat"`
	Transcript       readmodels.ChatTranscriptSnapshot `json:"transcript"`
	LegacySessionKey string                            `json:"legacySessionKey"`
	TranscriptPath   string                            `json:"transcriptPath"`
	ReadOnly         bool                              `json:"readOnly"`
}

func ImportSession(meta state.SessionMeta, messages []parser.Message, opts ImportOptions) ImportedSession {
	meta = fillMetaFromMessages(meta, messages)
	titleMeta := meta
	if isGeneratedLegacySessionTitle(titleMeta.SessionName, titleMeta) {
		titleMeta.SessionName = ""
	}
	projectID := ImportedProjectID(meta)
	chatID := ImportedChatID(meta)
	createdAt := firstTimestamp(meta, messages)
	updatedAt := lastTimestamp(meta, messages)
	if updatedAt == 0 {
		updatedAt = createdAt
	}

	project := readmodels.ProjectRecord{
		ID:        projectID,
		LocalPath: meta.Cwd,
		Title:     projectTitle(meta),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	provider := strings.TrimSpace(meta.Agent)
	var providerPtr *string
	if provider != "" && provider != "unknown" {
		providerPtr = &provider
	}
	chat := readmodels.ChatRecord{
		ID:            chatID,
		ProjectID:     projectID,
		Title:         state.ResolveSessionName(titleMeta),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		Unread:        false,
		Provider:      providerPtr,
		PlanMode:      false,
		SessionToken:  nil,
		HasMessages:   len(messages) > 0,
		LastMessageAt: updatedAt,
	}

	entries := mapMessages(meta, messages)
	hasOlder := false
	var olderCursor *string
	if opts.RecentLimit > 0 && len(entries) > opts.RecentLimit {
		hasOlder = true
		cursor := cursorFromEntry(entries[len(entries)-opts.RecentLimit-1])
		olderCursor = &cursor
		entries = entries[len(entries)-opts.RecentLimit:]
	}
	return ImportedSession{
		Project: project,
		Chat:    chat,
		Transcript: readmodels.ChatTranscriptSnapshot{
			Messages: entries,
			History: readmodels.ChatHistorySnapshot{
				HasOlder:    hasOlder,
				OlderCursor: olderCursor,
				RecentLimit: opts.RecentLimit,
			},
		},
		LegacySessionKey: meta.Key,
		TranscriptPath:   meta.TranscriptPath,
		ReadOnly:         false,
	}
}

func ImportedChatID(meta state.SessionMeta) string {
	return "chat-" + shortHash(chatIDSource(meta))
}

func LegacyChatID(meta state.SessionMeta) string {
	return ImportedChatID(meta)
}

func LegacyChatAliasID(meta state.SessionMeta) string {
	return "legacy-chat-" + shortHash(chatIDSource(meta))
}

func chatIDSource(meta state.SessionMeta) string {
	source := strings.TrimSpace(meta.Agent) + "\n" + strings.TrimSpace(meta.TranscriptPath)
	if strings.TrimSpace(meta.TranscriptPath) == "" {
		source = strings.TrimSpace(meta.Key)
	}
	if strings.TrimSpace(source) == "" {
		source = strings.TrimSpace(meta.SessionID)
	}
	return source
}

func ImportedProjectID(meta state.SessionMeta) string {
	return "project-" + shortHash(projectIDSource(meta))
}

func LegacyProjectAliasID(meta state.SessionMeta) string {
	return "legacy-project-" + shortHash(projectIDSource(meta))
}

func projectIDSource(meta state.SessionMeta) string {
	source := strings.TrimSpace(meta.Cwd)
	if source == "" {
		source = strings.TrimSpace(meta.ProjectName)
	}
	if source == "" {
		source = filepath.Dir(strings.TrimSpace(meta.TranscriptPath))
	}
	return source
}

func fillMetaFromMessages(meta state.SessionMeta, messages []parser.Message) state.SessionMeta {
	if meta.FirstPreview != "" && meta.LastPreview != "" && meta.MessageCountEstimate > 0 {
		return meta
	}
	meta.MessageCountEstimate = len(messages)
	firstAnyPreview := ""
	firstUserPreview := ""
	lastPreview := ""
	for _, message := range messages {
		text := strings.TrimSpace(message.Text)
		if text == "" {
			continue
		}
		isBootstrap := state.IsAgentBootstrapPrompt(text)
		if firstAnyPreview == "" && !isBootstrap {
			firstAnyPreview = text
		}
		if firstUserPreview == "" && strings.EqualFold(strings.TrimSpace(message.Role), "user") && !isBootstrap {
			firstUserPreview = text
		}
		lastPreview = text
	}
	if meta.FirstPreview == "" {
		meta.FirstPreview = firstNonEmpty(firstUserPreview, firstAnyPreview)
	}
	if meta.LastPreview == "" {
		meta.LastPreview = lastPreview
	}
	return meta
}

func isGeneratedLegacySessionTitle(title string, meta state.SessionMeta) bool {
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
	return looksLikeGeneratedSessionID(title)
}

func looksLikeGeneratedSessionID(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "rollout-") {
		return true
	}
	dashCount := strings.Count(value, "-")
	if dashCount < 3 {
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

func projectTitle(meta state.SessionMeta) string {
	if strings.TrimSpace(meta.ProjectName) != "" {
		return meta.ProjectName
	}
	if strings.TrimSpace(meta.Cwd) != "" {
		return filepath.Base(meta.Cwd)
	}
	return "unknown"
}

func mapMessages(meta state.SessionMeta, messages []parser.Message) []readmodels.TranscriptEntry {
	entries := make([]readmodels.TranscriptEntry, 0, len(messages))
	for _, message := range messages {
		entry := mapMessage(meta, message)
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func mapMessage(meta state.SessionMeta, message parser.Message) readmodels.TranscriptEntry {
	createdAt := messageCreatedAt(message)
	fields := map[string]any{
		"_id":       importedMessageID(meta, message),
		"createdAt": float64(createdAt),
		"messageId": message.ID,
	}
	switch strings.ToLower(strings.TrimSpace(message.Role)) {
	case "user":
		fields["content"] = message.Text
		return transcript.New(transcript.KindUserPrompt, fields)
	case "assistant":
		fields["text"] = message.Text
		return transcript.New(transcript.KindAssistantText, fields)
	case "tool":
		fields["toolId"] = firstNonEmpty(message.ID, fmt.Sprintf("tool-%d", message.Index))
		fields["content"] = message.Text
		return transcript.New(transcript.KindToolResult, fields)
	case "system":
		fields["status"] = message.Text
		return transcript.New(transcript.KindStatus, fields)
	default:
		fields["json"] = message.Text
		fields["role"] = message.Role
		fields["legacyKind"] = message.Kind
		return transcript.New(transcript.KindUnknown, fields)
	}
}

func importedMessageID(meta state.SessionMeta, message parser.Message) string {
	source := strings.Join([]string{
		ImportedChatID(meta),
		message.ID,
		fmt.Sprintf("%d", message.Index),
		message.Role,
		message.Kind,
	}, "\n")
	return "message-" + shortHash(source)
}

func firstTimestamp(meta state.SessionMeta, messages []parser.Message) int64 {
	for _, message := range messages {
		if message.CreatedAt != nil {
			return message.CreatedAt.UnixMilli()
		}
	}
	if !meta.UpdatedAt.IsZero() {
		return meta.UpdatedAt.UnixMilli()
	}
	return 0
}

func lastTimestamp(meta state.SessionMeta, messages []parser.Message) int64 {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].CreatedAt != nil {
			return messages[i].CreatedAt.UnixMilli()
		}
	}
	if !meta.UpdatedAt.IsZero() {
		return meta.UpdatedAt.UnixMilli()
	}
	return 0
}

func lastUserMessageTimestamp(messages []parser.Message) int64 {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			return messageCreatedAt(messages[i])
		}
	}
	return 0
}

func messageCreatedAt(message parser.Message) int64 {
	if message.CreatedAt != nil {
		return message.CreatedAt.UnixMilli()
	}
	if message.Index > 0 {
		return int64(message.Index)
	}
	return 0
}

func cursorFromEntry(entry readmodels.TranscriptEntry) string {
	if id, ok := entry["_id"].(string); ok {
		return id
	}
	return ""
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
