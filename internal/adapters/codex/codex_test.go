package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-session-viewer/internal/adapters"
)

func TestInstallAndUninstallHookPreservesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := `model = "gpt-5.4"

[features]
fast = true

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "echo keep"
`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter := &CodexAdapter{}
	if err := adapter.InstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}
	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read installed config: %v", err)
	}
	if !strings.Contains(string(installed), "codex_hooks = true") {
		t.Fatalf("expected codex_hooks in config: %s", string(installed))
	}
	if !strings.Contains(string(installed), "echo keep") {
		t.Fatalf("expected existing hook to remain: %s", string(installed))
	}

	if err := adapter.UninstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("UninstallHook: %v", err)
	}
	finalData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read final config: %v", err)
	}
	if strings.Contains(string(finalData), "ai-session-viewer hook --agent codex") {
		t.Fatalf("expected our hook to be removed: %s", string(finalData))
	}
	if !strings.Contains(string(finalData), "echo keep") {
		t.Fatalf("expected unrelated hook to remain: %s", string(finalData))
	}
}
