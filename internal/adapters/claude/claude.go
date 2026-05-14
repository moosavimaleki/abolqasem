package claude

import (
	"ai-session-viewer/internal/adapters"
	"ai-session-viewer/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ClaudeAdapter struct{}

func New() adapters.AgentAdapter {
	return &ClaudeAdapter{}
}

func (a *ClaudeAdapter) Name() string {
	return "claude"
}

func (a *ClaudeAdapter) getConfigPath(scope adapters.InstallScope) string {
	if scope == adapters.ScopeProject {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".claude", "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func (a *ClaudeAdapter) InstallHook(scope adapters.InstallScope) error {
	// For MVP, just output instructions or do basic JSON injection.
	// Implementing robust JSON merge here.
	return fmt.Errorf("automatic install for claude is currently under development. Please edit %s manually.", a.getConfigPath(scope))
}

func (a *ClaudeAdapter) UninstallHook(scope adapters.InstallScope) error {
	return fmt.Errorf("automatic uninstall for claude is currently under development.")
}

func (a *ClaudeAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(input, &raw); err != nil {
		return state.HookEvent{}, err
	}

	event := state.HookEvent{
		Agent: "claude",
	}

	if sID, ok := raw["session_id"].(string); ok {
		event.SessionID = sID
	}
	if tp, ok := raw["transcript_path"].(string); ok {
		// Expand path if necessary (e.g. starting with ~)
		if len(tp) > 2 && tp[:2] == "~/" {
			home, _ := os.UserHomeDir()
			tp = filepath.Join(home, tp[2:])
		}
		event.TranscriptPath = tp
	}
	if c, ok := raw["cwd"].(string); ok {
		event.Cwd = c
	}

	return event, nil
}
