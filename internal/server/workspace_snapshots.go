package server

import (
	"context"
	"path/filepath"
	"strings"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/eventstore"
	"ai-agent-manager/internal/workspace/protocol"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/tmuxruntime"
	"ai-agent-manager/internal/workspace/transcript"
)

var workspaceDataDir = func() string {
	return filepath.Join(state.GetStateDir(), "data")
}

func workspaceStore() *eventstore.Store {
	return eventstore.New(workspaceDataDir())
}

func workspaceChatHasTmuxRuntime(chat readmodels.ChatRecord) bool {
	return strings.TrimSpace(chat.TmuxSession) != ""
}

func workspaceSidebarSnapshot() any {
	storeState, err := workspaceStore().LoadStateLight()
	if err != nil {
		return readmodels.SidebarData{ProjectGroups: []readmodels.SidebarProjectGroup{}}
	}
	return mergeLegacySidebarData(readmodels.DeriveSidebarDataWithStatus(storeState, workspaceAgentCoordinator().ActiveStatuses()))
}

func workspaceLocalProjectsSnapshot() any {
	storeState, err := workspaceStore().LoadStateLight()
	if err != nil {
		storeState = readmodels.EmptyState()
	}
	return readmodels.DeriveLocalProjectsSnapshot(storeState, nil, workspaceMachineName(), workspacePlatform())
}

func workspaceChatSnapshot(chatID string, recentLimit int) any {
	if chatID == "" {
		return nil
	}
	store := workspaceStore()
	storeState, err := store.LoadStateLight()
	if err != nil {
		return nil
	}
	if chat, ok := storeState.ChatsByID[chatID]; ok && chat.DeletedAt == 0 {
		if !workspaceChatHasTmuxRuntime(chat) {
			if meta, ok := workspaceLegacySessionByChatID(chatID); ok && workspaceSyncLegacyForSnapshotIfNeeded(store, chatID, chat, meta) {
				storeState, _ = store.LoadStateLight()
				if refreshedChat, ok := storeState.ChatsByID[chatID]; ok {
					chat = refreshedChat
				}
			} else if meta, ok := workspaceLegacySessionByProviderToken(derefWorkspaceString(chat.Provider), derefWorkspaceString(chat.SessionToken)); ok && workspaceSyncLegacyForSnapshotIfNeeded(store, chatID, chat, meta) {
				storeState, _ = store.LoadStateLight()
				if refreshedChat, ok := storeState.ChatsByID[chatID]; ok {
					chat = refreshedChat
				}
			}
		}
		var transcript readmodels.ChatTranscriptSnapshot
		if workspaceChatHasTmuxRuntime(chat) {
			transcript = workspaceTmuxTranscriptSnapshot(chat, recentLimit)
		} else {
			transcript, err = workspaceChatTranscriptSnapshot(store, chatID, recentLimit)
			if err != nil {
				return nil
			}
			if refreshedState, refreshed := workspaceBackfillLegacySessionTokenForSnapshot(store, storeState, chatID, transcript.Messages); refreshed {
				storeState = refreshedState
			}
		}
		coordinator := workspaceAgentCoordinator()
		snapshot := readmodels.DeriveChatSnapshot(storeState, coordinator.ActiveStatuses(), coordinator.DrainingChatIDs(), chatID, transcript)
		if snapshot != nil {
			snapshot.AvailableProviders = workspaceAvailableProviders()
			applyTmuxRuntimeStatus(snapshot)
		}
		return snapshot
	}
	if _, ok := workspaceLegacySessionByChatID(chatID); ok {
		if materializedChatID, err := workspaceMaterializeLegacyChat(chatID); err == nil && materializedChatID != "" {
			return workspaceChatSnapshot(materializedChatID, recentLimit)
		}
	}
	if snapshot := workspaceLegacyChatSnapshot(chatID, recentLimit); snapshot != nil {
		return snapshot
	}
	return nil
}

func workspaceTmuxTranscriptSnapshot(chat readmodels.ChatRecord, recentLimit int) readmodels.ChatTranscriptSnapshot {
	lines := recentLimit
	if lines <= 0 {
		lines = 1000
	}
	if firstNonEmpty(chat.NativeSessionID, derefWorkspaceString(chat.SessionToken), derefWorkspaceString(chat.PendingForkSessionToken)) != "" {
		if projectPath, err := workspaceProjectLocalPathRequired(chat.ProjectID); err == nil {
			_ = tmuxruntime.EnsureSession(context.Background(), chat.TmuxSession, projectPath, workspaceTmuxCommandForChat(chat, ""))
		}
	}
	output, err := tmuxruntime.Capture(context.Background(), chat.TmuxSession, lines)
	if err != nil || strings.TrimSpace(output) == "" {
		return readmodels.ChatTranscriptSnapshot{History: readmodels.ChatHistorySnapshot{RecentLimit: recentLimit}}
	}
	return readmodels.ChatTranscriptSnapshot{
		Messages: []readmodels.TranscriptEntry{
			transcript.New(transcript.KindAssistantText, map[string]any{
				"_id":  "tmux-capture-" + chat.ID,
				"text": strings.TrimSpace(output),
			}),
		},
		History: readmodels.ChatHistorySnapshot{RecentLimit: recentLimit},
	}
}

func applyTmuxRuntimeStatus(snapshot *readmodels.ChatSnapshot) {
	if snapshot == nil || strings.TrimSpace(snapshot.Runtime.TmuxSession) == "" {
		return
	}
	status, err := tmuxruntime.ReadStatus(context.Background(), snapshot.Runtime.TmuxSession)
	if err != nil {
		return
	}
	switch status.State {
	case "waiting":
		snapshot.Runtime.Status = readmodels.StatusWaitingForUser
	case "idle":
		snapshot.Runtime.Status = readmodels.StatusIdle
	case "running":
		snapshot.Runtime.Status = readmodels.StatusRunning
	}
}

func workspaceBackfillLegacySessionTokenForSnapshot(
	store *eventstore.Store,
	storeState readmodels.StoreState,
	chatID string,
	messages []readmodels.TranscriptEntry,
) (readmodels.StoreState, bool) {
	chat, ok := storeState.ChatsByID[chatID]
	if !ok || chat.DeletedAt != 0 {
		return storeState, false
	}
	if strings.TrimSpace(derefWorkspaceString(chat.SessionToken)) != "" || strings.TrimSpace(derefWorkspaceString(chat.PendingForkSessionToken)) != "" {
		return storeState, false
	}
	meta, ok := workspaceLegacySessionForStoredChat(chat, messages)
	if !ok {
		return storeState, false
	}
	sessionToken := strings.TrimSpace(meta.SessionID)
	if sessionToken == "" {
		return storeState, false
	}
	if err := (&workspaceEventStore{store: store}).SetSessionToken(chatID, sessionToken); err != nil {
		return storeState, false
	}
	refreshedState, err := store.LoadStateLight()
	if err != nil {
		return storeState, false
	}
	return refreshedState, true
}

func workspaceSyncLegacyForSnapshotIfNeeded(store *eventstore.Store, chatID string, chat readmodels.ChatRecord, meta state.SessionMeta) bool {
	if !workspaceLegacySessionNeedsSync(chat, meta) {
		return false
	}
	if workspaceLegacyRestoreAlreadyCovers(store, chatID, meta) {
		_ = workspaceSyncLegacyBackedChat(chatID, meta)
		return true
	}
	_ = workspaceSyncLegacyBackedChat(chatID, meta)
	return true
}

func workspaceLegacyRestoreAlreadyCovers(store *eventstore.Store, chatID string, meta state.SessionMeta) bool {
	metaUpdatedAt := meta.UpdatedAt.UnixMilli()
	if metaUpdatedAt <= 0 {
		return false
	}
	eventType, eventTimestamp, err := store.LastMessageEventForChat(chatID)
	if err != nil {
		return false
	}
	return eventType == events.TypeChatRestoredToCheckpoint && eventTimestamp >= metaUpdatedAt
}

func subscriptionRecentLimit(topic protocol.SubscriptionTopic) int {
	if topic.RecentLimit == nil {
		return 0
	}
	if *topic.RecentLimit < 0 {
		return 0
	}
	return *topic.RecentLimit
}

func workspaceChatTranscriptSnapshot(store *eventstore.Store, chatID string, recentLimit int) (readmodels.ChatTranscriptSnapshot, error) {
	tailLimit := 0
	if recentLimit > 0 {
		tailLimit = recentLimit + 1
	}

	entries, err := store.ReplayTranscriptEntriesForChat(chatID, tailLimit)
	if err != nil {
		return readmodels.ChatTranscriptSnapshot{}, err
	}

	hasOlder := false
	var olderCursor *string
	if recentLimit > 0 && len(entries) > recentLimit {
		hasOlder = true
		cursor := workspaceTranscriptCursor(entries[len(entries)-recentLimit-1])
		olderCursor = &cursor
		entries = entries[len(entries)-recentLimit:]
	}
	return readmodels.ChatTranscriptSnapshot{
		Messages: workspaceTrimTranscriptSnapshotPayload(entries),
		History: readmodels.ChatHistorySnapshot{
			HasOlder:    hasOlder,
			OlderCursor: olderCursor,
			RecentLimit: recentLimit,
		},
	}, nil
}

func workspaceTrimTranscriptSnapshotPayload(entries []readmodels.TranscriptEntry) []readmodels.TranscriptEntry {
	toolKinds := map[string]string{}
	trimmed := make([]readmodels.TranscriptEntry, 0, len(entries))
	for _, entry := range entries {
		if workspaceEntryString(entry, "kind") == "tool_call" {
			if toolID := workspaceEntryToolID(entry); toolID != "" {
				toolKinds[toolID] = workspaceEntryToolKind(entry)
			}
			trimmed = append(trimmed, entry)
			continue
		}

		if workspaceEntryString(entry, "kind") == "tool_result" && workspaceEntryString(entry, "debugRaw") != "" {
			toolID := workspaceEntryString(entry, "toolId")
			if !workspaceToolResultNeedsDebugRaw(toolKinds[toolID]) {
				trimmed = append(trimmed, workspaceEntryWithoutField(entry, "debugRaw"))
				continue
			}
		}

		trimmed = append(trimmed, entry)
	}
	return trimmed
}

func workspaceEntryToolKind(entry readmodels.TranscriptEntry) string {
	tool, ok := entry["tool"].(map[string]any)
	if !ok {
		return ""
	}
	if value, ok := tool["toolKind"].(string); ok {
		return value
	}
	return ""
}

func workspaceToolResultNeedsDebugRaw(toolKind string) bool {
	return toolKind == "ask_user_question" || toolKind == "exit_plan_mode"
}

func workspaceEntryWithoutField(entry readmodels.TranscriptEntry, field string) readmodels.TranscriptEntry {
	clone := make(readmodels.TranscriptEntry, len(entry))
	for key, value := range entry {
		if key == field {
			continue
		}
		clone[key] = value
	}
	return clone
}

func workspaceTranscriptCursor(entry readmodels.TranscriptEntry) string {
	for _, key := range []string{"_id", "messageId", "id"} {
		if value, ok := entry[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
