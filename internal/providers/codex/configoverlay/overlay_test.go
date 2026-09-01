package configoverlay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestBuildPreservesUserConfigurationAndAddsLoopbackProvider(t *testing.T) {
	source := t.TempDir()
	runtime := t.TempDir()
	original := "model = \"gpt-5.6\"\n[mcp_servers.docs]\ncommand = \"docs-mcp\"\n"
	if err := os.WriteFile(filepath.Join(source, configName), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Build(Options{SourceHome: source, RuntimeHome: runtime, BaseURL: "http://127.0.0.1:8787/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderID != ProviderID || result.Home != runtime {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(runtime, configName))
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["model_provider"] != ProviderID || config["model"] != "gpt-5.6" {
		t.Fatalf("config does not preserve top-level values: %#v", config)
	}
	providers := config[providerKey].(map[string]any)
	provider := providers[ProviderID].(map[string]any)
	if provider["base_url"] != "http://127.0.0.1:8787/v1" || provider["wire_api"] != "responses" {
		t.Fatalf("unexpected provider: %#v", provider)
	}
	if _, ok := provider["experimental_bearer_token"]; ok {
		t.Fatal("overlay must not write a secret into config")
	}
}

func TestBuildRejectsNonLoopbackProvider(t *testing.T) {
	_, err := Build(Options{RuntimeHome: t.TempDir(), BaseURL: "https://example.com/v1"})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback validation error, got %v", err)
	}
}

func TestEnvironmentKeepsGatewayKeyOutOfConfig(t *testing.T) {
	result := Result{Home: "/tmp/codex", APIKeyEnv: APIKeyEnv}
	env, err := result.Environment("private-key")
	if err != nil || len(env) != 2 || env[1] != APIKeyEnv+"=private-key" {
		t.Fatalf("env=%#v err=%v", env, err)
	}
}

func TestBuildCustomAllowsRemoteProviderAndOptionalCredential(t *testing.T) {
	runtime := t.TempDir()
	result, err := BuildCustom(CustomOptions{
		RuntimeHome:  runtime,
		ProviderID:   "abolqasem_custom_example",
		ProviderName: "Example",
		BaseURL:      "https://provider.example/v1",
		APIKeyEnv:    "EXAMPLE_API_KEY",
	})
	if err != nil {
		t.Fatal(err)
	}
	configData, err := os.ReadFile(filepath.Join(runtime, configName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configData), "secret-value") {
		t.Fatal("custom credential must never be written to the overlay")
	}
	if got := result.OptionalEnvironment(""); len(got) != 1 || got[0] != "CODEX_HOME="+runtime {
		t.Fatalf("unexpected optional env without key: %#v", got)
	}
	if got := result.OptionalEnvironment("secret-value"); len(got) != 2 || got[1] != "EXAMPLE_API_KEY=secret-value" {
		t.Fatalf("unexpected optional env with key: %#v", got)
	}
}
