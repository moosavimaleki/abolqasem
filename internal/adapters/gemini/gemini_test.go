package gemini

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
	configPath := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := `{
  "hooks": {
    "AfterAgent": [
      {
        "matcher": "*",
        "hooks": [
          {
            "name": "keep-me",
            "type": "command",
            "command": "echo keep"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter := &GeminiAdapter{}
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
	if strings.Contains(string(data), afterAgentHookName) || strings.Contains(string(data), sessionEndHookName) {
		t.Fatalf("expected our hooks removed: %s", string(data))
	}
}

func TestInstallHookIsIdempotentAndRepairsExistingHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := `{
  "hooks": {
    "AfterAgent": [
      {
        "matcher": "*",
        "hooks": [
          {
            "name": "ai-session-viewer-gemini-after-agent",
            "type": "command",
            "command": "/old/bin/ai-session-viewer hook --agent gemini"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "*",
        "hooks": [
          {
            "name": "ai-session-viewer-gemini-session-end",
            "type": "command",
            "command": "/old/bin/ai-session-viewer hook --agent gemini"
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	adapter := &GeminiAdapter{}
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
		t.Fatalf("expected stale hook commands to be repaired: %s", config)
	}
	if !strings.Contains(config, " hook --agent gemini") {
		t.Fatalf("expected gemini hook command: %s", config)
	}
}
