package server

import (
	"ai-agent-manager/internal/parser"
	"ai-agent-manager/internal/state"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	searchDefaultLimit    = 50
	searchMaxLimit        = 100
	searchPerSessionLimit = 3
	searchMaxSnippetRunes = 220
	searchResultLookahead = 1
)

type sessionSearchResult struct {
	Key                  string               `json:"key"`
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
		"app":                "ai-agent-manager",
		"pid":                os.Getpid(),
		"latest_session_key": appState.LatestSessionKey,
		"latest_session_id":  appState.LatestSessionID,
		"latest_updated_at":  latestUpdatedAt,
		"session_count":      len(appState.Sessions),
		"server_time":        time.Now().Format(time.RFC3339),
	})
}

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := clampInt(parsePositiveInt(r.URL.Query().Get("limit"), searchDefaultLimit), 1, searchMaxLimit)
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

	sessions := make([]state.SessionMeta, 0, len(appState.Sessions))
	for _, meta := range appState.Sessions {
		sessions = append(sessions, meta)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	items := []sessionSearchResult{}
	matchedSessions := 0
	scannedSessions := 0
	stopAfter := offset + limit + searchResultLookahead
	for _, meta := range sessions {
		scannedSessions++
		if meta.MetadataOnly || strings.TrimSpace(meta.TranscriptPath) == "" {
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

		if matchedSessions >= offset && len(items) < limit {
			enriched := enrichSessionMeta(meta)
			items = append(items, newSessionSearchResult(enriched, result.Matches))
		}
		matchedSessions++
		if matchedSessions >= stopAfter {
			break
		}
	}

	nextOffset := 0
	if matchedSessions > offset+len(items) {
		nextOffset = offset + len(items)
	}
	writeJSON(w, map[string]any{
		"items":            items,
		"next_offset":      nextOffset,
		"total":            matchedSessions,
		"query":            query,
		"scanned_sessions": scannedSessions,
	})
}

func newSessionSearchResult(meta state.SessionMeta, matches []parser.SearchMatch) sessionSearchResult {
	return sessionSearchResult{
		Key:                  meta.Key,
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

	eventKey := meta.Key + ":" + meta.UpdatedAt.Format(time.RFC3339Nano)
	EventBroker.Broadcast(SSEEvent{
		Source:      "hook",
		EventKey:    eventKey,
		SessionKey:  meta.Key,
		SessionID:   meta.SessionID,
		SessionName: state.ResolveSessionName(meta),
		ProjectName: meta.ProjectName,
		UpdatedAt:   meta.UpdatedAt.Format(time.RFC3339),
	})

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
