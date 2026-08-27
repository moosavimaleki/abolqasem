package server

import (
	"path/filepath"
	"strings"
	"time"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/eventstore"
	"abolqasem/internal/workspace/protocol"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

var workspaceDataDir = func() string {
	return filepath.Join(state.GetStateDir(), "data")
}

func workspaceStore() *eventstore.Store {
	return eventstore.New(workspaceDataDir())
}

func workspaceChatHasTmuxRuntime(chat readmodels.ChatRecord) bool {
	// tmux used to be the chat transport. Existing records may still have this
	// metadata, but it must never decide how their history is read or how a new
	// turn is started. Native provider transcripts are the source of truth.
	_ = chat
	return false
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
	// Version 1.5.0 and earlier could leave an already accepted steer in durable
	// queue state when its websocket acknowledgement raced with reconnect.
	// Reconcile before reading so that stale delivery rows cannot survive a
	// refresh or be submitted as a later turn.
	_ = workspaceAgentCoordinator().ReconcileQueued(chatID)
	store := workspaceStore()
	storeState, err := store.LoadStateLight()
	if err != nil {
		return nil
	}
	if chat, ok := storeState.ChatsByID[chatID]; ok && chat.DeletedAt == 0 {
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
		var transcript readmodels.ChatTranscriptSnapshot
		if native, ok := workspaceNativeTranscriptSnapshot(chat, recentLimit); ok {
			transcript = native
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
			if pendingTool := coordinator.PendingTool(chatID); pendingTool != nil {
				snapshot.Messages = append(snapshot.Messages, workspacePendingToolTranscriptEntry(pendingTool))
			}
			snapshot.AvailableProviders = workspaceAvailableProviders()
			lock := workspaceCodexLockStatus(chat)
			snapshot.Runtime.CodexLock = lock
			snapshot.Runtime.ReadOnly = lock.State == codexLockOwnedElsewhere || lock.State == codexLockUnknown
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

// Native Codex session JSONL does not persist item/tool/requestUserInput as a
// transcript item. Keep it in the live chat snapshot while the turn waits, so
// the existing AskUserQuestion card remains available in Plan Mode.
func workspacePendingToolTranscriptEntry(pending *agent.PendingToolSnapshot) readmodels.TranscriptEntry {
	toolID := strings.TrimSpace(pending.ToolUseID)
	createdAt := pending.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	return readmodels.TranscriptEntry{
		"_id":       "pending-tool-" + toolID,
		"messageId": toolID,
		"createdAt": float64(createdAt),
		"kind":      transcript.KindToolCall,
		"tool": map[string]any{
			"kind":     "tool",
			"toolKind": pending.ToolKind,
			"toolName": pending.ToolName,
			"toolId":   toolID,
			"input":    pending.Input,
		},
	}
}

func workspaceNativeTranscriptSnapshot(chat readmodels.ChatRecord, recentLimit int) (readmodels.ChatTranscriptSnapshot, bool) {
	if strings.TrimSpace(chat.NativeTranscriptPath) == "" {
		return readmodels.ChatTranscriptSnapshot{}, false
	}
	stateSnapshot, err := workspaceStore().LoadStateLight()
	if err != nil {
		return readmodels.ChatTranscriptSnapshot{}, false
	}
	project, ok := stateSnapshot.ProjectsByID[chat.ProjectID]
	if !ok || project.DeletedAt != 0 {
		return readmodels.ChatTranscriptSnapshot{}, false
	}
	meta, ok := workspaceNativeTranscriptMetaForChatRecord(chat, project)
	if !ok {
		return readmodels.ChatTranscriptSnapshot{}, false
	}
	page, err := workspaceLoadNativeChatHistory(meta, "", recentLimit)
	if err != nil {
		return readmodels.ChatTranscriptSnapshot{}, false
	}
	messages, _ := page["messages"].([]readmodels.TranscriptEntry)
	hasOlder, _ := page["hasOlder"].(bool)
	var olderCursor *string
	switch cursor := page["olderCursor"].(type) {
	case *string:
		olderCursor = cursor
	case string:
		if cursor != "" {
			olderCursor = &cursor
		}
	}
	return readmodels.ChatTranscriptSnapshot{
		Messages: workspaceTrimTranscriptSnapshotPayload(messages),
		History: readmodels.ChatHistorySnapshot{
			HasOlder:    hasOlder,
			OlderCursor: olderCursor,
			RecentLimit: recentLimit,
		},
	}, true
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
