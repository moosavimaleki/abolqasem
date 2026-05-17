package server

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ai-agent-manager/internal/sessioninterop"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

type workspaceSessionConversionResult struct {
	ChatID               string `json:"chatId"`
	ProjectID            string `json:"projectId"`
	Provider             string `json:"provider"`
	ImportedMessageCount int    `json:"importedMessageCount"`
	PendingFork          bool   `json:"pendingFork"`
}

type workspaceSessionConversionPreview struct {
	SourceTitle          string `json:"sourceTitle"`
	SourceProvider       string `json:"sourceProvider"`
	TargetProvider       string `json:"targetProvider"`
	TargetProjectID      string `json:"targetProjectId,omitempty"`
	TargetProjectTitle   string `json:"targetProjectTitle,omitempty"`
	UserMessages         int    `json:"userMessages"`
	AssistantMessages    int    `json:"assistantMessages"`
	ToolCalls            int    `json:"toolCalls"`
	ToolResults          int    `json:"toolResults"`
	CompactBoundaries    int    `json:"compactBoundaries"`
	CompactSummaries     int    `json:"compactSummaries"`
	SkippedEntries       int    `json:"skippedEntries"`
	ImportedMessageCount int    `json:"importedMessageCount"`
	PendingFork          bool   `json:"pendingFork"`
}

func workspacePreviewConvertChat(raw json.RawMessage) (workspaceSessionConversionPreview, error) {
	var payload struct {
		ChatID          string `json:"chatId"`
		TargetProvider  string `json:"targetProvider"`
		TargetProjectID string `json:"targetProjectId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return workspaceSessionConversionPreview{}, err
	}
	payload.TargetProvider = strings.TrimSpace(payload.TargetProvider)
	if payload.TargetProvider == "" {
		return workspaceSessionConversionPreview{}, errors.New("targetProvider is required")
	}
	source, err := workspaceConversionSource(payload.ChatID)
	if err != nil {
		return workspaceSessionConversionPreview{}, err
	}
	targetProject, err := workspaceConversionTargetProject(source, payload.TargetProjectID)
	if err != nil {
		return workspaceSessionConversionPreview{}, err
	}
	stats := workspaceConversionEntryStats(source.Entries)
	entries := workspaceConvertedTranscriptEntries(source.Entries)
	return workspaceSessionConversionPreview{
		SourceTitle:          source.Title,
		SourceProvider:       source.Provider,
		TargetProvider:       payload.TargetProvider,
		TargetProjectID:      targetProject.ID,
		TargetProjectTitle:   targetProject.Title,
		UserMessages:         stats.UserMessages,
		AssistantMessages:    stats.AssistantMessages,
		ToolCalls:            stats.ToolCalls,
		ToolResults:          stats.ToolResults,
		CompactBoundaries:    stats.CompactBoundaries,
		CompactSummaries:     stats.CompactSummaries,
		SkippedEntries:       len(source.Entries) - len(entries),
		ImportedMessageCount: len(entries),
		PendingFork:          false,
	}, nil
}

func workspaceConvertChat(raw json.RawMessage) (workspaceSessionConversionResult, string, error) {
	var payload struct {
		ChatID          string `json:"chatId"`
		TargetProvider  string `json:"targetProvider"`
		TargetProjectID string `json:"targetProjectId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	payload.TargetProvider = strings.TrimSpace(payload.TargetProvider)
	if payload.TargetProvider == "" {
		return workspaceSessionConversionResult{}, "", errors.New("targetProvider is required")
	}

	source, err := workspaceConversionSource(payload.ChatID)
	if err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	targetProject, err := workspaceConversionTargetProject(source, payload.TargetProjectID)
	if err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	fork, err := workspaceCreateChat(targetProject.ID)
	if err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	store := &workspaceEventStore{store: workspaceStore()}
	if err := store.SetChatProvider(fork.ID, payload.TargetProvider); err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	if err := store.SetPlanMode(fork.ID, false); err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	if err := store.EnsureSystemInit(fork.ID, payload.TargetProvider, ""); err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	if err := workspaceRenameConvertedChat(fork.ID, source.Title, payload.TargetProvider); err != nil {
		return workspaceSessionConversionResult{}, "", err
	}

	entries := workspaceConvertedTranscriptEntries(source.Entries)
	exportResult, err := sessioninterop.ExportNativeSession(sessioninterop.ExportArgs{
		Provider:           payload.TargetProvider,
		LocalPath:          targetProject.LocalPath,
		Title:              source.Title,
		SourceSessionToken: source.SessionToken,
		Entries:            entries,
		PreferFork:         false,
	})
	if err != nil {
		return workspaceSessionConversionResult{}, "", err
	}
	for _, entry := range entries {
		if err := store.AppendTranscriptEntry(fork.ID, entry); err != nil {
			return workspaceSessionConversionResult{}, "", err
		}
	}
	pendingFork := false
	if exportResult.SessionToken != "" {
		if err := store.SetSessionToken(fork.ID, exportResult.SessionToken); err != nil {
			return workspaceSessionConversionResult{}, "", err
		}
	}
	return workspaceSessionConversionResult{
		ChatID:               fork.ID,
		ProjectID:            targetProject.ID,
		Provider:             payload.TargetProvider,
		ImportedMessageCount: len(entries),
		PendingFork:          pendingFork,
	}, fork.ID, nil
}

type workspaceConversionSourceSnapshot struct {
	Title        string
	Provider     string
	SessionToken string
	LocalPath    string
	ProjectTitle string
	Entries      []readmodels.TranscriptEntry
}

func workspaceConversionSource(chatID string) (workspaceConversionSourceSnapshot, error) {
	if chat, project, err := workspaceChatProjectRequired(chatID); err == nil {
		entries, err := workspaceChatMessages(chat.ID)
		if err != nil {
			return workspaceConversionSourceSnapshot{}, err
		}
		return workspaceConversionSourceSnapshot{
			Title:        chat.Title,
			Provider:     derefWorkspaceString(chat.Provider),
			SessionToken: derefWorkspaceString(chat.SessionToken),
			LocalPath:    project.LocalPath,
			ProjectTitle: project.Title,
			Entries:      entries,
		}, nil
	}
	meta, ok := workspaceLegacySessionByChatID(chatID)
	if !ok {
		return workspaceConversionSourceSnapshot{}, errors.New("chat not found")
	}
	imported, err := sessioninterop.ImportLegacySession(meta)
	if err != nil {
		return workspaceConversionSourceSnapshot{}, err
	}
	projectTitle := imported.ProjectName
	if projectTitle == "" {
		projectTitle = strings.TrimSpace(meta.ProjectName)
	}
	return workspaceConversionSourceSnapshot{
		Title:        imported.SessionName,
		Provider:     imported.Provider,
		SessionToken: imported.SessionToken,
		LocalPath:    imported.LocalPath,
		ProjectTitle: projectTitle,
		Entries:      imported.Entries,
	}, nil
}

func workspaceConversionTargetProject(source workspaceConversionSourceSnapshot, targetProjectID string) (readmodels.ProjectRecord, error) {
	targetProjectID = strings.TrimSpace(targetProjectID)
	if targetProjectID != "" {
		return workspaceProjectRequired(targetProjectID)
	}
	if strings.TrimSpace(source.LocalPath) == "" {
		return readmodels.ProjectRecord{}, errors.New("source project path is missing")
	}
	return workspaceOpenProject(source.LocalPath, firstNonEmpty(source.ProjectTitle, source.LocalPath))
}

type workspaceConversionStats struct {
	UserMessages      int
	AssistantMessages int
	ToolCalls         int
	ToolResults       int
	CompactBoundaries int
	CompactSummaries  int
}

func workspaceConversionEntryStats(entries []readmodels.TranscriptEntry) workspaceConversionStats {
	var stats workspaceConversionStats
	for _, entry := range entries {
		switch transcript.Kind(entry) {
		case transcript.KindUserPrompt:
			stats.UserMessages++
		case transcript.KindAssistantText:
			stats.AssistantMessages++
		case transcript.KindToolCall:
			stats.ToolCalls++
		case transcript.KindToolResult:
			stats.ToolResults++
		case transcript.KindCompactBoundary:
			stats.CompactBoundaries++
		case transcript.KindCompactSummary:
			stats.CompactSummaries++
		}
	}
	return stats
}

func workspaceConvertedTranscriptEntries(entries []readmodels.TranscriptEntry) []readmodels.TranscriptEntry {
	out := make([]readmodels.TranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		switch transcript.Kind(entry) {
		case transcript.KindSystemInit, transcript.KindAccountInfo, transcript.KindContextWindowUpdated, transcript.KindRateLimitUpdated:
			continue
		default:
			out = append(out, entry)
		}
	}
	return out
}

func workspaceRenameConvertedChat(chatID string, sourceTitle string, provider string) error {
	title := strings.TrimSpace(sourceTitle)
	if title == "" {
		title = "Converted Chat"
	}
	title += " (" + strings.Title(strings.TrimSpace(provider)) + ")"
	event, err := events.New(events.TypeChatRenamed, map[string]any{"chatId": chatID, "title": title})
	if err != nil {
		return err
	}
	return workspaceStore().Append(events.StreamChats, event)
}

func workspaceSetPendingForkSessionToken(chatID string, sessionToken string) error {
	return appendWorkspaceStoreEvent(workspaceStore(), events.StreamTurns, events.TypePendingForkSessionTokenSet, time.Now().UnixMilli(), map[string]any{
		"chatId":                  chatID,
		"pendingForkSessionToken": &sessionToken,
	})
}
