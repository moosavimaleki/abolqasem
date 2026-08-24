package server

import (
	"abolqasem/internal/agent"
	"abolqasem/internal/state"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const agentTurnTimeout = 5 * time.Minute

var workspaceWriteLocks = struct {
	sync.Mutex
	active map[string]bool
}{active: map[string]bool{}}

type agentTurnRequest struct {
	Agent      string `json:"agent"`
	SessionKey string `json:"session_key"`
	Message    string `json:"message"`
	Cwd        string `json:"cwd"`
	New        bool   `json:"new"`
	Model      string `json:"model"`
}

type agentStatusResponse struct {
	Agents []agentRuntimeStatus     `json:"agents"`
	Codex  legacyCodexRuntimeStatus `json:"codex"`
}

type agentRuntimeStatus struct {
	Agent        string            `json:"agent"`
	Label        string            `json:"label"`
	Available    bool              `json:"available"`
	Controllable bool              `json:"controllable"`
	DefaultModel string            `json:"default_model,omitempty"`
	Models       []agent.ModelInfo `json:"models"`
	Error        string            `json:"error,omitempty"`
	Capabilities map[string]bool   `json:"capabilities"`
}

type legacyCodexRuntimeStatus struct {
	Available    bool              `json:"available"`
	DefaultModel string            `json:"default_model,omitempty"`
	Models       []agent.ModelInfo `json:"models"`
	Capabilities map[string]bool   `json:"capabilities"`
}

func handleAPIAgentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	codexStatus := buildCodexRuntimeStatus()
	agents := []agentRuntimeStatus{
		codexStatus,
		buildReadOnlyRuntimeStatus("claude", "کلود", commandAvailable("claude")),
	}
	writeJSON(w, agentStatusResponse{
		Agents: agents,
		Codex: legacyCodexRuntimeStatus{
			Available:    codexStatus.Available,
			DefaultModel: codexStatus.DefaultModel,
			Models:       codexStatus.Models,
			Capabilities: codexStatus.Capabilities,
		},
	})
}

func handleAPIAgentTurn(w http.ResponseWriter, r *http.Request) {
	handleAgentTurn(w, r, "")
}

func handleAPICodexTurn(w http.ResponseWriter, r *http.Request) {
	handleAgentTurn(w, r, agent.CodexAgentName)
}

func handleAgentTurn(w http.ResponseWriter, r *http.Request, forcedAgent string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload agentTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if forcedAgent != "" {
		payload.Agent = forcedAgent
	}
	payload.Agent = normalizeAgentNameServer(payload.Agent)
	payload.Model = strings.TrimSpace(payload.Model)
	payload.Message = strings.TrimSpace(payload.Message)
	if payload.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, "Failed to load state", http.StatusInternalServerError)
		return
	}

	var existing state.SessionMeta
	if payload.SessionKey != "" && !payload.New {
		var ok bool
		existing, ok = appState.Sessions[payload.SessionKey]
		if !ok {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		if payload.Agent == "" {
			payload.Agent = existing.Agent
		}
		if existing.Agent != payload.Agent {
			http.Error(w, "Selected agent does not match this session", http.StatusBadRequest)
			return
		}
	}
	if payload.Agent == "" {
		payload.Agent = agent.CodexAgentName
	}
	if payload.Agent != agent.CodexAgentName {
		http.Error(w, "Web UI control for "+payload.Agent+" is not implemented yet", http.StatusNotImplemented)
		return
	}
	if !agent.CodexAvailable() {
		http.Error(w, "Codex is not available in PATH", http.StatusServiceUnavailable)
		return
	}

	cwd := firstNonEmptyServer(payload.Cwd, existing.Cwd)
	if cwd == "" {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			cwd = home
		}
	}
	if !acquireWorkspaceWriteLock(cwd) {
		http.Error(w, "Another session is changing this project", http.StatusConflict)
		return
	}
	defer releaseWorkspaceWriteLock(cwd)

	ctx, cancel := context.WithTimeout(r.Context(), agentTurnTimeout)
	defer cancel()

	result, err := agent.RunCodexTurn(ctx, agent.CodexRequest{
		ThreadID: existing.SessionID,
		Message:  payload.Message,
		Cwd:      cwd,
		New:      payload.New,
		Model:    payload.Model,
	})
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrThreadActive):
			http.Error(w, "This Codex session is already active in another client. Wait for the current turn to finish, then try again. "+err.Error(), http.StatusConflict)
		case errors.Is(err, context.DeadlineExceeded):
			http.Error(w, "Codex did not finish within 5 minutes. The app-server was stopped for this request. "+err.Error(), http.StatusGatewayTimeout)
		default:
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}

	appState, err = state.LoadState()
	if err != nil {
		http.Error(w, "Failed to reload state", http.StatusInternalServerError)
		return
	}
	meta := state.UpsertSession(appState, state.HookEvent{
		Agent:          agent.CodexAgentName,
		SessionID:      result.ThreadID,
		TranscriptPath: firstNonEmptyServer(result.TranscriptPath, existing.TranscriptPath),
		Cwd:            firstNonEmptyServer(result.Cwd, cwd),
		ProjectName:    agent.ProjectNameFromCwd(firstNonEmptyServer(result.Cwd, cwd)),
		LastPreview:    result.Preview,
		Model:          result.Model,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	})
	if err := state.SaveState(appState); err != nil {
		http.Error(w, "Failed to save state", http.StatusInternalServerError)
		return
	}

	eventKey := meta.Key + ":web:" + meta.UpdatedAt.Format(time.RFC3339Nano)
	EventBroker.Broadcast(SSEEvent{
		Source:      "runtime",
		EventKey:    eventKey,
		SessionKey:  meta.Key,
		SessionID:   meta.SessionID,
		SessionName: state.ResolveSessionName(meta),
		ProjectName: meta.ProjectName,
		UpdatedAt:   meta.UpdatedAt.Format(time.RFC3339),
	})

	writeJSON(w, map[string]any{
		"status":      "ok",
		"thread_id":   result.ThreadID,
		"turn_id":     result.TurnID,
		"agent":       agent.CodexAgentName,
		"model":       result.Model,
		"session":     enrichSessionMeta(meta),
		"session_key": meta.Key,
	})
}

func buildCodexRuntimeStatus() agentRuntimeStatus {
	capabilities := map[string]bool{
		"can_start":              true,
		"can_resume":             true,
		"can_send":               true,
		"supports_live_events":   false,
		"supports_multiple_runs": true,
	}
	status := agentRuntimeStatus{
		Agent:        agent.CodexAgentName,
		Label:        "کدکس",
		Available:    agent.CodexAvailable(),
		Controllable: true,
		Models:       []agent.ModelInfo{},
		Capabilities: capabilities,
	}
	if !status.Available {
		status.Capabilities["can_start"] = false
		status.Capabilities["can_resume"] = false
		status.Capabilities["can_send"] = false
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	models, err := agent.ListCodexModels(ctx)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Models = models
	for _, model := range models {
		if model.IsDefault {
			status.DefaultModel = model.Model
			break
		}
	}
	return status
}

func buildReadOnlyRuntimeStatus(agentName, label string, available bool) agentRuntimeStatus {
	return agentRuntimeStatus{
		Agent:        agentName,
		Label:        label,
		Available:    available,
		Controllable: false,
		Models:       []agent.ModelInfo{},
		Capabilities: map[string]bool{
			"can_start":              false,
			"can_resume":             false,
			"can_send":               false,
			"supports_live_events":   false,
			"supports_multiple_runs": false,
		},
	}
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func normalizeAgentNameServer(agentName string) string {
	switch strings.TrimSpace(strings.ToLower(agentName)) {
	case "codex", "claude":
		return strings.TrimSpace(strings.ToLower(agentName))
	default:
		return ""
	}
}

func acquireWorkspaceWriteLock(cwd string) bool {
	key := strings.TrimSpace(cwd)
	if key == "" {
		key = "unknown"
	}
	workspaceWriteLocks.Lock()
	defer workspaceWriteLocks.Unlock()
	if workspaceWriteLocks.active[key] {
		return false
	}
	workspaceWriteLocks.active[key] = true
	return true
}

func releaseWorkspaceWriteLock(cwd string) {
	key := strings.TrimSpace(cwd)
	if key == "" {
		key = "unknown"
	}
	workspaceWriteLocks.Lock()
	defer workspaceWriteLocks.Unlock()
	delete(workspaceWriteLocks.active, key)
}

func firstNonEmptyServer(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
