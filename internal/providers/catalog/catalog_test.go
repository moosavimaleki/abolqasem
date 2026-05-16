package catalog

import "testing"

func TestServerProvidersMatchesKannaCodexModels(t *testing.T) {
	providers := ServerProviders()
	var codex ProviderCatalogEntry
	for _, provider := range providers {
		if provider.ID == "codex" {
			codex = provider
			break
		}
	}
	if codex.ID == "" {
		t.Fatal("expected codex provider")
	}
	if codex.DefaultModel != "gpt-5.5" {
		t.Fatalf("expected default model gpt-5.5, got %q", codex.DefaultModel)
	}
	expected := []string{"gpt-5.5", "gpt-5.4", "gpt-5.3-codex", "gpt-5.3-codex-spark"}
	if len(codex.Models) != len(expected) {
		t.Fatalf("expected models %#v, got %#v", expected, codex.Models)
	}
	for index := range expected {
		if codex.Models[index].ID != expected[index] {
			t.Fatalf("expected models %#v, got %#v", expected, codex.Models)
		}
	}
}

func TestNormalizeModelUsesAliasesAndSafeFallback(t *testing.T) {
	if got := NormalizeModel("claude", "sonnet"); got != "claude-sonnet-4-6" {
		t.Fatalf("expected claude sonnet alias, got %q", got)
	}
	if got := NormalizeModel("unknown", "missing"); got != "gpt-5.5" {
		t.Fatalf("expected safe codex fallback, got %q", got)
	}
}
