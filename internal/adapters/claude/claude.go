package claude

import (
	"ai-session-viewer/internal/adapters"
	"ai-session-viewer/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const hookName = "ai-session-viewer-claude-stop"

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
	settings, raw, err := loadSettings(configPath)
	if err != nil {
		return err
	}

	hooks := ensureMap(settings, "hooks")
	stopBlocks := ensureBlocks(hooks["Stop"])
	command, args, err := adapters.CommandArgs("claude")
	if err != nil {
		return err
	}

	for _, block := range stopBlocks {
		for _, entry := range ensureBlocks(block["hooks"]) {
			if isEntryMatch(entry) {
				return fmt.Errorf("hook already installed")
			}
		}
	}

	stopBlocks = append(stopBlocks, map[string]any{
		"hooks": []map[string]any{
			{
				"name":    hookName,
				"type":    "command",
				"command": command,
				"args":    args,
				"timeout": 3,
			},
		},
	})
	hooks["Stop"] = stopBlocks
	return saveSettings(configPath, raw, settings)
}

func (a *ClaudeAdapter) UninstallHook(scope adapters.InstallScope) error {
	configPath := a.getConfigPath(scope)
	settings, _, err := loadSettings(configPath)
	if err != nil {
		return err
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return fmt.Errorf("hook not found")
	}

	stopBlocks := ensureBlocks(hooks["Stop"])
	changed := false
	keptBlocks := make([]map[string]any, 0, len(stopBlocks))
	for _, block := range stopBlocks {
		entries := ensureBlocks(block["hooks"])
		keptEntries := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			if isEntryMatch(entry) {
				changed = true
				continue
			}
			keptEntries = append(keptEntries, entry)
		}
		if len(keptEntries) == 0 {
			continue
		}
		block["hooks"] = keptEntries
		keptBlocks = append(keptBlocks, block)
	}
	if !changed {
		return fmt.Errorf("hook not found")
	}
	if len(keptBlocks) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = keptBlocks
	}
	return saveSettings(configPath, nil, settings)
}

func (a *ClaudeAdapter) IsHookInstalled(scope adapters.InstallScope) (bool, error) {
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
	for _, block := range ensureBlocks(hooks["Stop"]) {
		for _, entry := range ensureBlocks(block["hooks"]) {
			if isEntryMatch(entry) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (a *ClaudeAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return state.HookEvent{}, err
	}

	event := state.HookEvent{
		Agent:          "claude",
		SessionID:      stringValue(raw["session_id"]),
		TranscriptPath: stringValue(raw["transcript_path"]),
		Cwd:            stringValue(raw["cwd"]),
		LastPreview:    stringValue(raw["last_assistant_message"]),
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

func isEntryMatch(entry map[string]any) bool {
	if stringValue(entry["name"]) == hookName {
		return true
	}
	command := stringValue(entry["command"])
	return adapters.IsCommandMatch(command, "claude")
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
