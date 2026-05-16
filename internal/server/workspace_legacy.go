package server

import (
	"errors"
	"sort"
	"strings"

	"ai-agent-manager/internal/parser"
	"ai-agent-manager/internal/providers/catalog"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/legacyimport"
	"ai-agent-manager/internal/workspace/readmodels"
)

const legacyDefaultRecentLimit = 200

var workspaceLoadLegacyState = state.LoadState

func mergeLegacySidebarData(sidebar readmodels.SidebarData) readmodels.SidebarData {
	sessions := workspaceLegacySessions()
	if len(sessions) == 0 {
		return sidebar
	}

	groupIndexByPath := map[string]int{}
	for index, group := range sidebar.ProjectGroups {
		if group.LocalPath != "" {
			groupIndexByPath[group.LocalPath] = index
		}
	}

	for _, meta := range sessions {
		imported := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{})
		row := legacySidebarRow(imported, meta)
		if row.ChatID == "" {
			continue
		}
		groupIndex, ok := groupIndexByPath[imported.Project.LocalPath]
		if !ok {
			sidebar.ProjectGroups = append(sidebar.ProjectGroups, readmodels.SidebarProjectGroup{
				GroupKey:         imported.Project.ID,
				Title:            imported.Project.Title,
				RealTitle:        imported.Project.Title,
				LocalPath:        imported.Project.LocalPath,
				Chats:            []readmodels.SidebarChatRow{},
				PreviewChats:     []readmodels.SidebarChatRow{},
				OlderChats:       []readmodels.SidebarChatRow{},
				ArchivedChats:    []readmodels.SidebarChatRow{},
				DefaultCollapsed: false,
			})
			groupIndex = len(sidebar.ProjectGroups) - 1
			if imported.Project.LocalPath != "" {
				groupIndexByPath[imported.Project.LocalPath] = groupIndex
			}
		}
		if !sidebarHasChat(sidebar.ProjectGroups[groupIndex], row.ChatID) {
			sidebar.ProjectGroups[groupIndex].Chats = append(sidebar.ProjectGroups[groupIndex].Chats, row)
		}
	}

	for index := range sidebar.ProjectGroups {
		sort.SliceStable(sidebar.ProjectGroups[index].Chats, func(i, j int) bool {
			return sidebarChatTimestamp(sidebar.ProjectGroups[index].Chats[i]) > sidebarChatTimestamp(sidebar.ProjectGroups[index].Chats[j])
		})
	}
	sort.SliceStable(sidebar.ProjectGroups, func(i, j int) bool {
		return groupTimestamp(sidebar.ProjectGroups[i]) > groupTimestamp(sidebar.ProjectGroups[j])
	})
	return sidebar
}

func workspaceLegacyChatSnapshot(chatID string, recentLimit int) any {
	meta, ok := workspaceLegacySessionByChatID(chatID)
	if !ok {
		return nil
	}
	limit := recentLimit
	if limit <= 0 {
		limit = legacyDefaultRecentLimit
	}

	var messages []parser.Message
	var history readmodels.ChatHistorySnapshot
	result, err := parser.ParseMessages(meta.Agent, meta.SessionID, meta.TranscriptPath, parser.ParseOptions{Limit: limit})
	if err == nil && result != nil {
		messages = result.Items
		history.HasOlder = result.HasMoreBefore
		if result.OldestCursor != "" {
			cursor := result.OldestCursor
			history.OlderCursor = &cursor
		}
		history.RecentLimit = limit
	} else if err != nil && !errors.Is(err, parser.ErrTranscriptUnavailable) {
		history.RecentLimit = limit
	}

	imported := legacyimport.ImportSession(meta, messages, legacyimport.ImportOptions{RecentLimit: limit})
	if history.RecentLimit == 0 {
		history = imported.Transcript.History
	}
	return &readmodels.ChatSnapshot{
		Runtime: readmodels.ChatRuntime{
			ChatID:           imported.Chat.ID,
			ProjectID:        imported.Project.ID,
			LocalPath:        imported.Project.LocalPath,
			Title:            imported.Chat.Title,
			Status:           readmodels.StatusIdle,
			IsDraining:       false,
			Provider:         imported.Chat.Provider,
			PlanMode:         false,
			SessionToken:     nil,
			ReadOnly:         true,
			CanResume:        legacyCanResume(meta),
			LegacySessionKey: meta.Key,
		},
		QueuedMessages:     []readmodels.QueuedChatMessage{},
		Messages:           imported.Transcript.Messages,
		History:            history,
		AvailableProviders: catalog.ServerProviders(),
	}
}

func workspaceLegacySessions() []state.SessionMeta {
	appState, err := workspaceLoadLegacyState()
	if err != nil || appState == nil {
		return nil
	}
	sessions := make([]state.SessionMeta, 0, len(appState.Sessions))
	for _, meta := range appState.Sessions {
		if strings.TrimSpace(meta.Key) == "" {
			continue
		}
		sessions = append(sessions, meta)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions
}

func workspaceLegacySessionByChatID(chatID string) (state.SessionMeta, bool) {
	if !strings.HasPrefix(chatID, "legacy-chat-") {
		return state.SessionMeta{}, false
	}
	for _, meta := range workspaceLegacySessions() {
		if legacyimport.LegacyChatID(meta) == chatID {
			return meta, true
		}
	}
	return state.SessionMeta{}, false
}

func legacySidebarRow(imported legacyimport.ImportedSession, meta state.SessionMeta) readmodels.SidebarChatRow {
	lastMessageAt := imported.Chat.LastMessageAt
	return readmodels.SidebarChatRow{
		ID:               imported.Chat.ID,
		CreationTime:     imported.Chat.CreatedAt,
		ChatID:           imported.Chat.ID,
		Title:            imported.Chat.Title,
		Status:           string(readmodels.StatusIdle),
		Unread:           false,
		LocalPath:        imported.Project.LocalPath,
		Provider:         imported.Chat.Provider,
		LastMessageAt:    &lastMessageAt,
		HasAutomation:    false,
		CanFork:          false,
		ReadOnly:         true,
		CanResume:        legacyCanResume(meta),
		LegacySessionKey: meta.Key,
	}
}

func legacyCanResume(meta state.SessionMeta) bool {
	return strings.EqualFold(strings.TrimSpace(meta.Agent), "codex") &&
		!meta.MetadataOnly &&
		strings.TrimSpace(meta.SessionID) != ""
}

func sidebarHasChat(group readmodels.SidebarProjectGroup, chatID string) bool {
	for _, chat := range group.Chats {
		if chat.ChatID == chatID {
			return true
		}
	}
	return false
}

func sidebarChatTimestamp(chat readmodels.SidebarChatRow) int64 {
	if chat.LastMessageAt != nil {
		return *chat.LastMessageAt
	}
	return chat.CreationTime
}

func groupTimestamp(group readmodels.SidebarProjectGroup) int64 {
	var newest int64
	for _, chat := range group.Chats {
		if timestamp := sidebarChatTimestamp(chat); timestamp > newest {
			newest = timestamp
		}
	}
	return newest
}
