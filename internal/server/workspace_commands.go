package server

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	entries, err := workspaceChatMessages(payload.ChatID)
	if err != nil {
		return nil, "", err
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
	if _, _, err := workspaceChatProjectRequired(payload.ChatID); err != nil {
		return nil, err
	}
	if payload.Limit <= 0 {
		payload.Limit = 50
	}
	if payload.Limit > 500 {
		payload.Limit = 500
	}
	entries, err := workspaceChatMessages(payload.ChatID)
	if err != nil {
		return nil, err
	}
	end := len(entries)
	if payload.BeforeCursor != "" {
		for index, entry := range entries {
			if workspaceTranscriptCursor(entry) == payload.BeforeCursor {
				end = index
				break
			}
		}
	}
	start := end - payload.Limit
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

func workspaceChatMessages(chatID string) ([]readmodels.TranscriptEntry, error) {
	messageEvents, err := workspaceStore().Replay(events.StreamMessages)
	if err != nil {
		return nil, err
	}
	entries := make([]readmodels.TranscriptEntry, 0)
	for _, event := range messageEvents {
		if event.Type != events.TypeMessageAppended {
			continue
		}
		var data struct {
			ChatID string                     `json:"chatId"`
			Entry  readmodels.TranscriptEntry `json:"entry"`
		}
		if event.DecodeData(&data) != nil || data.ChatID != chatID {
			continue
		}
		entries = append(entries, data.Entry)
	}
	return entries, nil
}

func workspaceProjectRequired(projectID string) (readmodels.ProjectRecord, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return readmodels.ProjectRecord{}, errors.New("projectId is required")
	}
	state, err := workspaceStore().LoadState()
	if err != nil {
		return readmodels.ProjectRecord{}, err
	}
	project, ok := state.ProjectsByID[projectID]
	if !ok || project.DeletedAt != 0 {
		return readmodels.ProjectRecord{}, errors.New("project not found")
	}
	return project, nil
}

func workspaceChatProjectRequired(chatID string) (readmodels.ChatRecord, readmodels.ProjectRecord, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return readmodels.ChatRecord{}, readmodels.ProjectRecord{}, errors.New("chatId is required")
	}
	state, err := workspaceStore().LoadState()
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
