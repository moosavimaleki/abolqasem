package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWorkspaceMCPSaveListRemoveAcrossProviders(t *testing.T) {
	home := t.TempDir()
	codexHome := filepath.Join(home, "codex-home")
	geminiHome := filepath.Join(home, "gemini-home")
	geminiConfigDir := filepath.Join(geminiHome, ".gemini")
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("GEMINI_CLI_HOME", geminiHome)

	savePayload := []byte(`{
		"type": "mcp.save",
		"server": {
			"name": "browsermcp",
			"transport": "stdio",
			"command": "npx",
			"args": ["@browsermcp/mcp"],
			"env": {"BROWSERMCP_TOKEN": "test-token"},
			"providers": ["codex", "claude", "gemini"]
		}
	}`)
	result, err := workspaceMCPSave(savePayload)
	if err != nil {
		t.Fatalf("workspaceMCPSave returned error: %v", err)
	}
	if len(result.Servers) != 1 {
		t.Fatalf("expected one MCP server, got %#v", result.Servers)
	}
	if got := result.Servers[0].Providers; len(got) != 3 || got[0] != "codex" || got[1] != "claude" || got[2] != "gemini" {
		t.Fatalf("expected all providers in stable order, got %#v", got)
	}

	paths := workspaceMCPConfigPaths()
	for provider, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s config at %s: %v", provider, path, err)
		}
	}

	claudeConfig, err := readJSONMap(filepath.Join(home, ".mcp.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	if _, ok := asMap(claudeConfig["mcpServers"])["browsermcp"]; !ok {
		t.Fatalf("expected claude mcp server, got %#v", claudeConfig)
	}

	geminiConfig, err := readJSONMap(filepath.Join(geminiConfigDir, "settings.json"))
	if err != nil {
		t.Fatalf("read gemini config: %v", err)
	}
	if _, ok := asMap(geminiConfig["mcpServers"])["browsermcp"]; !ok {
		t.Fatalf("expected gemini mcp server, got %#v", geminiConfig)
	}

	list, err := workspaceMCPList(json.RawMessage(`{"type":"mcp.list"}`))
	if err != nil {
		t.Fatalf("workspaceMCPList returned error: %v", err)
	}
	if len(list.Servers) != 1 || list.Servers[0].Name != "browsermcp" {
		t.Fatalf("unexpected MCP list: %#v", list.Servers)
	}

	removed, err := workspaceMCPRemove([]byte(`{"type":"mcp.remove","name":"browsermcp"}`))
	if err != nil {
		t.Fatalf("workspaceMCPRemove returned error: %v", err)
	}
	if len(removed.Servers) != 0 {
		t.Fatalf("expected no servers after remove, got %#v", removed.Servers)
	}
}

func TestMCPRegistrySearchUsesOfficialRegistryShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/servers" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("search") != "postgres" || r.URL.Query().Get("version") != "latest" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"servers": [{
				"server": {
					"name": "ai.waystation/postgres",
					"description": "Connect to PostgreSQL.",
					"version": "0.3.1",
					"remotes": [{
						"type": "streamable-http",
						"url": "https://waystation.ai/postgres/mcp",
						"headers": [{"name": "Authorization", "value": "Bearer {token}", "isRequired": true}]
					}]
				},
				"_meta": {"io.modelcontextprotocol.registry/official": {"status": "active", "isLatest": true}}
			}],
			"metadata": {"count": 1}
		}`))
	}))
	defer server.Close()

	originalBaseURL := mcpRegistryBaseURL
	originalClient := mcpRegistryHTTPClient
	mcpRegistryBaseURL = server.URL
	mcpRegistryHTTPClient = server.Client()
	defer func() {
		mcpRegistryBaseURL = originalBaseURL
		mcpRegistryHTTPClient = originalClient
	}()

	snapshot, err := searchMCPRegistry(" postgres ", 5)
	if err != nil {
		t.Fatalf("searchMCPRegistry returned error: %v", err)
	}
	if snapshot.Query != "postgres" || snapshot.Count != 1 || len(snapshot.Servers) != 1 {
		t.Fatalf("unexpected registry snapshot: %#v", snapshot)
	}
	result := snapshot.Servers[0]
	if !result.Installable || result.Config == nil {
		t.Fatalf("expected installable result, got %#v", result)
	}
	if result.Config.Name != "postgres" || result.Config.Transport != mcpTransportHTTP || result.Config.URL != "https://waystation.ai/postgres/mcp" {
		t.Fatalf("unexpected generated config: %#v", result.Config)
	}
	if !result.RequiresConfiguration || len(result.ConfigurationNotes) == 0 {
		t.Fatalf("expected placeholder header to require configuration, got %#v", result.ConfigurationNotes)
	}
}

func TestMCPRegistryPackageMapsToStdioConfig(t *testing.T) {
	result := mcpRegistrySearchResultFromResponse(mcpRegistryServerResponse{
		Server: mcpRegistryServerJSON{
			Name:        "com.pulsemcp/pulse-fetch",
			Description: "Fetch web resources.",
			Version:     "0.3.2",
			Packages: []mcpRegistryPackage{{
				RegistryType: "npm",
				Identifier:   "@pulsemcp/pulse-fetch",
				Version:      "0.3.2",
				RuntimeHint:  "npx",
				RuntimeArguments: []mcpRegistryArgument{{
					mcpRegistryKeyValue: mcpRegistryKeyValue{Value: "-y"},
					Type:                "positional",
				}},
				Transport: mcpRegistryTransport{Type: "stdio"},
				EnvironmentVariables: []mcpRegistryKeyValue{{
					Name:       "FIRECRAWL_API_KEY",
					IsRequired: true,
					IsSecret:   true,
				}},
			}},
		},
		Meta: mcpRegistryMeta{
			Official: mcpRegistryOfficialMeta{Status: "active", IsLatest: true},
		},
	})
	if !result.Installable || result.Config == nil {
		t.Fatalf("expected installable package result, got %#v", result)
	}
	if result.Config.Name != "pulse-fetch" || result.Config.Transport != mcpTransportStdio || result.Config.Command != "npx" {
		t.Fatalf("unexpected generated config: %#v", result.Config)
	}
	if got := result.Config.Args; len(got) != 2 || got[0] != "-y" || got[1] != "@pulsemcp/pulse-fetch@0.3.2" {
		t.Fatalf("unexpected args: %#v", got)
	}
	expectedInstallCommand := []string{npmCommandName(), "cache", "add", "@pulsemcp/pulse-fetch@0.3.2"}
	if !slices.Equal(result.InstallCommand, expectedInstallCommand) {
		t.Fatalf("unexpected install command: %#v", result.InstallCommand)
	}
	if !result.RequiresConfiguration || len(result.ConfigurationNotes) == 0 {
		t.Fatalf("expected missing env to be reported, got %#v", result.ConfigurationNotes)
	}
}

func TestWorkspaceMCPRegistryInstallDownloadsAndSaves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-home"))
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, "gemini-home"))

	previousRun := runMCPRegistryInstallCommand
	var receivedCommand []string
	runMCPRegistryInstallCommand = func(command []string) (mcpRegistryCommandOutput, error) {
		receivedCommand = append([]string(nil), command...)
		return mcpRegistryCommandOutput{CWD: home, Stdout: "cached", Stderr: ""}, nil
	}
	t.Cleanup(func() { runMCPRegistryInstallCommand = previousRun })

	payload, err := json.Marshal(struct {
		Config         mcpServerConfig `json:"config"`
		InstallCommand []string        `json:"installCommand"`
	}{
		Config: mcpServerConfig{
			Name:      "pulse-fetch",
			Transport: mcpTransportStdio,
			Providers: []mcpProviderID{mcpProviderCodex, mcpProviderClaude},
			Command:   "npx",
			Args:      []string{"-y", "@pulsemcp/pulse-fetch@0.3.2"},
		},
		InstallCommand: []string{npmCommandName(), "cache", "add", "@pulsemcp/pulse-fetch@0.3.2"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := workspaceMCPRegistryInstall(payload)
	if err != nil {
		t.Fatalf("workspaceMCPRegistryInstall returned error: %v", err)
	}
	expectedCommand := []string{npmCommandName(), "cache", "add", "@pulsemcp/pulse-fetch@0.3.2"}
	if !slices.Equal(receivedCommand, expectedCommand) {
		t.Fatalf("unexpected download command: %#v", receivedCommand)
	}
	if result.Server.Name != "pulse-fetch" || result.CWD != home || result.Stdout != "cached" {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if len(result.Servers) != 1 || result.Servers[0].Name != "pulse-fetch" {
		t.Fatalf("expected saved MCP server, got %#v", result.Servers)
	}
}

func TestMCPRegistryInstallCommandValidationRejectsArbitraryCommands(t *testing.T) {
	if _, err := normalizeMCPRegistryInstallCommand([]string{"sh", "-c", "npm cache add package"}); err == nil {
		t.Fatal("expected shell install command to be rejected")
	}
	if _, err := normalizeMCPRegistryInstallCommand([]string{npmCommandName(), "install", "@pulsemcp/pulse-fetch"}); err == nil {
		t.Fatal("expected unsupported npm install command to be rejected")
	}
}
