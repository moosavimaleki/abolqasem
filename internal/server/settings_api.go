package server

import (
	"ai-agent-manager/internal/state"
	"encoding/json"
	"net/http"
)

type settingsPatch struct {
	HookUpdates                     *bool             `json:"hook_updates"`
	HookFollowMode                  *string           `json:"hook_follow_mode"`
	IgnoreHookNavigationWhileTyping *bool             `json:"ignore_hook_navigation_while_typing"`
	FilesystemDiscovery             *bool             `json:"filesystem_discovery"`
	DefaultAgent                    *string           `json:"default_agent"`
	AgentModels                     map[string]string `json:"agent_models"`
}

func handleAPISettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := state.LoadSettings()
		if err != nil {
			http.Error(w, "Failed to load settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, settings)
	case http.MethodPatch, http.MethodPost:
		settings, err := state.LoadSettings()
		if err != nil {
			http.Error(w, "Failed to load settings", http.StatusInternalServerError)
			return
		}

		var patch settingsPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if patch.HookUpdates != nil {
			settings.HookUpdates = *patch.HookUpdates
		}
		if patch.HookFollowMode != nil {
			settings.HookFollowMode = *patch.HookFollowMode
		}
		if patch.IgnoreHookNavigationWhileTyping != nil {
			settings.IgnoreHookNavigationWhileTyping = *patch.IgnoreHookNavigationWhileTyping
		}
		if patch.FilesystemDiscovery != nil {
			settings.FilesystemDiscovery = *patch.FilesystemDiscovery
		}
		if patch.DefaultAgent != nil {
			settings.DefaultAgent = *patch.DefaultAgent
		}
		if patch.AgentModels != nil {
			if settings.AgentModels == nil {
				settings.AgentModels = map[string]string{}
			}
			for agent, model := range patch.AgentModels {
				settings.AgentModels[agent] = model
			}
		}
		if err := state.SaveSettings(settings); err != nil {
			http.Error(w, "Failed to save settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, state.NormalizeSettings(settings))
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPIReloadSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	report, err := runDiscovery()
	if err != nil {
		http.Error(w, "Failed to reload sessions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status": "ok",
		"report": report,
	})
}

func handleAPIRestartServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := scheduleServerRestart(); err != nil {
		http.Error(w, "Failed to schedule restart", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "restarting"})
}

func handleAPIHooksStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"items": workspaceHookStatuses(),
	})
}
