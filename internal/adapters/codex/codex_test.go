package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-agent-manager/internal/adapters"
)

func TestInstallAndUninstallHookPreservesConfig(t *testing.T) {
	home := testHome(t)
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
	if !strings.Contains(string(installed), "hooks = true") {
		t.Fatalf("expected hooks feature in config: %s", string(installed))
	}
	if strings.Contains(string(installed), "codex_hooks") {
		t.Fatalf("expected deprecated codex_hooks to be absent: %s", string(installed))
	}
	if !strings.Contains(string(installed), "echo keep") {
		t.Fatalf("expected existing hook to remain: %s", string(installed))
	}
	if !strings.Contains(string(installed), "UserPromptSubmit") || !strings.Contains(string(installed), " hook --agent codex") {
		t.Fatalf("expected prompt hook to record codex events: %s", string(installed))
	}

	if err := adapter.UninstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("UninstallHook: %v", err)
	}
	finalData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read final config: %v", err)
	}
	if strings.Contains(string(finalData), " hook --agent codex") {
		t.Fatalf("expected our hook to be removed: %s", string(finalData))
	}
	if strings.Contains(string(finalData), " ensure-server") {
		t.Fatalf("expected prompt startup hook to be removed: %s", string(finalData))
	}
	if !strings.Contains(string(finalData), "echo keep") {
		t.Fatalf("expected unrelated hook to remain: %s", string(finalData))
	}
}

func TestInstallHookIsIdempotentAndMigratesDeprecatedFeature(t *testing.T) {
	home := testHome(t)
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := `[features]
codex_hooks = true

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "/old/bin/ai-agent-manager hook --agent codex"
timeout = 1

[[hooks.PromptSubmitted]]
[[hooks.PromptSubmitted.hooks]]
type = "command"
command = "/old/bin/ai-agent-manager __ensure-server"
timeout = 1
`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter := &CodexAdapter{}
	if err := adapter.InstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("first InstallHook: %v", err)
	}
	if err := adapter.InstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("second InstallHook should be idempotent: %v", err)
	}

	installed, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read installed config: %v", err)
	}
	config := string(installed)
	if !strings.Contains(config, "hooks = true") {
		t.Fatalf("expected new hooks feature: %s", config)
	}
	if strings.Contains(config, "codex_hooks") {
		t.Fatalf("expected deprecated feature to be removed: %s", config)
	}
	if strings.Contains(config, "/old/bin/ai-agent-manager") {
		t.Fatalf("expected stale hook command to be repaired: %s", config)
	}
	if !strings.Contains(config, " hook --agent codex") {
		t.Fatalf("expected codex hook command: %s", config)
	}
	if !strings.Contains(config, "UserPromptSubmit") {
		t.Fatalf("expected codex user prompt submit hook: %s", config)
	}
	if strings.Contains(config, "PromptSubmitted") {
		t.Fatalf("expected legacy prompt hook to be removed: %s", config)
	}
	if !strings.Contains(config, "timeout = 3") {
		t.Fatalf("expected timeout to be repaired: %s", config)
	}
}

func TestNormalizeHookInputCapturesPromptSubmit(t *testing.T) {
	event, err := (&CodexAdapter{}).NormalizeHookInput([]byte(`{
		"hook_event_name": "UserPromptSubmit",
		"session_id": "session-1",
		"transcript_path": "/tmp/session.jsonl",
		"cwd": "/tmp/project",
		"prompt": "برگرد به checkpoint"
	}`))
	if err != nil {
		t.Fatalf("NormalizeHookInput returned error: %v", err)
	}
	if event.HookEventName != "UserPromptSubmit" || event.PromptPreview != "برگرد به checkpoint" {
		t.Fatalf("unexpected hook event: %#v", event)
	}
}

func testHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	return home
}
