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

func TestServerProvidersExcludeGeminiForKannaParity(t *testing.T) {
	for _, provider := range ServerProviders() {
		if provider.ID == "gemini" {
			t.Fatal("gemini must stay legacy-viewer-only for Kanna parity")
		}
	}
}

func TestNormalizeModelUsesAliasesAndSafeFallback(t *testing.T) {
	if got := NormalizeModel("claude", "sonnet"); got != "claude-sonnet-4-6" {
		t.Fatalf("expected claude sonnet alias, got %q", got)
	}
	if got := NormalizeServerModel("codex", "gpt-5-codex"); got != "gpt-5.3-codex" {
		t.Fatalf("expected codex alias, got %q", got)
	}
	if got := NormalizeModel("unknown", "missing"); got != "gpt-5.5" {
		t.Fatalf("expected safe codex fallback, got %q", got)
	}
}

func TestNormalizeClaudeModelOptions(t *testing.T) {
	if got := NormalizeClaudeModelOptions("claude-opus-4-7", nil, "max"); got != (ClaudeModelOptions{
		ReasoningEffort: "max",
		ContextWindow:   "200k",
	}) {
		t.Fatalf("unexpected legacy effort normalization: %#v", got)
	}

	got := NormalizeClaudeModelOptions("claude-sonnet-4-6", &ModelOptions{
		Claude: &ClaudeModelOptionsPatch{
			ReasoningEffort: "medium",
			ContextWindow:   "1m",
		},
	}, "")
	if got != (ClaudeModelOptions{ReasoningEffort: "medium", ContextWindow: "1m"}) {
		t.Fatalf("unexpected claude options: %#v", got)
	}

	got = NormalizeClaudeModelOptions("claude-haiku-4-5-20251001", &ModelOptions{
		Claude: &ClaudeModelOptionsPatch{
			ReasoningEffort: "medium",
			ContextWindow:   "1m",
		},
	}, "")
	if got.ReasoningEffort != "medium" || got.ContextWindow != "200k" {
		t.Fatalf("unexpected haiku options: %#v", got)
	}
}

func TestNormalizeCodexModelOptions(t *testing.T) {
	if got := NormalizeCodexModelOptions(nil, ""); got != (CodexModelOptions{
		ReasoningEffort: "high",
		FastMode:        false,
	}) {
		t.Fatalf("unexpected default codex options: %#v", got)
	}

	fastMode := true
	normalized := NormalizeCodexModelOptions(&ModelOptions{
		Codex: &CodexModelOptionsPatch{
			ReasoningEffort: "xhigh",
			FastMode:        &fastMode,
		},
	}, "")
	if normalized != (CodexModelOptions{ReasoningEffort: "xhigh", FastMode: true}) {
		t.Fatalf("unexpected codex options: %#v", normalized)
	}
	if got := CodexServiceTierFromModelOptions(normalized); got != "fast" {
		t.Fatalf("expected fast service tier, got %q", got)
	}
}

func TestResolveClaudeAPIModelID(t *testing.T) {
	if got := ResolveClaudeAPIModelID("claude-opus-4-7", "1m"); got != "claude-opus-4-7[1m]" {
		t.Fatalf("expected 1m model id, got %q", got)
	}
	if got := ResolveClaudeAPIModelID("claude-sonnet-4-6", "200k"); got != "claude-sonnet-4-6" {
		t.Fatalf("expected base model id, got %q", got)
	}
}
