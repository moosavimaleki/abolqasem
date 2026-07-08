package gemini

import (
	"abolqasem/internal/adapters"
	"abolqasem/internal/appinfo"
	"abolqasem/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	afterAgentHookName       = appinfo.Name + "-gemini-after-agent"
	sessionEndHookName       = appinfo.Name + "-gemini-session-end"
	legacyAfterAgentHookName = appinfo.LegacyName + "-gemini-after-agent"
	legacySessionEndHookName = appinfo.LegacyName + "-gemini-session-end"
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
	settings, raw, err := loadSettings(configPath)
	if err != nil {
		return err
	}

	hooks := ensureMap(settings, "hooks")
	command, err := adapters.ShellCommand("gemini")
	if err != nil {
		return err
	}

	hooks["AfterAgent"], _ = ensureNamedHook(ensureBlocks(hooks["AfterAgent"]), afterAgentHookName, command)
	hooks["SessionEnd"], _ = ensureNamedHook(ensureBlocks(hooks["SessionEnd"]), sessionEndHookName, command)
	return saveSettings(configPath, raw, settings)
}

func (a *GeminiAdapter) UninstallHook(scope adapters.InstallScope) error {
	configPath := a.getConfigPath(scope)
	settings, _, err := loadSettings(configPath)
	if err != nil {
		return err
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return fmt.Errorf("hook not found")
	}

	afterBlocks, removedAfter := removeNamedHook(ensureBlocks(hooks["AfterAgent"]), afterAgentHookName, "gemini")
	sessionBlocks, removedSession := removeNamedHook(ensureBlocks(hooks["SessionEnd"]), sessionEndHookName, "gemini")
	if !removedAfter && !removedSession {
		return fmt.Errorf("hook not found")
	}
	if len(afterBlocks) == 0 {
		delete(hooks, "AfterAgent")
	} else {
		hooks["AfterAgent"] = afterBlocks
	}
	if len(sessionBlocks) == 0 {
		delete(hooks, "SessionEnd")
	} else {
		hooks["SessionEnd"] = sessionBlocks
	}
	return saveSettings(configPath, nil, settings)
}

func (a *GeminiAdapter) IsHookInstalled(scope adapters.InstallScope) (bool, error) {
	configPath := a.getConfigPath(scope)
	settings, _, err := loadSettings(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	for _, name := range []string{"AfterAgent", "SessionEnd"} {
		for _, block := range ensureBlocks(hooks[name]) {
			for _, entry := range ensureBlocks(block["hooks"]) {
				name := stringValue(entry["name"])
				if name == afterAgentHookName || name == sessionEndHookName || name == legacyAfterAgentHookName || name == legacySessionEndHookName || adapters.IsCommandMatch(stringValue(entry["command"]), "gemini") {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (a *GeminiAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return state.HookEvent{}, err
	}

	hookEventName := stringValue(raw["hook_event_name"])
	if hookEventName == "" {
		hookEventName = stringValue(raw["event_name"])
	}
	if hookEventName == "" {
		if stringValue(raw["response"]) != "" {
			hookEventName = "AfterAgent"
		} else {
			hookEventName = "SessionEnd"
		}
	}
	event := state.HookEvent{
		Agent:          "gemini",
		SessionID:      stringValue(raw["session_id"]),
		HookEventName:  hookEventName,
		TranscriptPath: stringValue(raw["transcript_path"]),
		Cwd:            stringValue(raw["cwd"]),
		LastPreview:    stringValue(raw["response"]),
	}
	return state.NormalizeAndValidateEvent(event), nil
}

func loadSettings(path string) (map[string]any, []byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil, nil
		}
		return nil, nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, data, nil
	}
	settings := map[string]any{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return settings, data, nil
}

func saveSettings(path string, raw []byte, settings map[string]any) error {
	if len(raw) > 0 {
		if err := os.WriteFile(path+".bak", raw, 0o644); err != nil {
			return err
		}
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func ensureMap(target map[string]any, key string) map[string]any {
	if value, ok := target[key].(map[string]any); ok {
		return value
	}
	child := map[string]any{}
	target[key] = child
	return child
}

func ensureBlocks(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if asMap, ok := item.(map[string]any); ok {
				out = append(out, asMap)
			}
		}
		return out
	default:
		return []map[string]any{}
	}
}

func ensureNamedHook(blocks []map[string]any, hookName, command string) ([]map[string]any, bool) {
	for _, block := range blocks {
		entries := ensureBlocks(block["hooks"])
		block["hooks"] = entries
		for _, entry := range entries {
			if stringValue(entry["name"]) == hookName || isLegacyGeminiHookName(stringValue(entry["name"])) || adapters.IsCommandMatch(stringValue(entry["command"]), "gemini") {
				changed := false
				if stringValue(entry["name"]) != hookName {
					entry["name"] = hookName
					changed = true
				}
				if stringValue(entry["type"]) != "command" {
					entry["type"] = "command"
					changed = true
				}
				if stringValue(entry["command"]) != command {
					entry["command"] = command
					changed = true
				}
				return blocks, changed
			}
		}
	}
	return append(blocks, map[string]any{
		"matcher": "*",
		"hooks": []map[string]any{
			{
				"name":    hookName,
				"type":    "command",
				"command": command,
			},
		},
	}), true
}

func removeNamedHook(blocks []map[string]any, hookName, agent string) ([]map[string]any, bool) {
	changed := false
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		entries := ensureBlocks(block["hooks"])
		kept := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			if stringValue(entry["name"]) == hookName || isLegacyGeminiHookName(stringValue(entry["name"])) || adapters.IsCommandMatch(stringValue(entry["command"]), agent) {
				changed = true
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) == 0 {
			continue
		}
		block["hooks"] = kept
		result = append(result, block)
	}
	return result, changed
}

func isLegacyGeminiHookName(name string) bool {
	return name == legacyAfterAgentHookName || name == legacySessionEndHookName
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
