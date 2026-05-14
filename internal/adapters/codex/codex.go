package codex

import (
	"ai-session-viewer/internal/adapters"
	"ai-session-viewer/internal/state"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const markerStart = "# BEGIN ai-session-viewer"
const markerEnd = "# END ai-session-viewer"
const hookConfig = `
[features]
codex_hooks = true

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "ai-session-viewer hook --agent codex"
timeout = 3
`

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
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(configPath), 0755)
		os.WriteFile(configPath, []byte(""), 0644)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	content := string(data)
	if strings.Contains(content, markerStart) {
		return fmt.Errorf("hook already installed")
	}

	os.WriteFile(configPath+".bak", data, 0644)

	newContent := content + "\n" + markerStart + hookConfig + markerEnd + "\n"
	return os.WriteFile(configPath, []byte(newContent), 0644)
}

func (a *CodexAdapter) UninstallHook(scope adapters.InstallScope) error {
	configPath := a.getConfigPath(scope)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	content := string(data)
	if !strings.Contains(content, markerStart) {
		return fmt.Errorf("hook not found")
	}

	startIdx := strings.Index(content, markerStart)
	endIdx := strings.Index(content, markerEnd) + len(markerEnd)

	newContent := content[:startIdx] + content[endIdx:]
	newContent = strings.ReplaceAll(newContent, "\n\n\n", "\n\n")

	return os.WriteFile(configPath, []byte(newContent), 0644)
}

func (a *CodexAdapter) NormalizeHookInput(input []byte) (state.HookEvent, error) {
	var event state.HookEvent
	if err := json.Unmarshal(input, &event); err != nil {
		return event, err
	}

	event.Agent = "codex"
	if event.SessionID == "" && event.TranscriptPath != "" {
		event.SessionID = filepath.Base(filepath.Dir(event.TranscriptPath))
	}
	return event, nil
}
