package server

import (
	"reflect"
	"testing"

	"abolqasem/internal/state"
)

func TestManagerModeKeepsNativeCodexCatalogWithoutAliases(t *testing.T) {
	settings := state.DefaultAppSettings()
	native := workspaceAvailableProvidersForSettings(settings)
	settings.CodexBackend.Mode = state.CodexBackendManager
	settings.CodexBackend.Enabled = true
	manager := workspaceAvailableProvidersForSettings(settings)
	if !reflect.DeepEqual(native, manager) {
		t.Fatalf("manager mode must retain Codex catalog and not add aliases:\n native=%#v\nmanager=%#v", native, manager)
	}
}

func TestCustomProviderCatalogShowsOnlyConfiguredAliases(t *testing.T) {
	settings := state.DefaultAppSettings()
	settings.CodexBackend.Mode = state.CodexBackendCustom
	settings.CodexBackend.Enabled = true
	settings.CodexBackend.CustomProviderID = "remote"
	settings.CodexBackend.CustomProviders["remote"] = state.CustomProviderSettings{Models: []state.CustomProviderModelSettings{{
		ID: "friendly", UpstreamID: "provider-model", DisplayName: "Friendly", ReasoningEfforts: []string{"high", "xhigh"},
	}}}
	providers := workspaceAvailableProvidersForSettings(settings)
	for _, provider := range providers {
		if provider.ID != "codex" {
			continue
		}
		if len(provider.Models) != 1 || provider.Models[0].ID != "friendly" || provider.DefaultModel != "friendly" {
			t.Fatalf("custom model aliases missing: %#v", provider)
		}
		if len(provider.Efforts) != 2 || provider.Efforts[1].ID != "xhigh" {
			t.Fatalf("custom effort metadata missing: %#v", provider.Efforts)
		}
		return
	}
	t.Fatal("codex provider not found")
}
