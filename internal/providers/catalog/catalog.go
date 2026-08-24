package catalog

import (
	"abolqasem/internal/appinfo"
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	codexprotocol "abolqasem/internal/providers/codex/protocol"
	codexrpc "abolqasem/internal/providers/codex/rpc"
	"abolqasem/internal/providers/providerexec"
)

type ProviderCatalogEntry struct {
	ID               string                 `json:"id"`
	Label            string                 `json:"label"`
	DefaultModel     string                 `json:"defaultModel"`
	DefaultEffort    string                 `json:"defaultEffort,omitempty"`
	SupportsPlanMode bool                   `json:"supportsPlanMode"`
	Models           []ProviderModelOption  `json:"models"`
	Efforts          []ProviderEffortOption `json:"efforts"`
}

type ProviderModelOption struct {
	ID                         string                        `json:"id"`
	Label                      string                        `json:"label"`
	SupportsEffort             bool                          `json:"supportsEffort"`
	Aliases                    []string                      `json:"aliases,omitempty"`
	ContextWindowOptions       []ProviderContextWindowOption `json:"contextWindowOptions,omitempty"`
	SupportsMaxReasoningEffort bool                          `json:"supportsMaxReasoningEffort,omitempty"`
}

type ProviderEffortOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type ProviderContextWindowOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type ProviderModelInventoryByProvider map[string]ProviderModelInventory

type ProviderModelInventory struct {
	CatalogModels    []ProviderModelOption `json:"catalog_models"`
	DiscoveredModels []ProviderModelOption `json:"discovered_models"`
	CustomModels     []ProviderModelOption `json:"custom_models"`
	LastRefreshAt    string                `json:"last_refresh_at,omitempty"`
	LastError        string                `json:"last_error,omitempty"`
}

type ModelOptions struct {
	Claude *ClaudeModelOptionsPatch `json:"claude,omitempty"`
	Codex  *CodexModelOptionsPatch  `json:"codex,omitempty"`
}

type ClaudeModelOptionsPatch struct {
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ContextWindow   string `json:"contextWindow,omitempty"`
}

type CodexModelOptionsPatch struct {
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	FastMode        *bool  `json:"fastMode,omitempty"`
}

type ClaudeModelOptions struct {
	ReasoningEffort string `json:"reasoningEffort"`
	ContextWindow   string `json:"contextWindow"`
}

type CodexModelOptions struct {
	ReasoningEffort string `json:"reasoningEffort"`
	FastMode        bool   `json:"fastMode"`
}

const (
	DefaultClaudeReasoningEffort = "high"
	DefaultClaudeContextWindow   = "200k"
	DefaultCodexModel            = "gpt-5.5"
	CompatibleCodexModel         = "gpt-5.4"
	MinGPT55CodexCLIVersion      = "0.124.0"
	DefaultCodexReasoningEffort  = "high"
	ServiceTierFast              = "fast"
)

type CodexRuntimeInfo struct {
	Available              bool
	Version                string
	DefaultModel           string
	DefaultReasoningEffort string
	Models                 []ProviderModelOption
	SupportsGPT55          bool
	Error                  string
}

type codexRuntimeTransport struct {
	stdin io.WriteCloser
	mu    sync.Mutex
}

type codexInitializeResult struct {
	UserAgent string `json:"userAgent"`
}

type codexModelListResult struct {
	Data []codexRuntimeModelItem `json:"data"`
}

type codexRuntimeModelItem struct {
	ID                     string `json:"id"`
	Model                  string `json:"model"`
	DisplayName            string `json:"displayName"`
	Description            string `json:"description"`
	Upgrade                string `json:"upgrade"`
	DefaultReasoningEffort string `json:"defaultReasoningEffort"`
	IsDefault              bool   `json:"isDefault"`
}

var (
	codexRuntimeOnce  sync.Once
	codexRuntimeInfo  CodexRuntimeInfo
	codexRuntimeProbe = probeCodexRuntime
)

func ServerProviders() []ProviderCatalogEntry {
	return cloneProviders(serverProviders)
}

func ServerProvidersWithInventory(inventory ProviderModelInventoryByProvider) []ProviderCatalogEntry {
	providers := cloneProviders(serverProviders)
	for index := range providers {
		providers[index] = withModelInventory(providers[index], inventory[providers[index].ID])
	}
	return providers
}

func Get(providerID string) (ProviderCatalogEntry, bool) {
	for _, provider := range serverProviders {
		if provider.ID == providerID {
			return withRuntimeProviderDefaults(cloneProvider(provider)), true
		}
	}
	return ProviderCatalogEntry{}, false
}

func GetWithInventory(providerID string, inventory ProviderModelInventoryByProvider) (ProviderCatalogEntry, bool) {
	for _, provider := range serverProviders {
		if provider.ID == providerID {
			cloned := withRuntimeProviderDefaults(cloneProvider(provider))
			return withModelInventory(cloned, inventory[provider.ID]), true
		}
	}
	return ProviderCatalogEntry{}, false
}

func GetOrDefault(providerID string) ProviderCatalogEntry {
	if provider, ok := Get(providerID); ok {
		return provider
	}
	provider, _ := Get("codex")
	return provider
}

func GetOrDefaultWithInventory(providerID string, inventory ProviderModelInventoryByProvider) ProviderCatalogEntry {
	if provider, ok := GetWithInventory(providerID, inventory); ok {
		return provider
	}
	provider, _ := GetWithInventory("codex", inventory)
	return provider
}

func CodexRuntimeDefaultModel() string {
	runtime := DetectCodexRuntime()
	if runtime.Available && runtime.DefaultModel != "" {
		return runtime.DefaultModel
	}
	return DefaultCodexModel
}

func CodexRuntimeDefaultReasoningEffort() string {
	runtime := DetectCodexRuntime()
	if runtime.Available && IsCodexReasoningEffort(runtime.DefaultReasoningEffort) {
		return runtime.DefaultReasoningEffort
	}
	return DefaultCodexReasoningEffort
}

func DetectCodexRuntime() CodexRuntimeInfo {
	codexRuntimeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		codexRuntimeInfo = codexRuntimeProbe(ctx)
	})
	return codexRuntimeInfo
}

func probeCodexRuntime(ctx context.Context) CodexRuntimeInfo {
	cmd := exec.CommandContext(ctx, providerexec.ExecutableOrName("codex"), "app-server")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CodexRuntimeInfo{Error: strings.TrimSpace(err.Error())}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return CodexRuntimeInfo{Error: strings.TrimSpace(err.Error())}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return CodexRuntimeInfo{Error: strings.TrimSpace(err.Error())}
	}

	transport := &codexRuntimeTransport{stdin: stdin}
	client := codexrpc.NewClient(transport)
	if err := cmd.Start(); err != nil {
		return CodexRuntimeInfo{Error: strings.TrimSpace(err.Error())}
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(250 * time.Millisecond):
		}
	}()
	go scanCodexRuntimeStdout(stdout, client)
	go scanCodexRuntimeStderr(stderr, client)

	var initResult codexInitializeResult
	if err := client.Call(ctx, "initialize", codexprotocol.InitializeParams{
		ClientInfo: codexprotocol.ClientInfo{
			Name:    appinfo.Name,
			Title:   appinfo.DisplayName,
			Version: "dev",
		},
		Capabilities: codexprotocol.Capabilities{
			ExperimentalAPI: true,
		},
	}, &initResult); err != nil {
		return CodexRuntimeInfo{Available: true, Error: strings.TrimSpace(err.Error())}
	}
	if err := transport.notify("initialized"); err != nil {
		return CodexRuntimeInfo{Available: true, Error: strings.TrimSpace(err.Error())}
	}

	var listResult codexModelListResult
	if err := client.Call(ctx, "model/list", map[string]any{"limit": 100}, &listResult); err != nil {
		version := parseCodexVersion(initResult.UserAgent)
		return CodexRuntimeInfo{
			Available:     true,
			Version:       version,
			SupportsGPT55: compareVersions(version, MinGPT55CodexCLIVersion) >= 0,
			Error:         strings.TrimSpace(err.Error()),
		}
	}
	models, defaultModel, defaultEffort := normalizeCodexRuntimeModels(listResult.Data)
	version := parseCodexVersion(initResult.UserAgent)
	return CodexRuntimeInfo{
		Available:              true,
		Version:                version,
		DefaultModel:           defaultModel,
		DefaultReasoningEffort: defaultEffort,
		Models:                 models,
		SupportsGPT55:          hasProviderModel(models, DefaultCodexModel) || compareVersions(version, MinGPT55CodexCLIVersion) >= 0,
	}
}

func normalizeCodexRuntimeModels(items []codexRuntimeModelItem) ([]ProviderModelOption, string, string) {
	models := make([]ProviderModelOption, 0, len(items))
	defaultModel := ""
	defaultEffort := ""
	for _, item := range items {
		if strings.TrimSpace(item.Upgrade) != "" {
			continue
		}
		id := firstNonEmpty(item.ID, item.Model)
		if id == "" {
			continue
		}
		model := ProviderModelOption{
			ID:             id,
			Label:          firstNonEmpty(item.DisplayName, item.Model, item.ID),
			SupportsEffort: true,
		}
		if id == "gpt-5.3-codex" {
			model.Aliases = []string{"gpt-5-codex"}
		}
		models = append(models, model)
		if item.IsDefault && defaultModel == "" {
			defaultModel = id
			defaultEffort = item.DefaultReasoningEffort
		}
	}
	if len(models) == 0 {
		return nil, "", ""
	}
	if defaultModel == "" {
		defaultModel = models[0].ID
	}
	if !IsCodexReasoningEffort(defaultEffort) {
		defaultEffort = DefaultCodexReasoningEffort
	}
	return models, defaultModel, defaultEffort
}

func hasProviderModel(models []ProviderModelOption, modelID string) bool {
	for _, model := range models {
		if model.ID == modelID {
			return true
		}
	}
	return false
}

func scanCodexRuntimeStdout(stdout io.Reader, client *codexrpc.Client) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		_ = client.HandleMessage(append([]byte(nil), scanner.Bytes()...))
	}
}

func scanCodexRuntimeStderr(stderr io.Reader, client *codexrpc.Client) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			client.RecordStderr(line)
		}
	}
}

func (t *codexRuntimeTransport) Send(message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	payload := append(append([]byte(nil), message...), '\n')
	_, err := t.stdin.Write(payload)
	return err
}

func (t *codexRuntimeTransport) notify(method string) error {
	data, err := json.Marshal(map[string]any{"method": method})
	if err != nil {
		return err
	}
	return t.Send(data)
}

func parseCodexVersion(output string) string {
	match := regexp.MustCompile(`\d+(?:\.\d+){1,3}`).FindString(output)
	return strings.TrimSpace(match)
}

func compareVersions(left string, right string) int {
	leftParts := versionParts(left)
	rightParts := versionParts(right)
	for index := 0; index < len(leftParts) || index < len(rightParts); index++ {
		leftValue := 0
		rightValue := 0
		if index < len(leftParts) {
			leftValue = leftParts[index]
		}
		if index < len(rightParts) {
			rightValue = rightParts[index]
		}
		if leftValue > rightValue {
			return 1
		}
		if leftValue < rightValue {
			return -1
		}
	}
	return 0
}

func versionParts(version string) []int {
	rawParts := strings.Split(strings.TrimSpace(version), ".")
	parts := make([]int, 0, len(rawParts))
	for _, raw := range rawParts {
		value, err := strconv.Atoi(raw)
		if err != nil {
			value = 0
		}
		parts = append(parts, value)
	}
	return parts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func NormalizeModel(providerID string, modelID string) string {
	return NormalizeServerModel(providerID, modelID)
}

func NormalizeServerModel(providerID string, modelID string) string {
	provider, ok := Get(providerID)
	if !ok {
		provider = GetOrDefault(providerID)
		return provider.DefaultModel
	}
	return normalizeModelForProvider(provider, modelID)
}

func NormalizeServerModelWithInventory(providerID string, modelID string, inventory ProviderModelInventoryByProvider) string {
	provider, ok := GetWithInventory(providerID, inventory)
	if !ok {
		provider = GetOrDefaultWithInventory(providerID, inventory)
		return provider.DefaultModel
	}
	return normalizeModelForProvider(provider, modelID)
}

func normalizeModelForProvider(provider ProviderCatalogEntry, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	for _, model := range provider.Models {
		if model.ID == modelID || modelAliasMatches(provider.ID, model.ID, modelID) {
			return model.ID
		}
		for _, alias := range model.Aliases {
			if alias == modelID {
				return model.ID
			}
		}
	}
	if modelID != "" {
		return modelID
	}
	return provider.DefaultModel
}

func NormalizeClaudeModelOptions(model string, modelOptions *ModelOptions, legacyEffort string) ClaudeModelOptions {
	reasoningEffort := ""
	contextWindow := ""
	if modelOptions != nil && modelOptions.Claude != nil {
		reasoningEffort = modelOptions.Claude.ReasoningEffort
		contextWindow = modelOptions.Claude.ContextWindow
	}
	if !IsClaudeReasoningEffort(reasoningEffort) {
		if IsClaudeReasoningEffort(legacyEffort) {
			reasoningEffort = legacyEffort
		} else {
			reasoningEffort = DefaultClaudeReasoningEffort
		}
	}
	return ClaudeModelOptions{
		ReasoningEffort: reasoningEffort,
		ContextWindow:   NormalizeClaudeContextWindow(model, contextWindow),
	}
}

func NormalizeCodexModelOptions(modelOptions *ModelOptions, legacyEffort string) CodexModelOptions {
	reasoningEffort := ""
	fastMode := false
	if modelOptions != nil && modelOptions.Codex != nil {
		reasoningEffort = modelOptions.Codex.ReasoningEffort
		if modelOptions.Codex.FastMode != nil {
			fastMode = *modelOptions.Codex.FastMode
		}
	}
	if !IsCodexReasoningEffort(reasoningEffort) {
		if IsCodexReasoningEffort(legacyEffort) {
			reasoningEffort = legacyEffort
		} else {
			reasoningEffort = DefaultCodexReasoningEffort
		}
	}
	return CodexModelOptions{
		ReasoningEffort: reasoningEffort,
		FastMode:        fastMode,
	}
}

func CodexServiceTierFromModelOptions(modelOptions CodexModelOptions) string {
	if modelOptions.FastMode {
		return ServiceTierFast
	}
	return ""
}

func NormalizeClaudeContextWindow(model string, contextWindow string) string {
	for _, provider := range serverProviders {
		if provider.ID != "claude" {
			continue
		}
		for _, option := range provider.Models {
			if option.ID != model {
				continue
			}
			for _, candidate := range option.ContextWindowOptions {
				if candidate.ID == contextWindow {
					return contextWindow
				}
			}
			return DefaultClaudeContextWindow
		}
	}
	return DefaultClaudeContextWindow
}

func ResolveClaudeAPIModelID(model string, contextWindow string) string {
	if contextWindow == "1m" {
		return model + "[1m]"
	}
	return model
}

func IsClaudeReasoningEffort(value string) bool {
	switch value {
	case "low", "medium", "high", "max":
		return true
	default:
		return false
	}
}

func IsCodexReasoningEffort(value string) bool {
	switch value {
	case "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

var serverProviders = []ProviderCatalogEntry{
	{
		ID:               "claude",
		Label:            "Claude",
		DefaultModel:     "claude-sonnet-4-6",
		DefaultEffort:    "high",
		SupportsPlanMode: true,
		Models: []ProviderModelOption{
			{
				ID:                         "claude-opus-4-7",
				Label:                      "Opus 4.7",
				SupportsEffort:             true,
				Aliases:                    []string{"opus"},
				ContextWindowOptions:       []ProviderContextWindowOption{{ID: "200k", Label: "200k"}, {ID: "1m", Label: "1M"}},
				SupportsMaxReasoningEffort: true,
			},
			{
				ID:                   "claude-sonnet-4-6",
				Label:                "Sonnet 4.6",
				SupportsEffort:       true,
				Aliases:              []string{"sonnet"},
				ContextWindowOptions: []ProviderContextWindowOption{{ID: "200k", Label: "200k"}, {ID: "1m", Label: "1M"}},
			},
			{
				ID:             "claude-haiku-4-5-20251001",
				Label:          "Haiku 4.5",
				SupportsEffort: true,
				Aliases:        []string{"haiku"},
			},
		},
		Efforts: []ProviderEffortOption{
			{ID: "low", Label: "Low"},
			{ID: "medium", Label: "Medium"},
			{ID: "high", Label: "High"},
			{ID: "max", Label: "Max"},
		},
	},
	{
		ID:               "codex",
		Label:            "Codex",
		DefaultModel:     DefaultCodexModel,
		SupportsPlanMode: true,
		Models: []ProviderModelOption{
			{ID: "gpt-5.5", Label: "GPT-5.5", SupportsEffort: false},
			{ID: "gpt-5.4", Label: "GPT-5.4", SupportsEffort: false},
			{ID: "gpt-5.3-codex", Label: "GPT-5.3 Codex", SupportsEffort: false},
			{ID: "gpt-5.3-codex-spark", Label: "GPT-5.3 Codex Spark", SupportsEffort: false},
		},
		Efforts: []ProviderEffortOption{},
	},
}

func modelAliasMatches(providerID string, modelID string, alias string) bool {
	switch providerID {
	case "codex":
		return modelID == "gpt-5.3-codex" && alias == "gpt-5-codex"
	default:
		return false
	}
}

func cloneProviders(providers []ProviderCatalogEntry) []ProviderCatalogEntry {
	cloned := make([]ProviderCatalogEntry, 0, len(providers))
	for _, provider := range providers {
		cloned = append(cloned, withRuntimeProviderDefaults(cloneProvider(provider)))
	}
	return cloned
}

func cloneProvider(provider ProviderCatalogEntry) ProviderCatalogEntry {
	cloned := provider
	cloned.Models = append([]ProviderModelOption(nil), provider.Models...)
	for index := range cloned.Models {
		cloned.Models[index].Aliases = append([]string(nil), provider.Models[index].Aliases...)
		cloned.Models[index].ContextWindowOptions = append([]ProviderContextWindowOption(nil), provider.Models[index].ContextWindowOptions...)
	}
	cloned.Efforts = append([]ProviderEffortOption(nil), provider.Efforts...)
	return cloned
}

func NormalizeProviderModelOption(providerID string, model ProviderModelOption) (ProviderModelOption, bool) {
	model.ID = strings.TrimSpace(model.ID)
	model.Label = strings.TrimSpace(model.Label)
	if model.ID == "" {
		return ProviderModelOption{}, false
	}
	if model.Label == "" {
		model.Label = model.ID
	}
	model.Aliases = trimStrings(model.Aliases)
	switch providerID {
	case "claude":
		model.SupportsEffort = true
		if len(model.ContextWindowOptions) == 0 && claudeModelSupportsLargeContext(model.ID) {
			model.ContextWindowOptions = []ProviderContextWindowOption{{ID: "200k", Label: "200k"}, {ID: "1m", Label: "1M"}}
		}
		if strings.Contains(strings.ToLower(model.ID), "opus") {
			model.SupportsMaxReasoningEffort = true
		}
	case "codex":
		if model.ID == "gpt-5.3-codex" && len(model.Aliases) == 0 {
			model.Aliases = []string{"gpt-5-codex"}
		}
	}
	return model, true
}

func withModelInventory(provider ProviderCatalogEntry, inventory ProviderModelInventory) ProviderCatalogEntry {
	catalogModels := normalizeProviderModelOptions(provider.ID, inventory.CatalogModels)
	discovered := normalizeProviderModelOptions(provider.ID, inventory.DiscoveredModels)
	custom := normalizeProviderModelOptions(provider.ID, inventory.CustomModels)
	if len(catalogModels) > 0 {
		provider.Models = catalogModels
	} else if len(discovered) > 0 {
		provider.Models = discovered
	}
	provider.Models = mergeProviderModelOptions(provider.Models, custom)
	if len(provider.Models) > 0 && !hasProviderModel(provider.Models, provider.DefaultModel) {
		provider.DefaultModel = provider.Models[0].ID
	}
	return provider
}

func normalizeProviderModelOptions(providerID string, models []ProviderModelOption) []ProviderModelOption {
	out := make([]ProviderModelOption, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		normalized, ok := NormalizeProviderModelOption(providerID, model)
		if !ok || seen[normalized.ID] {
			continue
		}
		seen[normalized.ID] = true
		out = append(out, normalized)
	}
	return out
}

func mergeProviderModelOptions(base []ProviderModelOption, extra []ProviderModelOption) []ProviderModelOption {
	out := append([]ProviderModelOption(nil), base...)
	seen := map[string]int{}
	for index, model := range out {
		seen[model.ID] = index
	}
	for _, model := range extra {
		if existingIndex, ok := seen[model.ID]; ok {
			out[existingIndex] = model
			continue
		}
		seen[model.ID] = len(out)
		out = append(out, model)
	}
	return out
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func claudeModelSupportsLargeContext(modelID string) bool {
	modelID = strings.ToLower(modelID)
	return strings.Contains(modelID, "opus") || strings.Contains(modelID, "sonnet")
}

func withRuntimeProviderDefaults(provider ProviderCatalogEntry) ProviderCatalogEntry {
	if provider.ID == "codex" {
		runtime := DetectCodexRuntime()
		if runtime.Available {
			if len(runtime.Models) > 0 {
				provider.Models = append([]ProviderModelOption(nil), runtime.Models...)
			}
			if runtime.DefaultModel != "" {
				provider.DefaultModel = runtime.DefaultModel
			}
			if IsCodexReasoningEffort(runtime.DefaultReasoningEffort) {
				provider.DefaultEffort = runtime.DefaultReasoningEffort
			}
		}
	}
	return provider
}
