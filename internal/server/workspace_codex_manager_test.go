package server

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
)

func TestWorkspacePrepareCodexRuntimeUsesIsolatedManagerOverlay(t *testing.T) {
	sourceHome := t.TempDir()
	runtimeHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceHome, "config.toml"), []byte("model = \"gpt-5.6\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", sourceHome)
	previousLoad := workspaceLoadProviderSettings
	previousSecret := workspaceGetCodexManagerSecret
	previousRuntime := workspaceCodexManagerRuntimeDir
	workspaceLoadProviderSettings = func() (state.AppSettings, error) {
		settings := state.DefaultAppSettings()
		settings.CodexBackend.Mode = state.CodexBackendManager
		settings.CodexBackend.Enabled = true
		settings.CodexBackend.ManagerBaseURL = "http://127.0.0.1:8787/v1"
		return settings, nil
	}
	workspaceGetCodexManagerSecret = func(name string) (string, error) {
		if name != codexManagerGatewaySecretName {
			t.Fatalf("unexpected secret name: %s", name)
		}
		return "gateway-secret", nil
	}
	workspaceCodexManagerRuntimeDir = func() string { return runtimeHome }
	t.Cleanup(func() {
		workspaceLoadProviderSettings = previousLoad
		workspaceGetCodexManagerSecret = previousSecret
		workspaceCodexManagerRuntimeDir = previousRuntime
	})

	runtime, err := workspacePrepareCodexRuntime([]string{"PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ModelProvider != "codex_manager" || runtime.Fingerprint != "codex_manager" {
		t.Fatalf("unexpected runtime: %#v", runtime)
	}
	if value := envValue(runtime.Env, "CODEX_HOME"); value != runtimeHome {
		t.Fatalf("isolated CODEX_HOME=%q", value)
	}
	if value := envValue(runtime.Env, "CODEX_MANAGER_GATEWAY_API_KEY"); value != "gateway-secret" {
		t.Fatalf("gateway key was not passed through env: %q", value)
	}
}

func TestWorkspacePrepareCodexRuntimeFailsWithoutGatewayKey(t *testing.T) {
	previousLoad := workspaceLoadProviderSettings
	previousSecret := workspaceGetCodexManagerSecret
	workspaceLoadProviderSettings = func() (state.AppSettings, error) {
		settings := state.DefaultAppSettings()
		settings.CodexBackend.Mode = state.CodexBackendManager
		settings.CodexBackend.Enabled = true
		return settings, nil
	}
	workspaceGetCodexManagerSecret = func(string) (string, error) { return "", errors.New("not configured") }
	t.Cleanup(func() {
		workspaceLoadProviderSettings = previousLoad
		workspaceGetCodexManagerSecret = previousSecret
	})

	if _, err := workspacePrepareCodexRuntime(nil); err == nil {
		t.Fatal("expected an actionable missing gateway key error")
	}
}

func TestWorkspacePrepareCodexTurnUsesCustomProviderMapping(t *testing.T) {
	runtimeHome := t.TempDir()
	previousLoad := workspaceLoadProviderSettings
	previousSecret := workspaceGetCustomProviderSecret
	previousRuntime := workspaceCodexManagerRuntimeDir
	workspaceLoadProviderSettings = func() (state.AppSettings, error) {
		settings := state.DefaultAppSettings()
		settings.CodexBackend.Mode = state.CodexBackendCustom
		settings.CodexBackend.Enabled = true
		settings.CodexBackend.CustomProviderID = "remote"
		settings.CodexBackend.CustomProviders["remote"] = state.CustomProviderSettings{
			Name: "Remote", BaseURL: "https://provider.example/v1", WireAPI: "responses", EnvKey: "REMOTE_API_KEY",
			Models: []state.CustomProviderModelSettings{{ID: "friendly-model", UpstreamID: "upstream-model"}},
		}
		return settings, nil
	}
	workspaceGetCustomProviderSecret = func(name string) (string, error) {
		if name != "custom-provider-remote" {
			t.Fatalf("unexpected secret name: %s", name)
		}
		return "provider-secret", nil
	}
	workspaceCodexManagerRuntimeDir = func() string { return runtimeHome }
	t.Cleanup(func() {
		workspaceLoadProviderSettings = previousLoad
		workspaceGetCustomProviderSecret = previousSecret
		workspaceCodexManagerRuntimeDir = previousRuntime
	})

	// The real secret store is intentionally not populated. The test verifies
	// the optional-key path as well as the mapping contract without persisting a
	// credential in the test user's state directory.
	previousConfigured := workspaceCustomProviderSecretConfigured
	workspaceCustomProviderSecretConfigured = func(string) bool { return true }
	t.Cleanup(func() { workspaceCustomProviderSecretConfigured = previousConfigured })

	request, runtime, err := workspacePrepareCodexTurn(agent.TurnRequest{Model: "friendly-model", Env: []string{"PATH=/bin"}})
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "upstream-model" || request.CodexModelProvider != "abolqasem_custom_remote" {
		t.Fatalf("custom runtime did not map request: %#v", request)
	}
	if runtime.Fingerprint != "abolqasem_custom_remote" || envValue(request.Env, "REMOTE_API_KEY") != "provider-secret" {
		t.Fatalf("unexpected custom runtime: %#v", runtime)
	}
}
