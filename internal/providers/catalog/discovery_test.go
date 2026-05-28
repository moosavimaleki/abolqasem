package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverClaudeModelsUsesSourceCatalogWithoutAPI(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY", "")

	models, err := discoverClaudeModels(context.Background())
	if err != nil {
		t.Fatalf("expected source catalog models, got error: %v", err)
	}
	if !hasProviderModel(models, "claude-sonnet-4-6") {
		t.Fatalf("expected Claude source catalog models, got %#v", models)
	}
}

func TestDiscoverClaudeModelsUsesGatewayModelsWhenOptedIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "1000" {
			t.Fatalf("unexpected limit: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("expected x-api-key header, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Fatal("expected anthropic-version header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-gateway-model", "display_name": "Gateway Model"},
			},
		})
	}))
	defer server.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_BASE_URL", server.URL)
	t.Setenv("CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY", "1")

	models, err := discoverClaudeModels(context.Background())
	if err != nil {
		t.Fatalf("expected gateway models, got error: %v", err)
	}
	if len(models) != 1 || models[0].ID != "claude-gateway-model" || models[0].Label != "Gateway Model" {
		t.Fatalf("unexpected gateway models: %#v", models)
	}
}

func TestDiscoverGeminiModelsUsesSourceCatalogWithoutAPI(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	models, err := discoverGeminiModels(context.Background())
	if err != nil {
		t.Fatalf("expected source catalog models, got error: %v", err)
	}
	for _, modelID := range []string{
		"auto",
		"gemini-3.1-pro-preview",
		"gemini-3.1-flash-lite-preview",
		"gemini-2.5-flash-lite",
	} {
		if !hasProviderModel(models, modelID) {
			t.Fatalf("expected Gemini source catalog model %q, got %#v", modelID, models)
		}
	}
}
