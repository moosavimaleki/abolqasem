package state

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadServerBaseURLRejectsRemoteEnvOverride(t *testing.T) {
	t.Setenv(BaseURLEnvName, "http://example.com:9090")

	if got := LoadServerBaseURL(); got != DefaultBaseURL(DefaultPort) {
		t.Fatalf("expected remote env override to be rejected, got %q", got)
	}
}

func TestLoadServerBaseURLCanonicalizesLoopbackEnvOverride(t *testing.T) {
	t.Setenv(BaseURLEnvName, "http://localhost:3210/")

	if got := LoadServerBaseURL(); got != DefaultBaseURL(3210) {
		t.Fatalf("expected loopback env override to be canonicalized, got %q", got)
	}
}

func TestLoadServerBaseURLRejectsRemoteSavedConfig(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	data, err := json.Marshal(ServerConfig{BaseURL: "http://192.168.1.25:9090", PID: 99})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(GetServerConfigPath(), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if got := LoadServerBaseURL(); got != DefaultBaseURL(DefaultPort) {
		t.Fatalf("expected remote saved config to be rejected, got %q", got)
	}
}

func TestSaveServerRuntimePersistsLoopbackOnly(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	if err := SaveServerRuntime("http://localhost:4567", 123); err != nil {
		t.Fatalf("save runtime: %v", err)
	}

	data, err := os.ReadFile(GetServerConfigPath())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL(4567) {
		t.Fatalf("expected canonical loopback url, got %q", cfg.BaseURL)
	}

	if err := SaveServerRuntime("http://evil.example:4567", 123); err != nil {
		t.Fatalf("save runtime with remote host: %v", err)
	}
	data, err = os.ReadFile(GetServerConfigPath())
	if err != nil {
		t.Fatalf("read config after remote save: %v", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config after remote save: %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL(DefaultPort) {
		t.Fatalf("expected remote host save to fall back to default loopback url, got %q", cfg.BaseURL)
	}
}
