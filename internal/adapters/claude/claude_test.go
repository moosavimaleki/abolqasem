package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-session-viewer/internal/adapters"
)

func TestInstallAndUninstallHookPreservesOtherHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
