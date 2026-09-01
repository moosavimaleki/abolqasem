package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abolqasem/internal/state"
)

func TestCodexManagerSidecarConfigUsesManagedStateAndLoopbackURL(t *testing.T) {
	settings := state.DefaultAppSettings()
	settings.CodexBackend.ManagerBaseURL = "http://127.0.0.1:8787/v1"
	config, err := codexManagerSidecarConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	if config.ListenAddress != "127.0.0.1:8787" || config.UpstreamBase == "" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.ManagerHome != filepath.Join(state.GetStateDir(), "codex-manager") {
		t.Fatalf("unexpected manager home: %s", config.ManagerHome)
	}
}

func TestCodexManagerDiagnosticsRedactsLocalCredentialDetails(t *testing.T) {
	previousDir, previousLiveRoot := codexManagerStateDir, codexManagerLiveAuthRoot
	stateDir, codexRoot := t.TempDir(), t.TempDir()
	codexManagerStateDir = func() string { return stateDir }
	codexManagerLiveAuthRoot = func() string { return codexRoot }
	t.Cleanup(func() { codexManagerStateDir, codexManagerLiveAuthRoot = previousDir, previousLiveRoot })
	if err := os.WriteFile(filepath.Join(codexRoot, "auth.json"), []byte(`{"tokens":{"refresh_token":"secret-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnostics := codexManagerDiagnostics()
	live, ok := diagnostics["liveAuth"].(map[string]any)
	if !ok || live["present"] != true {
		t.Fatalf("unexpected live auth diagnostics: %#v", diagnostics)
	}
	if serialized := fmt.Sprintf("%v", diagnostics); strings.Contains(serialized, "secret-token") || strings.Contains(serialized, codexRoot) {
		t.Fatalf("diagnostics exposed sensitive data: %s", serialized)
	}
}

func TestCodexManagerSidecarConfigRejectsRemoteOrPortlessURL(t *testing.T) {
	settings := state.DefaultAppSettings()
	settings.CodexBackend.ManagerBaseURL = "https://example.com/v1"
	if _, err := codexManagerSidecarConfig(settings); err == nil {
		t.Fatal("expected a remote URL rejection")
	}
	settings.CodexBackend.ManagerBaseURL = "http://127.0.0.1/v1"
	if _, err := codexManagerSidecarConfig(settings); err == nil {
		t.Fatal("expected a missing port rejection")
	}
}
