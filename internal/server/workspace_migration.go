package server

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/readmodels"
)

type workspaceTmuxMigrationChat struct {
	ChatID      string `json:"chatId"`
	Title       string `json:"title"`
	TmuxSession string `json:"tmuxSession"`
	LastSummary string `json:"lastSummary,omitempty"`
}

type workspaceTmuxMigrationResult struct {
	DryRun        bool                         `json:"dryRun"`
	MigratedCount int                          `json:"migratedCount"`
	SkippedCount  int                          `json:"skippedCount"`
	Compacted     bool                         `json:"compacted"`
	Chats         []workspaceTmuxMigrationChat `json:"chats"`
}

func workspaceMigrateChatsToTmux(raw json.RawMessage) (workspaceTmuxMigrationResult, error) {
	var payload struct {
		ChatIDs []string `json:"chatIds"`
		DryRun  bool     `json:"dryRun"`
		Compact *bool    `json:"compact"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return workspaceTmuxMigrationResult{}, err
	}

	store := workspaceStore()
	storeState, err := store.LoadState()
	if err != nil {
		return workspaceTmuxMigrationResult{}, err
	}

	targets, skipped := workspaceTmuxMigrationTargets(storeState, payload.ChatIDs)
	result := workspaceTmuxMigrationResult{
		DryRun:       payload.DryRun,
		SkippedCount: skipped,
		Chats:        make([]workspaceTmuxMigrationChat, 0, len(targets)),
	}

	for _, chat := range targets {
		result.Chats = append(result.Chats, workspaceTmuxMigrationChat{
			ChatID:      chat.ID,
			Title:       chat.Title,
			TmuxSession: workspaceChatTmuxSession(chat.ID),
			LastSummary: workspaceTmuxMigrationSummary(store, chat.ID),
		})
	}
	result.MigratedCount = len(result.Chats)
	if payload.DryRun || len(result.Chats) == 0 {
		return result, nil
	}

	timestamp := time.Now().UnixMilli()
	for _, chat := range result.Chats {
		data := map[string]any{
			"chatId":      chat.ChatID,
			"tmuxSession": chat.TmuxSession,
		}
		if chat.LastSummary != "" {
			data["lastSummary"] = chat.LastSummary
		}
		if err := appendWorkspaceStoreEvent(store, events.StreamChats, events.TypeChatRuntimeSet, timestamp, data); err != nil {
			return workspaceTmuxMigrationResult{}, err
		}
	}

	shouldCompact := true
	if payload.Compact != nil {
		shouldCompact = *payload.Compact
	}
	if shouldCompact {
		migratedState, err := store.LoadState()
		if err != nil {
			return workspaceTmuxMigrationResult{}, err
		}
		if err := store.Compact(migratedState); err != nil {
			return workspaceTmuxMigrationResult{}, err
		}
		result.Compacted = true
	}
	return result, nil
}

func workspaceTmuxMigrationTargets(storeState readmodels.StoreState, requestedChatIDs []string) ([]readmodels.ChatRecord, int) {
	requested := map[string]bool{}
	if len(requestedChatIDs) > 0 {
		for _, chatID := range requestedChatIDs {
			chatID = strings.TrimSpace(chatID)
			if chatID != "" {
				requested[chatID] = false
			}
		}
	}

	targets := make([]readmodels.ChatRecord, 0)
	skipped := 0
	for _, chat := range storeState.ChatsByID {
		if len(requested) > 0 {
			if _, ok := requested[chat.ID]; !ok {
				continue
			}
			requested[chat.ID] = true
		}
		if chat.DeletedAt != 0 || !chat.HasMessages || strings.TrimSpace(chat.TmuxSession) != "" {
			skipped += 1
			continue
		}
		targets = append(targets, chat)
	}
	for _, seen := range requested {
		if !seen {
			skipped += 1
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].UpdatedAt > targets[j].UpdatedAt
	})
	return targets, skipped
}

func workspaceTmuxMigrationSummary(store interface {
	ReplayTranscriptEntriesForChat(string, int) ([]readmodels.TranscriptEntry, error)
}, chatID string) string {
	entries, err := store.ReplayTranscriptEntriesForChat(chatID, 80)
	if err != nil {
		return ""
	}
	fallback := ""
	for index := len(entries) - 1; index >= 0; index -= 1 {
		text, isUser := workspaceTranscriptEntryPreview(entries[index])
		if summary := workspacePromptPreview(text); summary != "" {
			if !isUser {
				return summary
			}
			if fallback == "" {
				fallback = summary
			}
		}
	}
	return fallback
}
