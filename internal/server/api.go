package server

import (
	"ai-session-viewer/internal/parser"
	"ai-session-viewer/internal/state"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
		"latest_session_key": appState.LatestSessionKey,
		"latest_session_id":  appState.LatestSessionID,
		"latest_updated_at":  latestUpdatedAt,
		"session_count":      len(appState.Sessions),
		"server_time":        time.Now().Format(time.RFC3339),
	})
}

func handleAPISessions(w http.ResponseWriter, r *http.Request) {
	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	projectFilter := strings.TrimSpace(r.URL.Query().Get("project"))
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)

	items := make([]state.SessionMeta, 0, len(appState.Sessions))
	for _, meta := range appState.Sessions {
		if projectFilter != "" && !strings.Contains(strings.ToLower(meta.ProjectName), strings.ToLower(projectFilter)) {
			continue
		}
		items = append(items, enrichSessionMeta(meta))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	nextOffset := 0
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

	writeJSON(w, map[string]any{
		"items":       items,
		"next_offset": nextOffset,
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
		EventKey:    eventKey,
		SessionKey:  meta.Key,
		SessionID:   meta.SessionID,
		ProjectName: meta.ProjectName,
		UpdatedAt:   meta.UpdatedAt.Format(time.RFC3339),
	})

	writeJSON(w, map[string]any{
		"status":      "ok",
		"session_key": meta.Key,
	})
}

func handleAPISessionMessages(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 || parts[4] != "messages" {
		http.NotFound(w, r)
		return
	}

	sessionKey := parts[3]
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

func enrichSessionMeta(meta state.SessionMeta) state.SessionMeta {
	if meta.MetadataOnly {
		return meta
	}
	if meta.LastPreview != "" && meta.MessageCountEstimate > 0 {
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
		return meta
	}
	meta.LastPreview = summary.LastPreview
	meta.MessageCountEstimate = summary.MessageCountEstimate
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

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
