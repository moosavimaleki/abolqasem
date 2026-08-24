package server

import (
	"abolqasem/internal/appinfo"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"abolqasem/internal/parser"
	"abolqasem/internal/state"
	"abolqasem/internal/workspace/legacyimport"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

const (
	searchDefaultLimit    = 50
	searchMaxLimit        = 100
	searchPerSessionLimit = 3
	searchMaxSnippetRunes = 220
)

type sessionSearchResult struct {
	Key                  string               `json:"key"`
	ChatID               string               `json:"chat_id,omitempty"`
	Agent                string               `json:"agent"`
	SessionID            string               `json:"session_id"`
	SessionName          string               `json:"session_name"`
	TranscriptPath       string               `json:"transcript_path"`
	Cwd                  string               `json:"cwd"`
	ProjectName          string               `json:"project_name"`
	Model                string               `json:"model,omitempty"`
	UpdatedAt            time.Time            `json:"updated_at"`
	FirstPreview         string               `json:"first_preview,omitempty"`
	LastPreview          string               `json:"last_preview"`
	MessageCountEstimate int                  `json:"message_count_estimate"`
	MetadataOnly         bool                 `json:"metadata_only"`
	InvalidReason        string               `json:"invalid_reason,omitempty"`
	SearchMatches        []parser.SearchMatch `json:"search_matches"`
	SearchMatchCount     int                  `json:"search_match_count"`
}

type chatSearchMatch struct {
	MessageID string     `json:"message_id,omitempty"`
	EntryID   string     `json:"entry_id,omitempty"`
	Role      string     `json:"role"`
	Kind      string     `json:"kind,omitempty"`
	Index     int        `json:"index"`
	Snippet   string     `json:"snippet"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

func handleAPIState(w http.ResponseWriter, r *http.Request) {
	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var latestUpdatedAt string
	if meta, ok := appState.Sessions[appState.LatestSessionKey]; ok {
		latestUpdatedAt = meta.UpdatedAt.Format(time.RFC3339)
	}

	writeJSON(w, map[string]any{
		"app":                appinfo.Name,
		"pid":                os.Getpid(),
		"latest_session_key": appState.LatestSessionKey,
		"latest_session_id":  appState.LatestSessionID,
		"latest_updated_at":  latestUpdatedAt,
		"session_count":      len(appState.Sessions),
		"server_time":        time.Now().Format(time.RFC3339),
	})
}

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := clampInt(parsePositiveInt(r.URL.Query().Get("limit"), searchDefaultLimit), 1, searchMaxLimit)
	if chatID != "" {
		handleAPIChatSearch(w, r, chatID, query, limit)
		return
	}

	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)
	if query == "" {
		writeJSON(w, map[string]any{
			"items":       []sessionSearchResult{},
			"next_offset": 0,
			"total":       0,
			"query":       query,
		})
		return
	}

	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if items, nextOffset, total, scannedSessions, err := searchSessionsWithIndex(r.Context(), appState, query, offset, limit); err == nil {
		writeJSON(w, map[string]any{
			"items":            items,
			"next_offset":      nextOffset,
			"total":            total,
			"query":            query,
			"scanned_sessions": scannedSessions,
			"search_backend":   "bluge",
		})
		return
	} else if r.Context().Err() != nil {
		return
	}

	sessions := make([]state.SessionMeta, 0, len(appState.Sessions))
	for _, meta := range appState.Sessions {
		sessions = append(sessions, meta)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	storedWorkspaceChats := workspaceStoredChatSet()

	candidates := []sessionSearchResult{}
	scannedSessions := 0
	for _, meta := range sessions {
		scannedSessions++
		if meta.MetadataOnly || strings.TrimSpace(meta.TranscriptPath) == "" {
			continue
		}
		if _, ok := storedWorkspaceChats[legacyimport.ImportedChatID(meta)]; ok {
			continue
		}

		result, err := parser.SearchMessages(meta.Agent, meta.SessionID, meta.TranscriptPath, parser.SearchOptions{
			Query:        query,
			Limit:        searchPerSessionLimit,
			SnippetRunes: searchMaxSnippetRunes,
		})
		if err != nil {
			if errors.Is(err, parser.ErrTranscriptUnavailable) {
				continue
			}
			continue
		}
		if len(result.Matches) == 0 {
			continue
		}

		enriched := enrichSessionMeta(meta)
		candidates = append(candidates, newSessionSearchResult(enriched, result.Matches))
	}

	workspaceItems, workspaceScanned := searchWorkspaceSessions(query, searchPerSessionLimit)
	candidates = append(candidates, workspaceItems...)
	scannedSessions += workspaceScanned

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
	})

	total := len(candidates)
	items := []sessionSearchResult{}
	nextOffset := 0
	if offset < len(candidates) {
		end := offset + limit
		if end > len(candidates) {
			end = len(candidates)
		}
		items = candidates[offset:end]
		if end < len(candidates) {
			nextOffset = end
		}
	}
	writeJSON(w, map[string]any{
		"items":            items,
		"next_offset":      nextOffset,
		"total":            total,
		"query":            query,
		"scanned_sessions": scannedSessions,
	})
}

func handleAPIChatSearch(w http.ResponseWriter, r *http.Request, chatID string, query string, limit int) {
	if query == "" {
		writeJSON(w, map[string]any{
			"chat_id": chatID,
			"matches": []chatSearchMatch{},
			"query":   query,
			"total":   0,
		})
		return
	}

	var (
		matches []chatSearchMatch
		err     error
	)
	if meta, ok := workspaceLegacySessionByChatID(chatID); ok && !workspaceStoredChatExists(chatID) {
		matches, err = searchLegacyChatTranscript(meta, query, limit)
	} else {
		matches, err = searchWorkspaceChatTranscript(chatID, query, limit)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") || errors.Is(err, parser.ErrTranscriptUnavailable) {
			http.Error(w, "chat not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"chat_id": chatID,
		"matches": matches,
		"query":   query,
		"total":   len(matches),
	})
}

func searchLegacyChatTranscript(meta state.SessionMeta, query string, limit int) ([]chatSearchMatch, error) {
	result, err := parser.SearchMessages(meta.Agent, meta.SessionID, meta.TranscriptPath, parser.SearchOptions{
		Query:        query,
		Limit:        limit,
		SnippetRunes: searchMaxSnippetRunes,
	})
	if err != nil {
		return nil, err
	}

	matches := make([]chatSearchMatch, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, chatSearchMatch{
			MessageID: match.MessageID,
			Role:      match.Role,
			Index:     match.Index,
			Snippet:   match.Snippet,
			CreatedAt: match.CreatedAt,
		})
	}
	return matches, nil
}

func searchWorkspaceChatTranscript(chatID string, query string, limit int) ([]chatSearchMatch, error) {
	chat, _, err := workspaceChatProjectRequired(chatID)
	if err != nil {
		return nil, err
	}
	if meta, ok, err := workspaceNativeTranscriptMetaForChat(chatID); err != nil {
		return nil, err
	} else if ok {
		return searchNativeChatTranscript(meta, query, limit)
	}
	if workspaceChatHasTmuxRuntime(chat) {
		return []chatSearchMatch{}, nil
	}
	entries, err := workspaceChatMessages(chatID)
	if err != nil {
		return nil, err
	}
	return searchWorkspaceEntries(entries, query, limit), nil
}

func searchNativeChatTranscript(meta state.SessionMeta, query string, limit int) ([]chatSearchMatch, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	matches := []chatSearchMatch{}
	err := parser.StreamSearchableMessages(meta.Agent, meta.SessionID, meta.TranscriptPath, func(message parser.SearchableMessage) bool {
		text := strings.TrimSpace(message.Text)
		if text == "" || !strings.Contains(strings.ToLower(text), queryLower) {
			return true
		}
		matches = append(matches, chatSearchMatch{
			MessageID: message.ID,
			Role:      message.Role,
			Kind:      message.Kind,
			Index:     message.Index,
			Snippet:   serverSearchSnippet(text, queryLower, searchMaxSnippetRunes),
			CreatedAt: message.CreatedAt,
		})
		return len(matches) < limit
	})
	return matches, err
}

func searchWorkspaceEntries(entries []readmodels.TranscriptEntry, query string, limit int) []chatSearchMatch {
	queryLower := strings.ToLower(query)
	matches := make([]chatSearchMatch, 0, min(limit, len(entries)))
	toolCallIDByToolID := map[string]string{}
	for index, entry := range entries {
		if transcript.Kind(entry) == transcript.KindToolCall {
			if toolID := workspaceEntryToolID(entry); toolID != "" {
				toolCallIDByToolID[toolID] = workspaceEntryString(entry, "_id")
			}
		}

		kind, role, text := workspaceEntrySearchText(entry)
		if strings.TrimSpace(text) == "" || !strings.Contains(strings.ToLower(text), queryLower) {
			continue
		}

		entryID := workspaceEntryString(entry, "_id")
		messageID := firstNonEmptyString(
			workspaceSearchDisplayMessageID(entry, toolCallIDByToolID),
			workspaceEntryString(entry, "messageId"),
			workspaceEntryString(entry, "id"),
			entryID,
		)
		matches = append(matches, chatSearchMatch{
			MessageID: messageID,
			EntryID:   entryID,
			Role:      role,
			Kind:      kind,
			Index:     index + 1,
			Snippet:   serverSearchSnippet(text, queryLower, searchMaxSnippetRunes),
			CreatedAt: workspaceEntryCreatedAt(entry),
		})
		if len(matches) >= limit {
			break
		}
	}
	return matches
}

func workspaceSearchDisplayMessageID(entry readmodels.TranscriptEntry, toolCallIDByToolID map[string]string) string {
	if transcript.Kind(entry) != transcript.KindToolResult {
		return ""
	}
	toolID := workspaceEntryToolID(entry)
	if toolID == "" {
		return ""
	}
	return toolCallIDByToolID[toolID]
}

func workspaceStoredChatSet() map[string]struct{} {
	state, err := workspaceStore().LoadStateLight()
	if err != nil {
		return map[string]struct{}{}
	}
	chats := make(map[string]struct{}, len(state.ChatsByID))
	for chatID, chat := range state.ChatsByID {
		if chat.DeletedAt != 0 {
			continue
		}
		chats[chatID] = struct{}{}
	}
	return chats
}

func searchWorkspaceSessions(query string, perChatLimit int) ([]sessionSearchResult, int) {
	store := workspaceStore()
	storeState, err := store.LoadStateLight()
	if err != nil {
		return nil, 0
	}

	items := []sessionSearchResult{}
	scanned := 0
	for _, chat := range storeState.ChatsByID {
		if chat.DeletedAt != 0 {
			continue
		}
		project, ok := storeState.ProjectsByID[chat.ProjectID]
		if !ok || project.DeletedAt != 0 {
			continue
		}
		scanned++
		var matches []chatSearchMatch
		if meta, ok := workspaceNativeTranscriptMetaForChatRecord(chat, project); ok {
			matches, _ = searchNativeChatTranscript(meta, query, perChatLimit)
		} else if workspaceChatHasTmuxRuntime(chat) {
			matches = []chatSearchMatch{}
		} else if entries, err := store.ReplayTranscriptEntriesForChat(chat.ID, 0); err == nil {
			matches = searchWorkspaceEntries(entries, query, perChatLimit)
		}
		if len(matches) == 0 {
			continue
		}
		items = append(items, newWorkspaceSearchResult(chat, project, matches))
	}
	return items, scanned
}

func newWorkspaceSearchResult(chat readmodels.ChatRecord, project readmodels.ProjectRecord, matches []chatSearchMatch) sessionSearchResult {
	agent := ""
	if chat.Provider != nil {
		agent = *chat.Provider
	}
	parserMatches := make([]parser.SearchMatch, 0, len(matches))
	for _, match := range matches {
		parserMatches = append(parserMatches, parser.SearchMatch{
			MessageID: match.MessageID,
			Role:      match.Role,
			Index:     match.Index,
			Snippet:   match.Snippet,
			CreatedAt: match.CreatedAt,
		})
	}
	updatedAtMillis := max(chat.UpdatedAt, chat.LastMessageAt, chat.CreatedAt)
	updatedAt := time.UnixMilli(updatedAtMillis)
	return sessionSearchResult{
		Key:                  "workspace:" + chat.ID,
		ChatID:               chat.ID,
		Agent:                agent,
		SessionID:            chat.ID,
		SessionName:          chat.Title,
		Cwd:                  project.LocalPath,
		ProjectName:          project.Title,
		UpdatedAt:            updatedAt,
		LastPreview:          matches[0].Snippet,
		MessageCountEstimate: 0,
		MetadataOnly:         false,
		SearchMatches:        parserMatches,
		SearchMatchCount:     len(parserMatches),
	}
}

func workspaceEntrySearchText(entry readmodels.TranscriptEntry) (kind string, role string, text string) {
	kind = workspaceEntryString(entry, "kind")
	switch kind {
	case transcript.KindUserPrompt:
		return kind, "user", workspaceEntryString(entry, "content")
	case transcript.KindAssistantText:
		return kind, "assistant", workspaceEntryString(entry, "text")
	case transcript.KindStatus:
		return kind, "status", workspaceEntryString(entry, "status")
	case transcript.KindResult:
		return kind, "result", workspaceEntryString(entry, "result")
	case transcript.KindCompactSummary:
		return kind, "assistant", workspaceEntryString(entry, "summary")
	case transcript.KindUnknown:
		return kind, "unknown", workspaceEntryString(entry, "json")
	case transcript.KindToolCall:
		return kind, "tool", workspaceAnySearchText(entry["tool"], entry["input"])
	case transcript.KindToolResult:
		return kind, "tool", workspaceAnySearchText(entry["content"], entry["output"])
	default:
		return kind, kind, workspaceAnySearchText(entry["content"], entry["text"], entry["summary"], entry["json"])
	}
}

func workspaceEntryString(entry readmodels.TranscriptEntry, key string) string {
	if value, ok := entry[key]; ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return ""
}

func workspaceEntryToolID(entry readmodels.TranscriptEntry) string {
	if toolID := workspaceEntryString(entry, "toolId"); toolID != "" {
		return toolID
	}
	tool, ok := entry["tool"].(map[string]any)
	if !ok {
		return ""
	}
	if toolID, ok := tool["toolId"].(string); ok {
		return toolID
	}
	return ""
}

func workspaceAnySearchText(values ...any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(typed) != "" {
				parts = append(parts, typed)
			}
		default:
			encoded, err := json.Marshal(typed)
			if err == nil && len(encoded) > 0 {
				parts = append(parts, string(encoded))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func workspaceEntryCreatedAt(entry readmodels.TranscriptEntry) *time.Time {
	value, ok := entry["createdAt"]
	if !ok {
		return nil
	}

	var millis int64
	switch typed := value.(type) {
	case int64:
		millis = typed
	case int:
		millis = int64(typed)
	case int32:
		millis = int64(typed)
	case float64:
		millis = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return nil
		}
		millis = parsed
	default:
		return nil
	}
	if millis <= 0 {
		return nil
	}
	createdAt := time.UnixMilli(millis)
	return &createdAt
}

func serverSearchSnippet(text, queryLower string, maxRunes int) string {
	textRunes := []rune(strings.TrimSpace(text))
	if len(textRunes) <= maxRunes {
		return string(textRunes)
	}

	index := indexRunes([]rune(strings.ToLower(string(textRunes))), []rune(queryLower))
	if index < 0 {
		index = 0
	}
	start := index - (maxRunes / 3)
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(textRunes) {
		end = len(textRunes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}

	prefix := ""
	if start > 0 {
		prefix = "..."
	}
	suffix := ""
	if end < len(textRunes) {
		suffix = "..."
	}
	return prefix + string(textRunes[start:end]) + suffix
}

func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		matched := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func newSessionSearchResult(meta state.SessionMeta, matches []parser.SearchMatch) sessionSearchResult {
	return sessionSearchResult{
		Key:                  meta.Key,
		ChatID:               legacyimport.ImportedChatID(meta),
		Agent:                meta.Agent,
		SessionID:            meta.SessionID,
		SessionName:          state.ResolveSessionName(meta),
		TranscriptPath:       meta.TranscriptPath,
		Cwd:                  meta.Cwd,
		ProjectName:          meta.ProjectName,
		Model:                meta.Model,
		UpdatedAt:            meta.UpdatedAt,
		FirstPreview:         meta.FirstPreview,
		LastPreview:          meta.LastPreview,
		MessageCountEstimate: meta.MessageCountEstimate,
		MetadataOnly:         meta.MetadataOnly,
		InvalidReason:        meta.InvalidReason,
		SearchMatches:        matches,
		SearchMatchCount:     len(matches),
	}
}

func handleAPISessions(w http.ResponseWriter, r *http.Request) {
	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	projectFilter := strings.TrimSpace(r.URL.Query().Get("project"))
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	if limit <= 0 {
		limit = 50
	}
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)

	items := make([]state.SessionMeta, 0, len(appState.Sessions))
	for _, meta := range appState.Sessions {
		if projectFilter != "" && !strings.EqualFold(strings.TrimSpace(meta.ProjectName), projectFilter) {
			continue
		}
		items = append(items, meta)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	nextOffset := 0
	total := len(items)
	if offset < len(items) {
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		if end < len(items) {
			nextOffset = end
		}
		items = items[offset:end]
	} else {
		items = []state.SessionMeta{}
	}
	for i := range items {
		items[i] = enrichSessionMeta(items[i])
	}

	writeJSON(w, map[string]any{
		"items":       items,
		"next_offset": nextOffset,
		"total":       total,
	})
}

func handleAPIHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event state.HookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	event = state.NormalizeAndValidateEvent(event)

	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, "Failed to load state", http.StatusInternalServerError)
		return
	}
	meta := state.UpsertSession(appState, event)
	if err := state.SaveState(appState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	broadcastChatID := ""
	if isPromptSubmitHookEvent(event) {
		if record, err := workspaceRecordHookPromptCheckpoint(meta, event); err == nil && record.ProjectID != "" {
			workspaceConnections.broadcastProjectGit(record.ProjectID)
			broadcastChatID = record.ChatID
		}
	} else {
		_ = workspaceSyncMaterializedLegacyChat(meta)
	}
	if broadcastChatID == "" {
		broadcastChatID = workspaceLegacyBroadcastChatID(meta)
	}

	eventKey := meta.Key + ":" + normalizedHookEventName(event.HookEventName) + ":" + meta.UpdatedAt.Format(time.RFC3339Nano)
	EventBroker.Broadcast(SSEEvent{
		Source:           "hook",
		EventKey:         eventKey,
		SessionKey:       meta.Key,
		SessionID:        meta.SessionID,
		ChatID:           broadcastChatID,
		SessionName:      state.ResolveSessionName(meta),
		ProjectName:      meta.ProjectName,
		HookEventName:    event.HookEventName,
		ResponseComplete: isResponseCompleteHookEvent(event),
		UpdatedAt:        meta.UpdatedAt.Format(time.RFC3339),
	})
	workspaceConnections.broadcast(broadcastChatID)

	writeJSON(w, map[string]any{
		"status":      "ok",
		"session_key": meta.Key,
	})
}

func handleAPISessionMessages(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "session" {
		http.NotFound(w, r)
		return
	}

	sessionKey := parts[2]
	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionMeta, exists := appState.Sessions[sessionKey]
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	if len(parts) == 3 {
		handleAPISessionUpdate(w, r, appState, sessionMeta)
		return
	}
	if len(parts) != 4 || parts[3] != "messages" {
		http.NotFound(w, r)
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), 30)
	result, err := parser.ParseMessages(sessionMeta.Agent, sessionMeta.SessionID, sessionMeta.TranscriptPath, parser.ParseOptions{
		Limit:  limit,
		Before: r.URL.Query().Get("before"),
		After:  r.URL.Query().Get("after"),
	})
	if err != nil {
		if errors.Is(err, parser.ErrTranscriptUnavailable) || sessionMeta.MetadataOnly {
			writeJSON(w, map[string]any{
				"session_id": sessionMeta.SessionID,
				"items":      []parser.Message{},
				"status":     "metadata_only",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

func handleAPISessionUpdate(w http.ResponseWriter, r *http.Request, appState *state.AppState, sessionMeta state.SessionMeta) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		SessionName string `json:"session_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	sessionMeta.SessionName = state.SanitizeSessionName(payload.SessionName)
	appState.Sessions[sessionMeta.Key] = sessionMeta
	if err := state.SaveState(appState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}
	writeJSON(w, enrichSessionMeta(sessionMeta))
}

func enrichSessionMeta(meta state.SessionMeta) state.SessionMeta {
	if meta.MetadataOnly {
		meta.SessionName = state.ResolveSessionName(meta)
		return meta
	}
	if meta.FirstPreview != "" && meta.LastPreview != "" && meta.MessageCountEstimate > 0 {
		meta.SessionName = state.ResolveSessionName(meta)
		return meta
	}

	summary, err := parser.GetSessionSummary(meta.Agent, meta.SessionID, meta.TranscriptPath)
	if err != nil {
		if errors.Is(err, parser.ErrTranscriptUnavailable) {
			meta.MetadataOnly = true
			if meta.InvalidReason == "" {
				meta.InvalidReason = "transcript is not readable"
			}
		}
		meta.SessionName = state.ResolveSessionName(meta)
		return meta
	}
	meta.FirstPreview = summary.FirstPreview
	meta.LastPreview = summary.LastPreview
	meta.MessageCountEstimate = summary.MessageCountEstimate
	meta.SessionName = state.ResolveSessionName(meta)
	return meta
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
