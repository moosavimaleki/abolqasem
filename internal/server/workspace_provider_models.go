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
	return catalog.ServerProvidersWithInventory(settings.ProviderModelCatalog)
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
	for _, provider := range []string{"claude", "codex", "gemini"} {
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
	for _, provider := range []string{"claude", "codex", "gemini"} {
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
