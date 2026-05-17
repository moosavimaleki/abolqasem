package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type mcpTransport string

const (
	mcpTransportStdio mcpTransport = "stdio"
	mcpTransportHTTP  mcpTransport = "http"
)

type mcpProviderID string

const (
	mcpProviderCodex  mcpProviderID = "codex"
	mcpProviderClaude mcpProviderID = "claude"
	mcpProviderGemini mcpProviderID = "gemini"
)

type mcpServerConfig struct {
	Name      string            `json:"name"`
	Transport mcpTransport      `json:"transport"`
	Providers []mcpProviderID   `json:"providers"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

type mcpSettingsSnapshot struct {
	ConfigPaths map[mcpProviderID]string `json:"configPaths"`
	Servers     []mcpServerConfig        `json:"servers"`
}

type mcpSaveResult struct {
	mcpSettingsSnapshot
	Server mcpServerConfig `json:"server"`
}

var mcpServerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func workspaceMCPList(_ json.RawMessage) (mcpSettingsSnapshot, error) {
	return loadWorkspaceMCPSnapshot()
}

func workspaceMCPSave(raw json.RawMessage) (mcpSaveResult, error) {
	var payload struct {
		Server mcpServerConfig `json:"server"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return mcpSaveResult{}, err
	}
	server, err := normalizeMCPServerConfig(payload.Server)
	if err != nil {
		return mcpSaveResult{}, err
	}

	selected := map[mcpProviderID]bool{}
	for _, provider := range server.Providers {
		selected[provider] = true
	}
	for _, provider := range mcpProviderOrder() {
		if selected[provider] {
			if err := writeMCPServerForProvider(provider, server); err != nil {
				return mcpSaveResult{}, err
			}
			continue
		}
		if err := removeMCPServerFromProvider(provider, server.Name); err != nil {
			return mcpSaveResult{}, err
		}
	}

	snapshot, err := loadWorkspaceMCPSnapshot()
	if err != nil {
		return mcpSaveResult{}, err
	}
	return mcpSaveResult{mcpSettingsSnapshot: snapshot, Server: server}, nil
}

func workspaceMCPRemove(raw json.RawMessage) (mcpSettingsSnapshot, error) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return mcpSettingsSnapshot{}, err
	}
	name := strings.TrimSpace(payload.Name)
	if !mcpServerNamePattern.MatchString(name) {
		return mcpSettingsSnapshot{}, errors.New("invalid MCP server name")
	}
	for _, provider := range mcpProviderOrder() {
		if err := removeMCPServerFromProvider(provider, name); err != nil {
			return mcpSettingsSnapshot{}, err
		}
	}
	return loadWorkspaceMCPSnapshot()
}

func loadWorkspaceMCPSnapshot() (mcpSettingsSnapshot, error) {
	paths := workspaceMCPConfigPaths()
	merged := map[string]mcpServerConfig{}
	for _, provider := range mcpProviderOrder() {
		servers, err := readMCPServersForProvider(provider)
		if err != nil {
			return mcpSettingsSnapshot{}, err
		}
		for _, server := range servers {
			current := merged[server.Name]
			if current.Name == "" {
				current = server
				current.Providers = nil
			} else {
				current = fillMissingMCPServerFields(current, server)
			}
			current.Providers = appendMCPProvider(current.Providers, provider)
			merged[server.Name] = current
		}
	}
	servers := make([]mcpServerConfig, 0, len(merged))
	for _, server := range merged {
		server.Providers = sortedMCPProviders(server.Providers)
		servers = append(servers, server)
	}
	sort.Slice(servers, func(i, j int) bool {
		return strings.ToLower(servers[i].Name) < strings.ToLower(servers[j].Name)
	})
	return mcpSettingsSnapshot{ConfigPaths: paths, Servers: servers}, nil
}

func normalizeMCPServerConfig(server mcpServerConfig) (mcpServerConfig, error) {
	server.Name = strings.TrimSpace(server.Name)
	if !mcpServerNamePattern.MatchString(server.Name) {
		return mcpServerConfig{}, errors.New("MCP server name must be 1-64 characters and use letters, numbers, '.', '_' or '-'")
	}
	server.Transport = mcpTransport(strings.ToLower(strings.TrimSpace(string(server.Transport))))
	if server.Transport == "" {
		server.Transport = mcpTransportStdio
	}
	if server.Transport != mcpTransportStdio && server.Transport != mcpTransportHTTP {
		return mcpServerConfig{}, errors.New("unsupported MCP transport")
	}
	server.Command = strings.TrimSpace(server.Command)
	server.URL = strings.TrimSpace(server.URL)
	server.Args = normalizeStringList(server.Args)
	server.Env = normalizeStringMap(server.Env)
	server.Headers = normalizeStringMap(server.Headers)
	server.Providers = sortedMCPProviders(server.Providers)
	if len(server.Providers) == 0 {
		return mcpServerConfig{}, errors.New("select at least one provider")
	}
	if server.Transport == mcpTransportStdio {
		if server.Command == "" {
			return mcpServerConfig{}, errors.New("stdio MCP servers require a command")
		}
		server.URL = ""
		server.Headers = nil
	} else {
		if server.URL == "" {
			return mcpServerConfig{}, errors.New("HTTP MCP servers require a URL")
		}
		server.Command = ""
		server.Args = nil
		server.Env = nil
	}
	return server, nil
}

func mcpProviderOrder() []mcpProviderID {
	return []mcpProviderID{mcpProviderCodex, mcpProviderClaude, mcpProviderGemini}
}

func workspaceMCPConfigPaths() map[mcpProviderID]string {
	home, _ := os.UserHomeDir()
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	geminiHome := strings.TrimSpace(os.Getenv("GEMINI_CLI_HOME"))
	if geminiHome == "" {
		geminiHome = filepath.Join(home, ".gemini")
	} else if filepath.Base(filepath.Clean(geminiHome)) != ".gemini" {
		geminiHome = filepath.Join(geminiHome, ".gemini")
	}
	return map[mcpProviderID]string{
		mcpProviderCodex:  filepath.Join(codexHome, "config.toml"),
		mcpProviderClaude: filepath.Join(home, ".mcp.json"),
		mcpProviderGemini: filepath.Join(geminiHome, "settings.json"),
	}
}

func readMCPServersForProvider(provider mcpProviderID) ([]mcpServerConfig, error) {
	switch provider {
	case mcpProviderCodex:
		return readCodexMCPServers(workspaceMCPConfigPaths()[provider])
	case mcpProviderClaude:
		return readJSONMCPServers(workspaceMCPConfigPaths()[provider], provider)
	case mcpProviderGemini:
		return readJSONMCPServers(workspaceMCPConfigPaths()[provider], provider)
	default:
		return nil, fmt.Errorf("unsupported MCP provider: %s", provider)
	}
}

func writeMCPServerForProvider(provider mcpProviderID, server mcpServerConfig) error {
	switch provider {
	case mcpProviderCodex:
		return writeCodexMCPServer(workspaceMCPConfigPaths()[provider], server)
	case mcpProviderClaude:
		return writeJSONMCPServer(workspaceMCPConfigPaths()[provider], provider, server)
	case mcpProviderGemini:
		return writeJSONMCPServer(workspaceMCPConfigPaths()[provider], provider, server)
	default:
		return fmt.Errorf("unsupported MCP provider: %s", provider)
	}
}

func removeMCPServerFromProvider(provider mcpProviderID, name string) error {
	switch provider {
	case mcpProviderCodex:
		return removeCodexMCPServer(workspaceMCPConfigPaths()[provider], name)
	case mcpProviderClaude:
		return removeJSONMCPServer(workspaceMCPConfigPaths()[provider], name)
	case mcpProviderGemini:
		return removeJSONMCPServer(workspaceMCPConfigPaths()[provider], name)
	default:
		return fmt.Errorf("unsupported MCP provider: %s", provider)
	}
}

func readCodexMCPServers(path string) ([]mcpServerConfig, error) {
	root, err := readTOMLMap(path)
	if err != nil {
		return nil, err
	}
	rawServers := asMap(root["mcp_servers"])
	servers := make([]mcpServerConfig, 0, len(rawServers))
	for name, raw := range rawServers {
		rawMap := asMap(raw)
		server := mcpServerFromMap(name, rawMap)
		if server.Name != "" {
			servers = append(servers, server)
		}
	}
	return servers, nil
}

func writeCodexMCPServer(path string, server mcpServerConfig) error {
	root, err := readTOMLMap(path)
	if err != nil {
		return err
	}
	servers := asMap(root["mcp_servers"])
	if servers == nil {
		servers = map[string]any{}
	}
	servers[server.Name] = mcpServerToCodexMap(server)
	root["mcp_servers"] = servers
	return writeTOMLMap(path, root)
}

func removeCodexMCPServer(path string, name string) error {
	root, err := readTOMLMap(path)
	if err != nil {
		return err
	}
	servers := asMap(root["mcp_servers"])
	if servers == nil {
		return nil
	}
	delete(servers, name)
	root["mcp_servers"] = servers
	return writeTOMLMap(path, root)
}

func readJSONMCPServers(path string, provider mcpProviderID) ([]mcpServerConfig, error) {
	root, err := readJSONMap(path)
	if err != nil {
		return nil, err
	}
	rawServers := asMap(root["mcpServers"])
	servers := make([]mcpServerConfig, 0, len(rawServers))
	for name, raw := range rawServers {
		rawMap := asMap(raw)
		server := mcpServerFromMap(name, rawMap)
		if provider == mcpProviderGemini && server.URL == "" {
			server.URL = stringFromAny(rawMap["httpUrl"])
			if server.URL != "" {
				server.Transport = mcpTransportHTTP
			}
		}
		if server.Name != "" {
			servers = append(servers, server)
		}
	}
	return servers, nil
}

func writeJSONMCPServer(path string, provider mcpProviderID, server mcpServerConfig) error {
	root, err := readJSONMap(path)
	if err != nil {
		return err
	}
	servers := asMap(root["mcpServers"])
	if servers == nil {
		servers = map[string]any{}
	}
	servers[server.Name] = mcpServerToJSONMap(provider, server)
	root["mcpServers"] = servers
	return writeJSONMap(path, root)
}

func removeJSONMCPServer(path string, name string) error {
	root, err := readJSONMap(path)
	if err != nil {
		return err
	}
	servers := asMap(root["mcpServers"])
	if servers == nil {
		return nil
	}
	delete(servers, name)
	root["mcpServers"] = servers
	return writeJSONMap(path, root)
}

func mcpServerFromMap(name string, raw map[string]any) mcpServerConfig {
	name = strings.TrimSpace(name)
	if !mcpServerNamePattern.MatchString(name) {
		return mcpServerConfig{}
	}
	server := mcpServerConfig{
		Name:    name,
		Command: stringFromAny(raw["command"]),
		Args:    stringListFromAny(raw["args"]),
		URL:     firstNonEmptyMCPString(stringFromAny(raw["url"]), stringFromAny(raw["httpUrl"])),
		Env:     stringMapFromAny(raw["env"]),
		Headers: firstNonEmptyStringMap(stringMapFromAny(raw["headers"]), stringMapFromAny(raw["http_headers"])),
	}
	rawType := strings.ToLower(strings.TrimSpace(stringFromAny(raw["type"])))
	if server.URL != "" || rawType == "http" || rawType == "sse" {
		server.Transport = mcpTransportHTTP
	} else {
		server.Transport = mcpTransportStdio
	}
	return server
}

func mcpServerToCodexMap(server mcpServerConfig) map[string]any {
	raw := map[string]any{}
	if server.Transport == mcpTransportHTTP {
		raw["url"] = server.URL
		if len(server.Headers) > 0 {
			raw["http_headers"] = server.Headers
		}
		return raw
	}
	raw["command"] = server.Command
	if len(server.Args) > 0 {
		raw["args"] = server.Args
	}
	if len(server.Env) > 0 {
		raw["env"] = server.Env
	}
	return raw
}

func mcpServerToJSONMap(provider mcpProviderID, server mcpServerConfig) map[string]any {
	raw := map[string]any{}
	if server.Transport == mcpTransportHTTP {
		raw["type"] = "http"
		if provider == mcpProviderGemini {
			raw["httpUrl"] = server.URL
		} else {
			raw["url"] = server.URL
		}
		if len(server.Headers) > 0 {
			raw["headers"] = server.Headers
		}
		return raw
	}
	raw["command"] = server.Command
	if len(server.Args) > 0 {
		raw["args"] = server.Args
	}
	if len(server.Env) > 0 {
		raw["env"] = server.Env
	}
	return raw
}

func readTOMLMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	root := map[string]any{}
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func writeTOMLMap(path string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func writeJSONMap(path string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func asMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func stringListFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringFromAny(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapFromAny(value any) map[string]string {
	raw := asMap(value)
	if len(raw) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range raw {
		key = strings.TrimSpace(key)
		stringValue := stringFromAny(value)
		if key != "" && stringValue != "" {
			out[key] = stringValue
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmptyMCPString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyStringMap(values ...map[string]string) map[string]string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func appendMCPProvider(providers []mcpProviderID, provider mcpProviderID) []mcpProviderID {
	for _, candidate := range providers {
		if candidate == provider {
			return providers
		}
	}
	return append(providers, provider)
}

func sortedMCPProviders(providers []mcpProviderID) []mcpProviderID {
	allowed := map[mcpProviderID]bool{}
	for _, provider := range providers {
		switch provider {
		case mcpProviderCodex, mcpProviderClaude, mcpProviderGemini:
			allowed[provider] = true
		}
	}
	out := make([]mcpProviderID, 0, len(allowed))
	for _, provider := range mcpProviderOrder() {
		if allowed[provider] {
			out = append(out, provider)
		}
	}
	return out
}

func fillMissingMCPServerFields(base mcpServerConfig, fallback mcpServerConfig) mcpServerConfig {
	if base.Transport == "" {
		base.Transport = fallback.Transport
	}
	if base.Command == "" {
		base.Command = fallback.Command
	}
	if len(base.Args) == 0 {
		base.Args = fallback.Args
	}
	if base.URL == "" {
		base.URL = fallback.URL
	}
	if len(base.Env) == 0 {
		base.Env = fallback.Env
	}
	if len(base.Headers) == 0 {
		base.Headers = fallback.Headers
	}
	return base
}
