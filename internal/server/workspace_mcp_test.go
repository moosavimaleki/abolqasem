package server

import (
	"encoding/json"
	"os"
	"path/filepath"
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
