package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-session-viewer/internal/adapters"
)

func TestInstallAndUninstallHookPreservesOtherHooks(t *testing.T) {
	home := testHome(t)
	configPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "keep-me"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter := &ClaudeAdapter{}
	if err := adapter.InstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}
	if err := adapter.UninstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("UninstallHook: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "keep-me") {
		t.Fatalf("expected unrelated hook to remain: %s", string(data))
	}
	if strings.Contains(string(data), hookName) {
		t.Fatalf("expected our hook name removed: %s", string(data))
	}
}

func TestInstallHookIsIdempotentAndRepairsExistingHook(t *testing.T) {
	home := testHome(t)
	configPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "name": "ai-session-viewer-claude-stop",
            "type": "command",
            "command": "/old/bin/ai-session-viewer hook --agent claude",
            "timeout": 1
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter := &ClaudeAdapter{}
	if err := adapter.InstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("first InstallHook: %v", err)
	}
	if err := adapter.InstallHook(adapters.ScopeUser); err != nil {
		t.Fatalf("second InstallHook should be idempotent: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	config := string(data)
	if strings.Contains(config, "/old/bin/ai-session-viewer") {
		t.Fatalf("expected stale hook command to be repaired: %s", config)
	}
	if !strings.Contains(config, "hook") || !strings.Contains(config, "--agent") || !strings.Contains(config, "claude") {
		t.Fatalf("expected claude hook command args: %s", config)
	}
	if !strings.Contains(config, `"timeout": 3`) {
		t.Fatalf("expected timeout to be repaired: %s", config)
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
