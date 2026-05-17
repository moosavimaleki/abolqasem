package server

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"ai-agent-manager/internal/sessioninterop"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/readmodels"
)

func workspaceCreateProject(raw json.RawMessage) (readmodels.ProjectRecord, error) {
	var payload struct {
		LocalPath string `json:"localPath"`
		Title     string `json:"title"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return readmodels.ProjectRecord{}, err
	}
	return workspaceOpenProject(payload.LocalPath, payload.Title)
}

func workspaceRenameProject(raw json.RawMessage) error {
	var payload struct {
		ProjectID string `json:"projectId"`
		Title     string `json:"title"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if _, err := workspaceProjectRequired(payload.ProjectID); err != nil {
		return err
	}
	title := strings.TrimSpace(payload.Title)
	event, err := events.New(events.TypeProjectSidebarRenamed, map[string]any{
		"projectId": payload.ProjectID,
		"title":     &title,
	})
	if err != nil {
		return err
	}
	return workspaceStore().Append(events.StreamProjects, event)
}

func workspaceRemoveProject(raw json.RawMessage) error {
	var payload struct {
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if _, err := workspaceProjectRequired(payload.ProjectID); err != nil {
		return err
	}
	event, err := events.New(events.TypeProjectRemoved, map[string]any{"projectId": payload.ProjectID})
	if err != nil {
		return err
	}
	return workspaceStore().Append(events.StreamProjects, event)
}

func workspaceRenameChat(raw json.RawMessage) (string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	chatID, err := workspaceMaterializeImportedChatIfNeeded(payload.ChatID)
	if err != nil {
		return "", err
	}
	payload.ChatID = chatID
	if _, _, err := workspaceChatProjectRequired(payload.ChatID); err != nil {
		return "", err
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		return "", errors.New("title is required")
	}
	event, err := events.New(events.TypeChatRenamed, map[string]any{"chatId": payload.ChatID, "title": title})
	if err != nil {
		return "", err
	}
	if err := workspaceStore().Append(events.StreamChats, event); err != nil {
		return "", err
	}
	return payload.ChatID, nil
}

func workspaceArchiveChat(raw json.RawMessage) (string, error) {
	return workspaceMarkChat(raw, events.TypeChatArchived)
}

func workspaceUnarchiveChat(raw json.RawMessage) (string, error) {
	return workspaceMarkChat(raw, events.TypeChatUnarchived)
}

func workspaceDeleteChat(raw json.RawMessage) (string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	chatID, err := workspaceMaterializeImportedChatIfNeeded(payload.ChatID)
	if err != nil {
		return "", err
	}
	payload.ChatID = chatID
	if err := workspaceAgentCoordinator().Cancel(payload.ChatID); err != nil {
		return "", err
	}
	return workspaceMarkChatID(payload.ChatID, events.TypeChatDeleted)
}

func workspaceForkChat(raw json.RawMessage) (map[string]any, string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", err
	}
	chatID, err := workspaceMaterializeImportedChatIfNeeded(payload.ChatID)
	if err != nil {
		return nil, "", err
	}
	payload.ChatID = chatID
	chat, project, err := workspaceChatProjectRequired(payload.ChatID)
	if err != nil {
		return nil, "", err
	}
	fork, err := workspaceCreateChat(project.ID)
	if err != nil {
		return nil, "", err
	}
	title := strings.TrimSpace(chat.Title)
	if title == "" {
		title = "Forked Chat"
	} else {
		title += " (Fork)"
	}
	renameEvent, err := events.New(events.TypeChatRenamed, map[string]any{"chatId": fork.ID, "title": title})
	if err != nil {
		return nil, "", err
	}
	if err := workspaceStore().Append(events.StreamChats, renameEvent); err != nil {
		return nil, "", err
	}
	if provider := derefWorkspaceString(chat.Provider); strings.TrimSpace(provider) != "" {
		providerEvent, err := events.New(events.TypeChatProviderSet, map[string]any{"chatId": fork.ID, "provider": provider})
		if err != nil {
			return nil, "", err
		}
		if err := workspaceStore().Append(events.StreamChats, providerEvent); err != nil {
			return nil, "", err
		}
	}
	entries, err := workspaceChatMessages(payload.ChatID)
	if err != nil {
		return nil, "", err
	}
	provider := strings.TrimSpace(derefWorkspaceString(chat.Provider))
	if provider == "gemini" && strings.TrimSpace(project.LocalPath) != "" {
		exportResult, err := sessioninterop.ExportNativeSession(sessioninterop.ExportArgs{
			Provider:           provider,
			LocalPath:          project.LocalPath,
			Title:              title,
			SourceSessionToken: derefWorkspaceString(chat.SessionToken),
			Entries:            entries,
			PreferFork:         true,
		})
		if err != nil {
			return nil, "", err
		}
		if exportResult.SessionToken != "" {
			store := &workspaceEventStore{store: workspaceStore()}
			if err := store.SetSessionToken(fork.ID, exportResult.SessionToken); err != nil {
				return nil, "", err
			}
		}
	} else if token := firstNonEmpty(derefWorkspaceString(chat.PendingForkSessionToken), derefWorkspaceString(chat.SessionToken)); token != "" {
		if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamTurns, events.TypePendingForkSessionTokenSet, time.Now().UnixMilli(), map[string]any{
			"chatId":                  fork.ID,
			"pendingForkSessionToken": &token,
		}); err != nil {
			return nil, "", err
		}
	}
	for _, entry := range entries {
		event, err := events.New(events.TypeMessageAppended, map[string]any{"chatId": fork.ID, "entry": entry})
		if err != nil {
			return nil, "", err
		}
		if err := workspaceStore().Append(events.StreamMessages, event); err != nil {
			return nil, "", err
		}
	}
	return map[string]any{"chatId": fork.ID}, fork.ID, nil
}

func workspaceMarkChat(raw json.RawMessage, eventType string) (string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	return workspaceMarkChatID(payload.ChatID, eventType)
}

func workspaceMarkChatID(chatID string, eventType string) (string, error) {
	normalizedChatID, err := workspaceMaterializeImportedChatIfNeeded(chatID)
	if err != nil {
		return "", err
	}
	chatID = normalizedChatID
	if _, _, err := workspaceChatProjectRequired(chatID); err != nil {
		return "", err
	}
	event, err := events.New(eventType, map[string]any{"chatId": chatID})
	if err != nil {
		return "", err
	}
	if err := workspaceStore().Append(events.StreamChats, event); err != nil {
		return "", err
	}
	return chatID, nil
}

func workspaceLoadChatHistory(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		ChatID       string `json:"chatId"`
		BeforeCursor string `json:"beforeCursor"`
		Limit        int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.Limit <= 0 {
		payload.Limit = 50
	}
	if payload.Limit > 500 {
		payload.Limit = 500
	}
	if workspaceStoredChatExists(payload.ChatID) {
		return workspaceLoadStoredChatHistory(payload.ChatID, payload.BeforeCursor, payload.Limit)
	}
	if meta, ok := workspaceLegacySessionByChatID(payload.ChatID); ok {
		return workspaceLoadLegacyChatHistory(meta, payload.BeforeCursor, payload.Limit)
	}
	return workspaceLoadStoredChatHistory(payload.ChatID, payload.BeforeCursor, payload.Limit)
}

func workspaceLoadChatHistoryAround(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		ChatID       string `json:"chatId"`
		TargetCursor string `json:"targetCursor"`
		Limit        int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.Limit <= 0 {
		payload.Limit = 80
	}
	if payload.Limit > 500 {
		payload.Limit = 500
	}
	if strings.TrimSpace(payload.TargetCursor) == "" {
		return nil, errors.New("targetCursor is required")
	}
	if workspaceStoredChatExists(payload.ChatID) {
		return workspaceLoadStoredChatHistoryAround(payload.ChatID, payload.TargetCursor, payload.Limit)
	}
	if meta, ok := workspaceLegacySessionByChatID(payload.ChatID); ok {
		return workspaceLoadLegacyChatHistoryAround(meta, payload.TargetCursor, payload.Limit)
	}
	return workspaceLoadStoredChatHistoryAround(payload.ChatID, payload.TargetCursor, payload.Limit)
}

type workspaceTranscriptIndexItem struct {
	ID              string `json:"id"`
	Sequence        int    `json:"sequence"`
	Role            string `json:"role"`
	EstimatedHeight int    `json:"estimatedHeight,omitempty"`
	HasError        bool   `json:"hasError,omitempty"`
	HasCode         bool   `json:"hasCode,omitempty"`
	IsPinned        bool   `json:"isPinned,omitempty"`
	Preview         string `json:"preview,omitempty"`
}

func workspaceReadChatTranscriptIndex(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	chatID := strings.TrimSpace(payload.ChatID)
	if chatID == "" {
		return nil, errors.New("chatId is required")
	}

	if meta, ok := workspaceLegacySessionByChatID(chatID); ok && !workspaceStoredChatExists(chatID) {
		imported, err := workspaceImportedLegacySession(meta)
		if err != nil && !workspaceLegacyTranscriptUnavailable(err) {
			return nil, err
		}
		return map[string]any{
			"chatId": chatID,
			"items":  buildWorkspaceTranscriptIndex(imported.Transcript.Messages),
		}, nil
	}

	if _, _, err := workspaceChatProjectRequired(chatID); err != nil {
		return nil, err
	}

	entries, err := workspaceChatMessages(chatID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"chatId": chatID,
		"items":  buildWorkspaceTranscriptIndex(entries),
	}, nil
}

func buildWorkspaceTranscriptIndex(entries []readmodels.TranscriptEntry) []workspaceTranscriptIndexItem {
	items := make([]workspaceTranscriptIndexItem, 0, len(entries))
	pendingToolItemIndex := map[string]int{}

	for _, entry := range entries {
		kind := workspaceEntryString(entry, "kind")
		if kind == "tool_result" {
			toolID := workspaceEntryString(entry, "toolId")
			index, ok := pendingToolItemIndex[toolID]
			if ok && index >= 0 && index < len(items) {
				items[index].HasError = items[index].HasError || workspaceEntryBool(entry, "isError")
			}
			continue
		}

		item, ok := workspaceTranscriptIndexItemFromEntry(entry, len(items))
		if !ok {
			continue
		}
		items = append(items, item)

		if kind == "tool_call" {
			toolID := workspaceEntryToolID(entry)
			if toolID != "" {
				pendingToolItemIndex[toolID] = len(items) - 1
			}
		}
	}

	return items
}

func workspaceTranscriptIndexItemFromEntry(entry readmodels.TranscriptEntry, sequence int) (workspaceTranscriptIndexItem, bool) {
	if workspaceEntryBool(entry, "hidden") {
		return workspaceTranscriptIndexItem{}, false
	}

	kind := workspaceEntryString(entry, "kind")
	role := workspaceTranscriptIndexRole(kind)
	if role == "" {
		return workspaceTranscriptIndexItem{}, false
	}

	text := workspaceTranscriptIndexText(entry)
	hasCode := workspaceTranscriptHasCode(kind, text)
	return workspaceTranscriptIndexItem{
		ID:              workspaceTranscriptCursor(entry),
		Sequence:        sequence,
		Role:            role,
		EstimatedHeight: workspaceTranscriptEstimatedHeight(role, text, hasCode),
		HasError:        kind == "result" && workspaceEntryBool(entry, "isError"),
		HasCode:         hasCode,
		Preview:         workspaceTranscriptPreviewText(text),
	}, true
}

func workspaceTranscriptIndexRole(kind string) string {
	switch kind {
	case "user_prompt":
		return "user"
	case "assistant_text":
		return "assistant"
	case "tool_call":
		return "tool"
	case "system_init", "account_info", "result", "status", "compact_summary", "context_cleared", "interrupted":
		return "system"
	default:
		return ""
	}
}

func workspaceTranscriptIndexText(entry readmodels.TranscriptEntry) string {
	switch workspaceEntryString(entry, "kind") {
	case "user_prompt":
		return workspaceEntryString(entry, "content")
	case "assistant_text":
		return workspaceEntryString(entry, "text")
	case "system_init":
		return workspaceEntryString(entry, "model")
	case "account_info":
		return workspaceEntryString(entry, "debugRaw")
	case "tool_call":
		return workspaceTranscriptToolSummary(entry)
	case "result":
		return workspaceEntryString(entry, "result")
	case "status":
		return workspaceEntryString(entry, "status")
	case "compact_summary":
		return workspaceEntryString(entry, "summary")
	default:
		return workspaceEntryString(entry, "debugRaw")
	}
}

func workspaceTranscriptToolSummary(entry readmodels.TranscriptEntry) string {
	tool, ok := entry["tool"].(map[string]any)
	if !ok {
		return ""
	}

	parts := make([]string, 0, 3)
	if name := workspaceAnyString(tool["toolName"]); name != "" {
		parts = append(parts, name)
	}
	if input, ok := tool["input"].(map[string]any); ok {
		for _, key := range []string{"command", "description", "filePath", "pattern", "text", "summary", "status"} {
			if value := workspaceAnyString(input[key]); value != "" {
				parts = append(parts, value)
				break
			}
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func workspaceTranscriptHasCode(kind string, text string) bool {
	if strings.Contains(text, "```") {
		return true
	}
	if kind == "tool_call" {
		return strings.Contains(text, "\n") || strings.Contains(text, " --") || strings.Contains(text, "/")
	}
	return false
}

func workspaceTranscriptEstimatedHeight(role string, text string, hasCode bool) int {
	base := 28
	switch role {
	case "user":
		base = 36
	case "assistant":
		base = 44
	case "tool":
		base = 38
	case "system":
		base = 30
	}

	if strings.TrimSpace(text) == "" {
		return base
	}

	runeCount := utf8.RuneCountInString(text)
	lineCount := strings.Count(text, "\n") + 1
	if wrappedLines := (runeCount + 87) / 88; wrappedLines > lineCount {
		lineCount = wrappedLines
	}
	height := base + (lineCount-1)*18
	if hasCode {
		height += 22
	}
	if height > 220 {
		return 220
	}
	return height
}

func workspaceTranscriptPreviewText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	normalized := strings.Join(strings.Fields(text), " ")
	runes := []rune(normalized)
	if len(runes) <= 140 {
		return normalized
	}
	return string(runes[:140]) + "…"
}

func workspaceEntryBool(entry readmodels.TranscriptEntry, key string) bool {
	value, ok := entry[key].(bool)
	return ok && value
}

func workspaceAnyString(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func workspaceLoadStoredChatHistory(chatID string, beforeCursor string, limit int) (map[string]any, error) {
	if _, _, err := workspaceChatProjectRequired(chatID); err != nil {
		return nil, err
	}
	entries, err := workspaceChatMessages(chatID)
	if err != nil {
		return nil, err
	}
	end := len(entries)
	if beforeCursor != "" {
		for index, entry := range entries {
			if workspaceTranscriptCursor(entry) == beforeCursor {
				end = index
				break
			}
		}
	}
	start := end - limit
	if start < 0 {
		start = 0
	}
	page := entries[start:end]
	var olderCursor *string
	if start > 0 {
		cursor := workspaceTranscriptCursor(entries[start-1])
		olderCursor = &cursor
	}
	return map[string]any{
		"messages":    page,
		"hasOlder":    start > 0,
		"olderCursor": olderCursor,
	}, nil
}

func workspaceLoadStoredChatHistoryAround(chatID string, targetCursor string, limit int) (map[string]any, error) {
	if _, _, err := workspaceChatProjectRequired(chatID); err != nil {
		return nil, err
	}
	entries, err := workspaceChatMessages(chatID)
	if err != nil {
		return nil, err
	}
	messages, hasOlder, olderCursor, targetFound := workspaceSliceTranscriptEntriesAround(entries, targetCursor, limit)
	return map[string]any{
		"messages":    messages,
		"hasOlder":    hasOlder,
		"olderCursor": olderCursor,
		"targetFound": targetFound,
	}, nil
}

func workspaceMaterializeImportedChatIfNeeded(chatID string) (string, error) {
	if _, ok := workspaceLegacySessionByChatID(chatID); !ok {
		return chatID, nil
	}
	return workspaceMaterializeLegacyChat(chatID)
}

func workspaceStoredChatExists(chatID string) bool {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return false
	}
	state, err := workspaceStore().LoadStateLight()
	if err != nil {
		return false
	}
	chat, ok := state.ChatsByID[chatID]
	return ok && chat.DeletedAt == 0
}

func workspaceLoadLegacyChatHistory(meta state.SessionMeta, beforeCursor string, limit int) (map[string]any, error) {
	imported, err := workspaceImportedLegacySession(meta)
	if err != nil && !workspaceLegacyTranscriptUnavailable(err) {
		return nil, err
	}
	if len(imported.Transcript.Messages) == 0 {
		return map[string]any{
			"messages":    []readmodels.TranscriptEntry{},
			"hasOlder":    false,
			"olderCursor": nil,
		}, nil
	}
	messages, hasOlder, olderCursor := workspaceSliceTranscriptEntriesBefore(imported.Transcript.Messages, beforeCursor, limit)
	return map[string]any{
		"messages":    messages,
		"hasOlder":    hasOlder,
		"olderCursor": olderCursor,
	}, nil
}

func workspaceLoadLegacyChatHistoryAround(meta state.SessionMeta, targetCursor string, limit int) (map[string]any, error) {
	imported, err := workspaceImportedLegacySession(meta)
	if err != nil && !workspaceLegacyTranscriptUnavailable(err) {
		return nil, err
	}
	messages, hasOlder, olderCursor, targetFound := workspaceSliceTranscriptEntriesAround(imported.Transcript.Messages, targetCursor, limit)
	return map[string]any{
		"messages":    messages,
		"hasOlder":    hasOlder,
		"olderCursor": olderCursor,
		"targetFound": targetFound,
	}, nil
}

func workspaceChatMessages(chatID string) ([]readmodels.TranscriptEntry, error) {
	return workspaceStore().ReplayTranscriptEntriesForChat(chatID, 0)
}

func workspaceProjectRequired(projectID string) (readmodels.ProjectRecord, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return readmodels.ProjectRecord{}, errors.New("projectId is required")
	}
	state, err := workspaceStore().LoadStateLight()
	if err != nil {
		return readmodels.ProjectRecord{}, err
	}
	project, ok := state.ProjectsByID[projectID]
	if !ok || project.DeletedAt != 0 {
		return readmodels.ProjectRecord{}, errors.New("project not found")
	}
	return project, nil
}

func workspaceRuntimeProjectRequired(projectID string) (readmodels.ProjectRecord, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return readmodels.ProjectRecord{}, errors.New("projectId is required")
	}
	state, err := workspaceStore().LoadStateLight()
	if err != nil {
		return readmodels.ProjectRecord{}, err
	}
	if project, ok := state.ProjectsByID[projectID]; ok && project.DeletedAt == 0 {
		return project, nil
	}
	if project, ok := workspaceLegacyProjectByID(projectID); ok && strings.TrimSpace(project.LocalPath) != "" {
		return project, nil
	}
	return readmodels.ProjectRecord{}, errors.New("project not found")
}

func workspaceChatProjectRequired(chatID string) (readmodels.ChatRecord, readmodels.ProjectRecord, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return readmodels.ChatRecord{}, readmodels.ProjectRecord{}, errors.New("chatId is required")
	}
	state, err := workspaceStore().LoadStateLight()
	if err != nil {
		return readmodels.ChatRecord{}, readmodels.ProjectRecord{}, err
	}
	chat, ok := state.ChatsByID[chatID]
	if !ok || chat.DeletedAt != 0 {
		return readmodels.ChatRecord{}, readmodels.ProjectRecord{}, errors.New("chat not found")
	}
	project, ok := state.ProjectsByID[chat.ProjectID]
	if !ok || project.DeletedAt != 0 {
		return readmodels.ChatRecord{}, readmodels.ProjectRecord{}, errors.New("project not found")
	}
	return chat, project, nil
}

func workspaceAck() map[string]any {
	return map[string]any{"ok": true, "timestamp": time.Now().UnixMilli()}
}
