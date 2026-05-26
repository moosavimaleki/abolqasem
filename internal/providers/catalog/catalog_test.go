package catalog

import (
	"context"
	"sync"
	"testing"
)

func withCodexRuntimeProbe(t *testing.T, info CodexRuntimeInfo) {
	t.Helper()
	oldProbe := codexRuntimeProbe
	oldInfo := codexRuntimeInfo
	codexRuntimeProbe = func(context.Context) CodexRuntimeInfo {
		return info
	}
	codexRuntimeOnce = sync.Once{}
	codexRuntimeInfo = CodexRuntimeInfo{}
	t.Cleanup(func() {
		codexRuntimeProbe = oldProbe
		codexRuntimeOnce = sync.Once{}
		codexRuntimeInfo = oldInfo
	})
}

func TestServerProvidersExposeCodexRuntimeModelsWithRuntimeDefault(t *testing.T) {
	withCodexRuntimeProbe(t, CodexRuntimeInfo{
		Available:              true,
		Version:                "0.98.0",
		DefaultModel:           "gpt-5.2-codex",
		DefaultReasoningEffort: "medium",
		Models: []ProviderModelOption{
			{ID: "gpt-5.2-codex", Label: "gpt-5.2-codex", SupportsEffort: true},
			{ID: "gpt-5.4", Label: "gpt-5.4", SupportsEffort: true},
			{ID: "gpt-5.4-mini", Label: "GPT-5.4-Mini", SupportsEffort: true},
		},
		SupportsGPT55: false,
	})

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
	if codex.DefaultModel != "gpt-5.2-codex" {
		t.Fatalf("expected runtime default model gpt-5.2-codex, got %q", codex.DefaultModel)
	}
	if codex.DefaultEffort != "medium" {
		t.Fatalf("expected runtime default effort medium, got %q", codex.DefaultEffort)
	}
	expected := []string{"gpt-5.2-codex", "gpt-5.4", "gpt-5.4-mini"}
	if len(codex.Models) != len(expected) {
		t.Fatalf("expected models %#v, got %#v", expected, codex.Models)
	}
	for index := range expected {
		if codex.Models[index].ID != expected[index] {
			t.Fatalf("expected models %#v, got %#v", expected, codex.Models)
		}
	}
}

func TestServerProvidersUsesAbolqasemDefaultForSupportedCodexCLI(t *testing.T) {
	withCodexRuntimeProbe(t, CodexRuntimeInfo{Available: true, Version: "0.124.0", SupportsGPT55: true})

	codex := GetOrDefault("codex")
	if codex.DefaultModel != "gpt-5.5" {
		t.Fatalf("expected Abolqasem default model gpt-5.5, got %q", codex.DefaultModel)
	}
}

func TestServerProvidersExposeGemini(t *testing.T) {
	gemini, ok := Get("gemini")
	if !ok {
		t.Fatal("expected gemini provider")
	}
	if gemini.DefaultModel == "" || len(gemini.Models) == 0 {
		t.Fatalf("expected gemini defaults, got %#v", gemini)
	}
}

func TestNormalizeModelUsesAliasesAndSafeFallback(t *testing.T) {
	withCodexRuntimeProbe(t, CodexRuntimeInfo{})

	if got := NormalizeModel("claude", "sonnet"); got != "claude-sonnet-4-6" {
		t.Fatalf("expected claude sonnet alias, got %q", got)
	}
	if got := NormalizeServerModel("codex", "gpt-5-codex"); got != "gpt-5.3-codex" {
		t.Fatalf("expected codex alias, got %q", got)
	}
	if got := NormalizeModel("unknown", "missing"); got != "gpt-5.5" {
		t.Fatalf("expected safe static codex fallback, got %q", got)
	}
	if got := NormalizeModel("gemini", "gemini-custom-pro"); got != "gemini-custom-pro" {
		t.Fatalf("expected custom gemini model passthrough, got %q", got)
	}
}

func TestNormalizeModelUsesRuntimeFallback(t *testing.T) {
	withCodexRuntimeProbe(t, CodexRuntimeInfo{
		Available:    true,
		DefaultModel: "gpt-5.2-codex",
		Models: []ProviderModelOption{
			{ID: "gpt-5.2-codex", Label: "gpt-5.2-codex", SupportsEffort: true},
		},
	})
	if got := NormalizeModel("unknown", "missing"); got != "gpt-5.2-codex" {
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
