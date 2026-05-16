package server

import (
	"path/filepath"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/eventstore"
	"ai-agent-manager/internal/workspace/protocol"
	"ai-agent-manager/internal/workspace/readmodels"
)

var workspaceDataDir = func() string {
	return filepath.Join(state.GetStateDir(), "data")
}

func workspaceStore() *eventstore.Store {
	return eventstore.New(workspaceDataDir())
}

func workspaceSidebarSnapshot() any {
	storeState, err := workspaceStore().LoadState()
	if err != nil {
		return readmodels.SidebarData{ProjectGroups: []readmodels.SidebarProjectGroup{}}
	}
	return readmodels.DeriveSidebarData(storeState)
}

func workspaceLocalProjectsSnapshot() any {
	storeState, err := workspaceStore().LoadState()
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
	storeState, err := store.LoadState()
	if err != nil {
		return nil
	}
	transcript, err := workspaceChatTranscriptSnapshot(store, chatID, recentLimit)
	if err != nil {
		return nil
	}
	return readmodels.DeriveChatSnapshot(storeState, workspaceAgentCoordinator().ActiveStatuses(), map[string]bool{}, chatID, transcript)
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
	messageEvents, err := store.Replay(events.StreamMessages)
	if err != nil {
		return readmodels.ChatTranscriptSnapshot{}, err
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

	hasOlder := false
	var olderCursor *string
	if recentLimit > 0 && len(entries) > recentLimit {
		hasOlder = true
		cursor := workspaceTranscriptCursor(entries[len(entries)-recentLimit-1])
		olderCursor = &cursor
		entries = entries[len(entries)-recentLimit:]
	}
	return readmodels.ChatTranscriptSnapshot{
		Messages: entries,
		History: readmodels.ChatHistorySnapshot{
			HasOlder:    hasOlder,
			OlderCursor: olderCursor,
			RecentLimit: recentLimit,
		},
	}, nil
}

func workspaceTranscriptCursor(entry readmodels.TranscriptEntry) string {
	for _, key := range []string{"_id", "messageId", "id"} {
		if value, ok := entry[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
