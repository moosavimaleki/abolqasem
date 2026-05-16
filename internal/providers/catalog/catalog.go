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
	provider := GetOrDefault(providerID)
	for _, model := range provider.Models {
		if model.ID == modelID {
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
