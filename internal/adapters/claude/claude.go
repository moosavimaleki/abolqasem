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
	configPath := a.getConfigPath(scope)
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var settings map[string]interface{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse %s: %v", configPath, err)
		}
	} else {
		settings = make(map[string]interface{})
	}

	// Check if already installed
	if data != nil {
		strData := string(data)
		if filepath.Base(os.Args[0]) == "ai-session-viewer" {
			// Basic check to see if command is already there
			if len(strData) > 0 {
				// Very loose check, if it causes false positives we can tighten it
			}
		}
	}

	// Helper for navigating map
	ensureMap := func(m map[string]interface{}, key string) map[string]interface{} {
		if val, ok := m[key]; ok {
			if vm, ok2 := val.(map[string]interface{}); ok2 {
				return vm
			}
		}
		newMap := make(map[string]interface{})
		m[key] = newMap
		return newMap
	}

	hooks := ensureMap(settings, "hooks")

	// Claude's Stop hook is an array of objects
	var stopArr []interface{}
	if val, ok := hooks["Stop"]; ok {
		if arr, ok2 := val.([]interface{}); ok2 {
			stopArr = arr
		}
	}

	// Check if already in the array
	alreadyInstalled := false
	for _, hookObj := range stopArr {
		if hm, ok := hookObj.(map[string]interface{}); ok {
			if innerHooks, ok := hm["hooks"].([]interface{}); ok {
				for _, ih := range innerHooks {
					if ihm, ok := ih.(map[string]interface{}); ok {
						if cmd, ok := ihm["command"].(string); ok && cmd == "ai-session-viewer" {
							alreadyInstalled = true
						}
					}
				}
			}
		}
	}

	if alreadyInstalled {
		return fmt.Errorf("hook already installed")
	}

	// Create new hook block
	newHook := map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": "ai-session-viewer",
				"args":    []string{"hook", "--agent", "claude"},
				"timeout": 3,
			},
		},
	}

	stopArr = append(stopArr, newHook)
	hooks["Stop"] = stopArr

	// Write backup
	if len(data) > 0 {
		os.WriteFile(configPath+".bak", data, 0644)
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}

func (a *ClaudeAdapter) UninstallHook(scope adapters.InstallScope) error {
	configPath := a.getConfigPath(scope)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse json: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("hook not found")
	}

	stopArr, ok := hooks["Stop"].([]interface{})
	if !ok {
		return fmt.Errorf("hook not found")
	}

	var newStopArr []interface{}
	found := false

	for _, hookObj := range stopArr {
		keep := true
		if hm, ok := hookObj.(map[string]interface{}); ok {
			if innerHooks, ok := hm["hooks"].([]interface{}); ok {
				for _, ih := range innerHooks {
					if ihm, ok := ih.(map[string]interface{}); ok {
						if cmd, ok := ihm["command"].(string); ok && cmd == "ai-session-viewer" {
							keep = false
							found = true
						}
					}
				}
			}
		}
		if keep {
			newStopArr = append(newStopArr, hookObj)
		}
	}

	if !found {
		return fmt.Errorf("hook not found")
	}

	hooks["Stop"] = newStopArr

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
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

	// Fallback for session ID
	if event.SessionID == "" {
		if event.TranscriptPath != "" {
			event.SessionID = filepath.Base(filepath.Dir(event.TranscriptPath))
		} else if event.Cwd != "" {
			event.SessionID = filepath.Base(event.Cwd) + "-session"
		} else {
			event.SessionID = "unknown-session"
		}
	}

	return event, nil
}
