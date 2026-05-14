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

	// Inject into AfterAgent
	afterAgentArr, _ := hooks["AfterAgent"].([]interface{})
	
	alreadyInstalled := false
	for _, hookObj := range afterAgentArr {
		if hm, ok := hookObj.(map[string]interface{}); ok {
			if innerHooks, ok := hm["hooks"].([]interface{}); ok {
				for _, ih := range innerHooks {
					if ihm, ok := ih.(map[string]interface{}); ok {
						if name, ok := ihm["name"].(string); ok && name == "ai-session-viewer-after-agent" {
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

	newHook := map[string]interface{}{
		"matcher": "*",
		"hooks": []interface{}{
			map[string]interface{}{
				"name":    "ai-session-viewer-after-agent",
				"type":    "command",
				"command": "ai-session-viewer hook --agent gemini",
			},
		},
	}

	hooks["AfterAgent"] = append(afterAgentArr, newHook)

	// Inject into SessionEnd
	sessionEndArr, _ := hooks["SessionEnd"].([]interface{})
	newHookEnd := map[string]interface{}{
		"matcher": "*",
		"hooks": []interface{}{
			map[string]interface{}{
				"name":    "ai-session-viewer-session-end",
				"type":    "command",
				"command": "ai-session-viewer hook --agent gemini",
			},
		},
	}
	hooks["SessionEnd"] = append(sessionEndArr, newHookEnd)

	if len(data) > 0 {
		os.WriteFile(configPath+".bak", data, 0644)
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}

func (a *GeminiAdapter) UninstallHook(scope adapters.InstallScope) error {
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

	cleanHookArray := func(hookName string, targetName string) bool {
		arr, ok := hooks[hookName].([]interface{})
		if !ok {
			return false
		}
		var newArr []interface{}
		found := false

		for _, hookObj := range arr {
			keep := true
			if hm, ok := hookObj.(map[string]interface{}); ok {
				if innerHooks, ok := hm["hooks"].([]interface{}); ok {
					for _, ih := range innerHooks {
						if ihm, ok := ih.(map[string]interface{}); ok {
							if name, ok := ihm["name"].(string); ok && name == targetName {
								keep = false
								found = true
							}
						}
					}
				}
			}
			if keep {
				newArr = append(newArr, hookObj)
			}
		}

		if found {
			hooks[hookName] = newArr
		}
		return found
	}

	f1 := cleanHookArray("AfterAgent", "ai-session-viewer-after-agent")
	f2 := cleanHookArray("SessionEnd", "ai-session-viewer-session-end")

	if !f1 && !f2 {
		return fmt.Errorf("hook not found")
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
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
