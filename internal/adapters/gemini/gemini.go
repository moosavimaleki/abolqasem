package gemini

import (
	"ai-session-viewer/internal/adapters"
	"ai-session-viewer/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type GeminiAdapter struct{}

func New() adapters.AgentAdapter {
	return &GeminiAdapter{}
}

func (a *GeminiAdapter) Name() string {
	return "gemini"
}

func (a *GeminiAdapter) getConfigPath(scope adapters.InstallScope) string {
	if scope == adapters.ScopeProject {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".gemini", "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "settings.json")
}

func (a *GeminiAdapter) InstallHook(scope adapters.InstallScope) error {
	return fmt.Errorf("automatic install for gemini is currently under development. Please edit %s manually.", a.getConfigPath(scope))
}

func (a *GeminiAdapter) UninstallHook(scope adapters.InstallScope) error {
	return fmt.Errorf("automatic uninstall for gemini is currently under development.")
}

func (a *GeminiAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(input, &raw); err != nil {
		return state.HookEvent{}, err
	}

	event := state.HookEvent{
		Agent: "gemini",
	}

	if sID, ok := raw["session_id"].(string); ok {
		event.SessionID = sID
	}
	if tp, ok := raw["transcript_path"].(string); ok {
		event.TranscriptPath = tp
	}
	if c, ok := raw["cwd"].(string); ok {
		event.Cwd = c
	}

	return event, nil
}
