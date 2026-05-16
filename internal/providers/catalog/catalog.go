package catalog

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
	DefaultCodexReasoningEffort  = "high"
	ServiceTierFast              = "fast"
)

func ServerProviders() []ProviderCatalogEntry {
	return cloneProviders(serverProviders)
}

func Get(providerID string) (ProviderCatalogEntry, bool) {
	for _, provider := range serverProviders {
		if provider.ID == providerID {
			return cloneProvider(provider), true
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

func NormalizeModel(providerID string, modelID string) string {
	return NormalizeServerModel(providerID, modelID)
}

func NormalizeServerModel(providerID string, modelID string) string {
	provider := GetOrDefault(providerID)
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
		DefaultModel:     "gpt-5.5",
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
		cloned = append(cloned, cloneProvider(provider))
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
