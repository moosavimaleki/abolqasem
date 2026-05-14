package server

import (
    "codex-rtl/internal/parser"
    "codex-rtl/internal/state"
    "encoding/json"
    "net/http"
    "path/filepath"
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
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "latest_session_id": appState.LatestSessionID,
        "session_count": len(appState.Sessions),
        "server_time": time.Now().Format(time.RFC3339),
    })
}

func handleAPISessions(w http.ResponseWriter, r *http.Request) {
    appState, err := state.LoadState()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    var items []state.SessionMeta
    for _, s := range appState.Sessions {
        items = append(items, s)
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "items": items,
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
    
    appState, err := state.LoadState()
    if err != nil {
        http.Error(w, "Failed to load state", http.StatusInternalServerError)
        return
    }
    
    projectName := filepath.Base(event.Cwd)
    if projectName == "." || projectName == "/" {
        projectName = "unknown"
    }

    appState.LatestSessionID = event.SessionID
    appState.Sessions[event.SessionID] = state.SessionMeta{
        SessionID:      event.SessionID,
        TranscriptPath: event.TranscriptPath,
        Cwd:            event.Cwd,
        ProjectName:    projectName,
        UpdatedAt:      time.Now(),
    }
    
    if err := state.SaveState(appState); err != nil {
        http.Error(w, "Failed to save state", http.StatusInternalServerError)
        return
    }
    
    EventBroker.Broadcast(SSEEvent{
		SessionID:   event.SessionID,
		ProjectName: projectName,
		UpdatedAt:   time.Now().Format(time.RFC3339),
	})
    
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"status":"ok"}`))
}

func handleAPISessionMessages(w http.ResponseWriter, r *http.Request) {
    parts := strings.Split(r.URL.Path, "/")
    if len(parts) < 5 || parts[4] != "messages" {
        http.NotFound(w, r)
        return
    }
    
    sessionID := parts[3]
    
    appState, err := state.LoadState()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    sessionMeta, exists := appState.Sessions[sessionID]
    if !exists {
        http.Error(w, "Session not found", http.StatusNotFound)
        return
    }
    
    limit := 30
    if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
        limit = l
    }
    
    beforeIndex := 0
    if b, err := strconv.Atoi(r.URL.Query().Get("before")); err == nil && b > 0 {
        beforeIndex = b
    }
    
    result, err := parser.ParseMessages(sessionID, sessionMeta.TranscriptPath, limit, beforeIndex)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}
