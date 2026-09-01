package server

import (
	"abolqasem/internal/providers/catalog"
	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
	"context"
	"strings"
	"time"
)

const providerModelRefreshInterval = 24 * time.Hour

func workspaceAvailableProviders() []catalog.ProviderCatalogEntry {
	settings, err := state.LoadSettings()
	if err != nil {
		return catalog.ServerProviders()
	}
	return workspaceAvailableProvidersForSettings(settings)
}

func workspaceAvailableProvidersForSettings(settings state.AppSettings) []catalog.ProviderCatalogEntry {
	settings = state.NormalizeSettings(settings)
	providerexec.SetConfiguredExecutables(settings.ProviderExecutables)
	providers := catalog.ServerProvidersWithInventory(settings.ProviderModelCatalog)
	for index := range providers {
		providers[index].Available = providerexec.Executable(providers[index].ID) != ""
	}
	if settings.CodexBackend.Mode != state.CodexBackendCustom || !settings.CodexBackend.Enabled {
		return providers
	}
	customProvider, ok := settings.CodexBackend.CustomProviders[settings.CodexBackend.CustomProviderID]
	if !ok {
		return providers
	}
	for index := range providers {
		if providers[index].ID != "codex" {
			continue
		}
		models := customProviderCatalogModels(customProvider.Models)
		if len(models) == 0 {
			return providers // passthrough remains intentionally available without discovery.
		}
		providers[index].Models = models
		providers[index].DefaultModel = models[0].ID
		providers[index].Efforts = customProviderCatalogEfforts(customProvider.Models)
		return providers
	}
	return providers
}

func customProviderCatalogModels(models []state.CustomProviderModelSettings) []catalog.ProviderModelOption {
	result := make([]catalog.ProviderModelOption, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(model.DisplayName)
		if label == "" {
			label = id
		}
		result = append(result, catalog.ProviderModelOption{ID: id, Label: label, SupportsEffort: len(model.ReasoningEfforts) > 0})
	}
	return result
}

func customProviderCatalogEfforts(models []state.CustomProviderModelSettings) []catalog.ProviderEffortOption {
	allowed := map[string]bool{}
	for _, model := range models {
		for _, effort := range model.ReasoningEfforts {
			effort = strings.ToLower(strings.TrimSpace(effort))
			if catalog.IsCodexReasoningEffort(effort) {
				allowed[effort] = true
			}
		}
	}
	options := make([]catalog.ProviderEffortOption, 0, len(allowed))
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		if allowed[effort] {
			options = append(options, catalog.ProviderEffortOption{ID: effort, Label: strings.ToUpper(effort[:1]) + effort[1:]})
		}
	}
	return options
}

func workspaceRefreshProviderModels(force bool) (state.AppSettings, error) {
	settings, err := state.LoadSettings()
	if err != nil {
		return settings, err
	}
	settings = state.NormalizeSettings(settings)
	if !force && !providerModelRefreshDue(settings.ProviderModelCatalog) {
		return settings, nil
	}

	discovered := catalog.DiscoverProviderModelInventory(context.Background())
	if settings.ProviderModelCatalog == nil {
		settings.ProviderModelCatalog = catalog.ProviderModelInventoryByProvider{}
	}
	for provider, next := range discovered {
		current := settings.ProviderModelCatalog[provider]
		if len(next.DiscoveredModels) > 0 {
			current.DiscoveredModels = next.DiscoveredModels
		}
		current.LastRefreshAt = next.LastRefreshAt
		current.LastError = next.LastError
		settings.ProviderModelCatalog[provider] = current
	}
	if err := state.SaveSettings(settings); err != nil {
		return settings, err
	}
	return state.NormalizeSettings(settings), nil
}

func providerModelRefreshDue(inventory catalog.ProviderModelInventoryByProvider) bool {
	for _, provider := range []string{"claude", "codex", "opencode"} {
		lastRefresh := strings.TrimSpace(inventory[provider].LastRefreshAt)
		if lastRefresh == "" {
			return true
		}
		parsed, err := time.Parse(time.RFC3339, lastRefresh)
		if err != nil || time.Since(parsed) > providerModelRefreshInterval {
			return true
		}
	}
	return false
}

func providerModelCatalogSnapshot(inventory catalog.ProviderModelInventoryByProvider) map[string]any {
	out := map[string]any{}
	for _, provider := range []string{"claude", "codex", "opencode"} {
		current := inventory[provider]
		out[provider] = map[string]any{
			"catalogModels":    nonNilProviderModelOptions(current.CatalogModels),
			"discoveredModels": nonNilProviderModelOptions(current.DiscoveredModels),
			"customModels":     nonNilProviderModelOptions(current.CustomModels),
			"lastRefreshAt":    current.LastRefreshAt,
			"lastError":        current.LastError,
		}
	}
	return out
}

func nonNilProviderModelOptions(models []catalog.ProviderModelOption) []catalog.ProviderModelOption {
	if models == nil {
		return []catalog.ProviderModelOption{}
	}
	return models
}
