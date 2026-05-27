package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const mcpRegistrySearchDefaultLimit = 50

var (
	mcpRegistryBaseURL       = "https://registry.modelcontextprotocol.io"
	mcpRegistryHTTPClient    = http.DefaultClient
	mcpRegistryNameSafeRE    = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
	mcpRegistryNPXYesArgRE   = regexp.MustCompile(`^--?y(?:es)?$`)
	mcpRegistryPlaceholderRE = regexp.MustCompile(`\{[^{}]+\}`)
	mcpRegistryDefaultAgent  = mcpProviderOrder()

	runMCPRegistryInstallCommand = defaultRunMCPRegistryInstallCommand
)

type mcpRegistrySearchSnapshot struct {
	Query   string                    `json:"query"`
	Servers []mcpRegistrySearchResult `json:"servers"`
	Count   int                       `json:"count"`
}

type mcpRegistrySearchResult struct {
	ID                    string           `json:"id"`
	RegistryName          string           `json:"registryName"`
	Name                  string           `json:"name"`
	ConfigName            string           `json:"configName,omitempty"`
	Title                 string           `json:"title,omitempty"`
	Description           string           `json:"description"`
	Version               string           `json:"version"`
	Status                string           `json:"status"`
	SourceURL             string           `json:"sourceUrl,omitempty"`
	RepositoryURL         string           `json:"repositoryUrl,omitempty"`
	WebsiteURL            string           `json:"websiteUrl,omitempty"`
	Transport             mcpTransport     `json:"transport,omitempty"`
	Command               string           `json:"command,omitempty"`
	Args                  []string         `json:"args,omitempty"`
	URL                   string           `json:"url,omitempty"`
	InstallCommand        []string         `json:"installCommand,omitempty"`
	Installable           bool             `json:"installable"`
	InstallReason         string           `json:"installReason,omitempty"`
	RequiresConfiguration bool             `json:"requiresConfiguration,omitempty"`
	ConfigurationNotes    []string         `json:"configurationNotes,omitempty"`
	Config                *mcpServerConfig `json:"config,omitempty"`
}

type mcpRegistryInstallResult struct {
	mcpSaveResult
	InstallCommand []string `json:"installCommand,omitempty"`
	CWD            string   `json:"cwd,omitempty"`
	Stdout         string   `json:"stdout,omitempty"`
	Stderr         string   `json:"stderr,omitempty"`
}

type mcpRegistryCommandOutput struct {
	CWD    string
	Stdout string
	Stderr string
}

type mcpRegistryListResponse struct {
	Servers  []mcpRegistryServerResponse `json:"servers"`
	Metadata mcpRegistryMetadata         `json:"metadata"`
}

type mcpRegistryMetadata struct {
	Count int `json:"count"`
}

type mcpRegistryServerResponse struct {
	Server mcpRegistryServerJSON `json:"server"`
	Meta   mcpRegistryMeta       `json:"_meta"`
}

type mcpRegistryMeta struct {
	Official mcpRegistryOfficialMeta `json:"io.modelcontextprotocol.registry/official"`
}

type mcpRegistryOfficialMeta struct {
	Status        string `json:"status"`
	StatusMessage string `json:"statusMessage"`
	IsLatest      bool   `json:"isLatest"`
}

type mcpRegistryServerJSON struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Repository  mcpRegistryRepository  `json:"repository"`
	WebsiteURL  string                 `json:"websiteUrl"`
	Packages    []mcpRegistryPackage   `json:"packages"`
	Remotes     []mcpRegistryTransport `json:"remotes"`
}

type mcpRegistryRepository struct {
	URL string `json:"url"`
}

type mcpRegistryPackage struct {
	RegistryType         string                `json:"registryType"`
	RegistryBaseURL      string                `json:"registryBaseUrl"`
	Identifier           string                `json:"identifier"`
	Version              string                `json:"version"`
	RuntimeHint          string                `json:"runtimeHint"`
	RuntimeArguments     []mcpRegistryArgument `json:"runtimeArguments"`
	PackageArguments     []mcpRegistryArgument `json:"packageArguments"`
	EnvironmentVariables []mcpRegistryKeyValue `json:"environmentVariables"`
	Transport            mcpRegistryTransport  `json:"transport"`
}

type mcpRegistryTransport struct {
	Type      string                         `json:"type"`
	URL       string                         `json:"url"`
	Headers   []mcpRegistryKeyValue          `json:"headers"`
	Variables map[string]mcpRegistryKeyValue `json:"variables"`
}

type mcpRegistryKeyValue struct {
	Name        string   `json:"name"`
	Value       string   `json:"value"`
	Default     string   `json:"default"`
	Description string   `json:"description"`
	IsRequired  bool     `json:"isRequired"`
	IsSecret    bool     `json:"isSecret"`
	Choices     []string `json:"choices"`
}

type mcpRegistryArgument struct {
	mcpRegistryKeyValue
	Type      string                         `json:"type"`
	ValueHint string                         `json:"valueHint"`
	Variables map[string]mcpRegistryKeyValue `json:"variables"`
}

func workspaceMCPRegistrySearch(raw json.RawMessage) (mcpRegistrySearchSnapshot, error) {
	var payload struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return mcpRegistrySearchSnapshot{}, err
	}
	return searchMCPRegistry(payload.Query, payload.Limit)
}

func workspaceMCPRegistryInstall(raw json.RawMessage) (mcpRegistryInstallResult, error) {
	var payload struct {
		Config         mcpServerConfig `json:"config"`
		Server         mcpServerConfig `json:"server"`
		InstallCommand []string        `json:"installCommand"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return mcpRegistryInstallResult{}, err
	}
	config := payload.Config
	if config.Name == "" {
		config = payload.Server
	}
	command, err := normalizeMCPRegistryInstallCommand(payload.InstallCommand)
	if err != nil {
		return mcpRegistryInstallResult{}, err
	}
	output := mcpRegistryCommandOutput{}
	if len(command) > 0 {
		output, err = runMCPRegistryInstallCommand(command)
		if err != nil {
			return mcpRegistryInstallResult{}, err
		}
	}
	savePayload, err := json.Marshal(struct {
		Server mcpServerConfig `json:"server"`
	}{Server: config})
	if err != nil {
		return mcpRegistryInstallResult{}, err
	}
	saveResult, err := workspaceMCPSave(savePayload)
	if err != nil {
		return mcpRegistryInstallResult{}, err
	}
	return mcpRegistryInstallResult{
		mcpSaveResult:  saveResult,
		InstallCommand: command,
		CWD:            output.CWD,
		Stdout:         output.Stdout,
		Stderr:         output.Stderr,
	}, nil
}

func searchMCPRegistry(query string, limit int) (mcpRegistrySearchSnapshot, error) {
	normalizedQuery := strings.TrimSpace(query)
	if len(normalizedQuery) < 2 {
		return mcpRegistrySearchSnapshot{
			Query:   normalizedQuery,
			Servers: []mcpRegistrySearchResult{},
			Count:   0,
		}, nil
	}
	if limit <= 0 {
		limit = mcpRegistrySearchDefaultLimit
	}
	if limit > mcpRegistrySearchDefaultLimit {
		limit = mcpRegistrySearchDefaultLimit
	}

	base, err := url.Parse(mcpRegistryBaseURL)
	if err != nil {
		return mcpRegistrySearchSnapshot{}, err
	}
	endpoint := base.JoinPath("v0", "servers")
	values := endpoint.Query()
	values.Set("search", normalizedQuery)
	values.Set("version", "latest")
	values.Set("limit", fmt.Sprintf("%d", limit))
	endpoint.RawQuery = values.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return mcpRegistrySearchSnapshot{}, err
	}
	response, err := mcpRegistryHTTPClient.Do(request)
	if err != nil {
		return mcpRegistrySearchSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return mcpRegistrySearchSnapshot{}, fmt.Errorf("MCP registry search failed with status %d.", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return mcpRegistrySearchSnapshot{}, err
	}
	var payload mcpRegistryListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return mcpRegistrySearchSnapshot{}, err
	}

	results := make([]mcpRegistrySearchResult, 0, len(payload.Servers))
	for _, entry := range payload.Servers {
		result := mcpRegistrySearchResultFromResponse(entry)
		if result.ID != "" {
			results = append(results, result)
		}
	}
	return mcpRegistrySearchSnapshot{
		Query:   normalizedQuery,
		Servers: results,
		Count:   payload.Metadata.Count,
	}, nil
}

func mcpRegistrySearchResultFromResponse(entry mcpRegistryServerResponse) mcpRegistrySearchResult {
	server := entry.Server
	registryName := strings.TrimSpace(server.Name)
	if registryName == "" {
		return mcpRegistrySearchResult{}
	}
	displayName := strings.TrimSpace(server.Title)
	if displayName == "" {
		displayName = registryDisplayName(registryName)
	}
	status := strings.TrimSpace(entry.Meta.Official.Status)
	if status == "" {
		status = "active"
	}
	config, installCommand, notes, err := mcpConfigFromRegistryServer(server)
	result := mcpRegistrySearchResult{
		ID:                    registryName + "@" + strings.TrimSpace(server.Version),
		RegistryName:          registryName,
		Name:                  displayName,
		Description:           strings.TrimSpace(server.Description),
		Version:               strings.TrimSpace(server.Version),
		Status:                status,
		SourceURL:             firstNonEmptyMCPString(server.WebsiteURL, server.Repository.URL),
		RepositoryURL:         strings.TrimSpace(server.Repository.URL),
		WebsiteURL:            strings.TrimSpace(server.WebsiteURL),
		Installable:           err == nil,
		InstallReason:         "",
		ConfigurationNotes:    notes,
		RequiresConfiguration: len(notes) > 0,
	}
	if err != nil {
		result.InstallReason = err.Error()
		return result
	}
	result.Config = config
	result.ConfigName = config.Name
	result.Transport = config.Transport
	result.Command = config.Command
	result.Args = config.Args
	result.URL = config.URL
	result.InstallCommand = installCommand
	return result
}

func mcpConfigFromRegistryServer(server mcpRegistryServerJSON) (*mcpServerConfig, []string, []string, error) {
	if config, notes, ok := mcpConfigFromRegistryRemote(server); ok {
		return config, nil, notes, nil
	}
	if config, installCommand, notes, ok := mcpConfigFromRegistryPackage(server); ok {
		return config, installCommand, notes, nil
	}
	return nil, nil, nil, errors.New("No directly installable remote or stdio package was found.")
}

func mcpConfigFromRegistryRemote(server mcpRegistryServerJSON) (*mcpServerConfig, []string, bool) {
	for _, remote := range server.Remotes {
		transportType := strings.ToLower(strings.TrimSpace(remote.Type))
		if transportType != "streamable-http" && transportType != "sse" {
			continue
		}
		remoteURL := strings.TrimSpace(remote.URL)
		if remoteURL == "" {
			continue
		}
		resolvedURL, notes := applyRegistryVariables(remoteURL, remote.Variables, "URL")
		if mcpRegistryPlaceholderRE.MatchString(resolvedURL) {
			notes = append(notes, "Replace URL placeholders.")
		}
		headers, headerNotes := registryKeyValuesToMap(remote.Headers, "header")
		notes = append(notes, headerNotes...)
		config := &mcpServerConfig{
			Name:      localMCPServerName(server.Name),
			Transport: mcpTransportHTTP,
			Providers: append([]mcpProviderID(nil), mcpRegistryDefaultAgent...),
			URL:       resolvedURL,
			Headers:   headers,
		}
		if normalized, err := normalizeMCPServerConfig(*config); err == nil {
			return &normalized, notes, true
		}
	}
	return nil, nil, false
}

func mcpConfigFromRegistryPackage(server mcpRegistryServerJSON) (*mcpServerConfig, []string, []string, bool) {
	for _, pkg := range server.Packages {
		if strings.ToLower(strings.TrimSpace(pkg.Transport.Type)) != "stdio" {
			continue
		}
		command, args, installCommand, notes, ok := commandForRegistryPackage(pkg)
		if !ok {
			continue
		}
		env, envNotes := registryKeyValuesToMap(pkg.EnvironmentVariables, "environment variable")
		notes = append(notes, envNotes...)
		config := &mcpServerConfig{
			Name:      localMCPServerName(server.Name),
			Transport: mcpTransportStdio,
			Providers: append([]mcpProviderID(nil), mcpRegistryDefaultAgent...),
			Command:   command,
			Args:      args,
			Env:       env,
		}
		if normalized, err := normalizeMCPServerConfig(*config); err == nil {
			return &normalized, installCommand, notes, true
		}
	}
	return nil, nil, nil, false
}

func commandForRegistryPackage(pkg mcpRegistryPackage) (string, []string, []string, []string, bool) {
	registryType := strings.ToLower(strings.TrimSpace(pkg.RegistryType))
	identifier := strings.TrimSpace(pkg.Identifier)
	if identifier == "" {
		return "", nil, nil, nil, false
	}

	command := strings.TrimSpace(pkg.RuntimeHint)
	if command == "" {
		switch registryType {
		case "npm":
			command = "npx"
		case "pypi":
			command = "uvx"
		default:
			return "", nil, nil, nil, false
		}
	}

	args, notes := registryArgumentsToList(pkg.RuntimeArguments, "runtime argument")
	switch registryType {
	case "npm":
		if strings.EqualFold(command, "npx") && !registryArgsContainNPXYes(args) {
			args = append([]string{"-y"}, args...)
		}
		if baseURL := strings.TrimSpace(pkg.RegistryBaseURL); baseURL != "" && baseURL != "https://registry.npmjs.org" {
			args = append(args, "--registry", baseURL)
		}
		args = append(args, npmPackageSpec(identifier, pkg.Version))
	case "pypi":
		args = append(args, pypiPackageSpec(identifier, pkg.Version))
	default:
		args = append(args, identifier)
	}

	packageArgs, packageNotes := registryArgumentsToList(pkg.PackageArguments, "package argument")
	notes = append(notes, packageNotes...)
	args = append(args, packageArgs...)
	return command, normalizeStringList(args), installCommandForRegistryPackage(pkg), notes, true
}

func installCommandForRegistryPackage(pkg mcpRegistryPackage) []string {
	registryType := strings.ToLower(strings.TrimSpace(pkg.RegistryType))
	identifier := strings.TrimSpace(pkg.Identifier)
	if identifier == "" {
		return nil
	}
	switch registryType {
	case "npm":
		command := []string{npmCommandName(), "cache", "add", npmPackageSpec(identifier, pkg.Version)}
		if baseURL := strings.TrimSpace(pkg.RegistryBaseURL); baseURL != "" && baseURL != "https://registry.npmjs.org" {
			command = append(command, "--registry", baseURL)
		}
		return command
	case "pypi":
		return []string{uvCommandName(), "tool", "install", pypiPackageSpec(identifier, pkg.Version)}
	default:
		return nil
	}
}

func registryArgumentsToList(arguments []mcpRegistryArgument, label string) ([]string, []string) {
	out := []string{}
	notes := []string{}
	for _, argument := range arguments {
		name := strings.TrimSpace(argument.Name)
		value := firstNonEmptyMCPString(argument.Value, argument.Default)
		if value != "" {
			value, notes = applyRegistryArgumentVariables(value, argument.Variables, label, notes)
			if mcpRegistryPlaceholderRE.MatchString(value) {
				notes = append(notes, fmt.Sprintf("Replace placeholder in %s.", label))
			}
		}
		if name != "" {
			flag := registryArgumentName(argument)
			if value == "" {
				if argument.IsRequired {
					notes = append(notes, fmt.Sprintf("Set %s %s.", label, flag))
				}
				continue
			}
			out = append(out, flag, value)
			continue
		}
		if value != "" {
			out = append(out, value)
		} else if argument.IsRequired {
			notes = append(notes, fmt.Sprintf("Set required %s.", label))
		}
	}
	return out, notes
}

func registryArgumentName(argument mcpRegistryArgument) string {
	name := strings.TrimSpace(argument.Name)
	if strings.EqualFold(strings.TrimSpace(argument.Type), "named") && !strings.HasPrefix(name, "-") {
		return "--" + name
	}
	return name
}

func registryKeyValuesToMap(values []mcpRegistryKeyValue, label string) (map[string]string, []string) {
	out := map[string]string{}
	notes := []string{}
	for _, item := range values {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		value := firstNonEmptyMCPString(item.Value, item.Default)
		if value == "" {
			if item.IsRequired {
				notes = append(notes, fmt.Sprintf("Set %s %s.", label, name))
			}
			continue
		}
		if mcpRegistryPlaceholderRE.MatchString(value) {
			notes = append(notes, fmt.Sprintf("Replace placeholder in %s %s.", label, name))
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil, notes
	}
	return out, notes
}

func applyRegistryVariables(value string, variables map[string]mcpRegistryKeyValue, label string) (string, []string) {
	notes := []string{}
	for name, input := range variables {
		key := "{" + strings.TrimSpace(name) + "}"
		if !strings.Contains(value, key) {
			continue
		}
		replacement := firstNonEmptyMCPString(input.Value, input.Default)
		if replacement == "" {
			notes = append(notes, fmt.Sprintf("Set %s variable %s.", label, name))
			continue
		}
		value = strings.ReplaceAll(value, key, replacement)
	}
	return value, notes
}

func applyRegistryArgumentVariables(value string, variables map[string]mcpRegistryKeyValue, label string, notes []string) (string, []string) {
	for name, input := range variables {
		key := "{" + strings.TrimSpace(name) + "}"
		if !strings.Contains(value, key) {
			continue
		}
		replacement := firstNonEmptyMCPString(input.Value, input.Default)
		if replacement == "" {
			notes = append(notes, fmt.Sprintf("Set %s variable %s.", label, name))
			continue
		}
		value = strings.ReplaceAll(value, key, replacement)
	}
	return value, notes
}

func registryArgsContainNPXYes(args []string) bool {
	for _, arg := range args {
		if mcpRegistryNPXYesArgRE.MatchString(strings.TrimSpace(arg)) {
			return true
		}
	}
	return false
}

func npmPackageSpec(identifier string, version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "latest" {
		return strings.TrimSpace(identifier)
	}
	return strings.TrimSpace(identifier) + "@" + version
}

func pypiPackageSpec(identifier string, version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "latest" {
		return strings.TrimSpace(identifier)
	}
	return strings.TrimSpace(identifier) + "==" + version
}

func registryDisplayName(registryName string) string {
	name := strings.TrimSpace(registryName)
	if slash := strings.LastIndex(name, "/"); slash >= 0 && slash < len(name)-1 {
		name = name[slash+1:]
	}
	return name
}

func localMCPServerName(registryName string) string {
	name := registryDisplayName(registryName)
	name = mcpRegistryNameSafeRE.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-_")
	if name == "" {
		name = "mcp-server"
	}
	if len(name) > 64 {
		name = strings.Trim(name[:64], ".-_")
	}
	if name == "" || !mcpServerNamePattern.MatchString(name) {
		return "mcp-server"
	}
	return name
}
