package codex

import (
	"ai-agent-manager/internal/adapters"
	"ai-agent-manager/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

type CodexAdapter struct{}

func New() adapters.AgentAdapter {
	return &CodexAdapter{}
}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) getConfigPath(scope adapters.InstallScope) string {
	if scope == adapters.ScopeProject {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".codex", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

func (a *CodexAdapter) InstallHook(scope adapters.InstallScope) error {
	configPath := a.getConfigPath(scope)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(data) == 0 {
		data = []byte{}
	}

	cfg := map[string]any{}
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse %s: %w", configPath, err)
		}
	}

	features := ensureMap(cfg, "features")
	features["hooks"] = true
	delete(features, "codex_hooks")

	hooks := ensureMap(cfg, "hooks")
	stopBlocks := ensureHookBlocks(hooks["Stop"])
	hooks["Stop"], _ = ensureCodexHook(stopBlocks)
	promptBlocks := ensureHookBlocks(hooks["PromptSubmitted"])
	hooks["PromptSubmitted"], _ = ensureCodexEnsureServerHook(promptBlocks)

	if len(data) > 0 {
		if err := os.WriteFile(configPath+".bak", data, 0o644); err != nil {
			return err
		}
	}

	out, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0o644)
}

func (a *CodexAdapter) UninstallHook(scope adapters.InstallScope) error {
	configPath := a.getConfigPath(scope)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	cfg := map[string]any{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok {
		return fmt.Errorf("hook not found")
	}

	stopBlocks := ensureHookBlocks(hooks["Stop"])
	newBlocks, removed := removeCodexHook(stopBlocks)
	promptBlocks := ensureHookBlocks(hooks["PromptSubmitted"])
	newPromptBlocks, removedPrompt := removeCodexEnsureServerHook(promptBlocks)
	if !removed && !removedPrompt {
		return fmt.Errorf("hook not found")
	}
	if removed && len(newBlocks) == 0 {
		delete(hooks, "Stop")
	} else if removed {
		hooks["Stop"] = newBlocks
	}
	if removedPrompt && len(newPromptBlocks) == 0 {
		delete(hooks, "PromptSubmitted")
	} else if removedPrompt {
		hooks["PromptSubmitted"] = newPromptBlocks
	}

	out, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0o644)
}

func (a *CodexAdapter) IsHookInstalled(scope adapters.InstallScope) (bool, error) {
	configPath := a.getConfigPath(scope)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	cfg := map[string]any{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}
	for _, block := range ensureHookBlocks(hooks["Stop"]) {
		for _, inner := range ensureHookEntries(block["hooks"]) {
			if adapters.IsCommandMatch(stringValue(inner["command"]), "codex") {
				return true, nil
			}
		}
	}
	return false, nil
}

func (a *CodexAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var event state.HookEvent
	if err := json.Unmarshal(input, &event); err != nil {
		return event, err
	}
	event.Agent = "codex"
	return state.NormalizeAndValidateEvent(event), nil
}

func ensureCodexHook(blocks []map[string]any) ([]map[string]any, bool) {
	command, err := adapters.ShellCommand("codex")
	if err != nil {
		command = "ai-agent-manager hook --agent codex"
	}
	for _, block := range blocks {
		entries := ensureHookEntries(block["hooks"])
		block["hooks"] = entries
		for _, inner := range entries {
			if adapters.IsCommandMatch(stringValue(inner["command"]), "codex") {
				changed := false
				if stringValue(inner["type"]) != "command" {
					inner["type"] = "command"
					changed = true
				}
				if stringValue(inner["command"]) != command {
					inner["command"] = command
					changed = true
				}
				if inner["timeout"] != 3 {
					inner["timeout"] = 3
					changed = true
				}
				return blocks, changed
			}
		}
	}
	return append(blocks, map[string]any{
		"hooks": []map[string]any{
			{
				"type":    "command",
				"command": command,
				"timeout": 3,
			},
		},
	}), true
}

func ensureCodexEnsureServerHook(blocks []map[string]any) ([]map[string]any, bool) {
	command, err := adapters.EnsureServerShellCommand()
	if err != nil {
		command = "ai-agent-manager __ensure-server"
	}
	for _, block := range blocks {
		entries := ensureHookEntries(block["hooks"])
		block["hooks"] = entries
		for _, inner := range entries {
			if adapters.IsEnsureServerCommandMatch(stringValue(inner["command"])) {
				changed := false
				if stringValue(inner["type"]) != "command" {
					inner["type"] = "command"
					changed = true
				}
				if stringValue(inner["command"]) != command {
					inner["command"] = command
					changed = true
				}
				if inner["timeout"] != 3 {
					inner["timeout"] = 3
					changed = true
				}
				return blocks, changed
			}
		}
	}
	return append(blocks, map[string]any{
		"hooks": []map[string]any{
			{
				"type":    "command",
				"command": command,
				"timeout": 3,
			},
		},
	}), true
}

func removeCodexHook(blocks []map[string]any) ([]map[string]any, bool) {
	changed := false
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		entries := ensureHookEntries(block["hooks"])
		kept := make([]map[string]any, 0, len(entries))
		for _, inner := range entries {
			if adapters.IsCommandMatch(stringValue(inner["command"]), "codex") {
				changed = true
				continue
			}
			kept = append(kept, inner)
		}
		if len(kept) == 0 {
			continue
		}
		block["hooks"] = kept
		result = append(result, block)
	}
	return result, changed
}

func removeCodexEnsureServerHook(blocks []map[string]any) ([]map[string]any, bool) {
	changed := false
	result := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		entries := ensureHookEntries(block["hooks"])
		kept := make([]map[string]any, 0, len(entries))
		for _, inner := range entries {
			if adapters.IsEnsureServerCommandMatch(stringValue(inner["command"])) {
				changed = true
				continue
			}
			kept = append(kept, inner)
		}
		if len(kept) == 0 {
			continue
		}
		block["hooks"] = kept
		result = append(result, block)
	}
	return result, changed
}

func ensureMap(target map[string]any, key string) map[string]any {
	if value, ok := target[key].(map[string]any); ok {
		return value
	}
	child := map[string]any{}
	target[key] = child
	return child
}

func ensureHookBlocks(value any) []map[string]any {
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
	case nil:
		return []map[string]any{}
	default:
		return []map[string]any{}
	}
}

func ensureHookEntries(value any) []map[string]any {
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
	case nil:
		return []map[string]any{}
	default:
		return []map[string]any{}
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
