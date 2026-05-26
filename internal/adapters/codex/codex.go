package codex

import (
	"ai-agent-manager/internal/adapters"
	"ai-agent-manager/internal/appinfo"
	"ai-agent-manager/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

type CodexAdapter struct{}

const (
	promptSubmitHookEvent       = "UserPromptSubmit"
	legacyPromptSubmitHookEvent = "PromptSubmitted"
)

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
	legacyPromptBlocks, removedLegacyPrompt := removeCodexEnsureServerHook(ensureHookBlocks(hooks[legacyPromptSubmitHookEvent]))
	if removedLegacyPrompt && len(legacyPromptBlocks) == 0 {
		delete(hooks, legacyPromptSubmitHookEvent)
	} else if removedLegacyPrompt {
		hooks[legacyPromptSubmitHookEvent] = legacyPromptBlocks
	}
	promptBlocks, _ := removeCodexEnsureServerHook(ensureHookBlocks(hooks[promptSubmitHookEvent]))
	hooks[promptSubmitHookEvent], _ = ensureCodexHook(promptBlocks)

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
	promptBlocks := ensureHookBlocks(hooks[promptSubmitHookEvent])
	newPromptBlocks, removedPromptCodex := removeCodexHook(promptBlocks)
	newPromptBlocks, removedPromptEnsure := removeCodexEnsureServerHook(newPromptBlocks)
	removedPrompt := removedPromptCodex || removedPromptEnsure
	legacyPromptBlocks := ensureHookBlocks(hooks[legacyPromptSubmitHookEvent])
	newLegacyPromptBlocks, removedLegacyPromptCodex := removeCodexHook(legacyPromptBlocks)
	newLegacyPromptBlocks, removedLegacyPromptEnsure := removeCodexEnsureServerHook(newLegacyPromptBlocks)
	removedLegacyPrompt := removedLegacyPromptCodex || removedLegacyPromptEnsure
	if !removed && !removedPrompt && !removedLegacyPrompt {
		return fmt.Errorf("hook not found")
	}
	if removed && len(newBlocks) == 0 {
		delete(hooks, "Stop")
	} else if removed {
		hooks["Stop"] = newBlocks
	}
	if removedPrompt && len(newPromptBlocks) == 0 {
		delete(hooks, promptSubmitHookEvent)
	} else if removedPrompt {
		hooks[promptSubmitHookEvent] = newPromptBlocks
	}
	if removedLegacyPrompt && len(newLegacyPromptBlocks) == 0 {
		delete(hooks, legacyPromptSubmitHookEvent)
	} else if removedLegacyPrompt {
		hooks[legacyPromptSubmitHookEvent] = newLegacyPromptBlocks
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
	return hasCodexHook(ensureHookBlocks(hooks["Stop"])) && hasCodexHook(ensureHookBlocks(hooks[promptSubmitHookEvent])), nil
}

func hasCodexHook(blocks []map[string]any) bool {
	for _, block := range blocks {
		for _, inner := range ensureHookEntries(block["hooks"]) {
			if adapters.IsCommandMatch(stringValue(inner["command"]), "codex") {
				return true
			}
		}
	}
	return false
}

func (a *CodexAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return state.HookEvent{}, err
	}
	event := state.HookEvent{
		Agent:          "codex",
		SessionID:      firstString(raw, "session_id", "sessionId", "thread_id", "threadId"),
		HookEventName:  firstString(raw, "hook_event_name", "hookEventName", "event_name", "eventName", "event", "type", "name"),
		TranscriptPath: firstString(raw, "transcript_path", "transcriptPath", "transcript"),
		Cwd:            firstString(raw, "cwd", "current_working_directory", "working_directory", "workspace"),
		ProjectName:    firstString(raw, "project_name", "projectName"),
		PromptPreview:  firstString(raw, "prompt_preview", "promptPreview", "prompt", "user_prompt", "userPrompt", "message", "input"),
		LastPreview:    firstString(raw, "last_preview", "lastPreview", "last_response", "lastResponse", "response"),
		Model:          firstString(raw, "model"),
		UpdatedAt:      firstString(raw, "updated_at", "updatedAt"),
	}
	return state.NormalizeAndValidateEvent(event), nil
}

func ensureCodexHook(blocks []map[string]any) ([]map[string]any, bool) {
	command, err := adapters.ShellCommand("codex")
	if err != nil {
		command = appinfo.Name + " hook --agent codex"
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
		command = appinfo.Name + " __ensure-server"
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

func firstString(raw map[string]any, keys ...string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(key)] = true
		if value := stringValue(raw[key]); value != "" {
			return value
		}
	}
	return findStringByKey(raw, keySet, 0)
}

func findStringByKey(value any, keys map[string]bool, depth int) string {
	if depth > 6 {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if keys[strings.ToLower(key)] {
				if text := stringValue(item); text != "" {
					return text
				}
			}
		}
		for _, item := range typed {
			if text := findStringByKey(item, keys, depth+1); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := findStringByKey(item, keys, depth+1); text != "" {
				return text
			}
		}
	}
	return ""
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
