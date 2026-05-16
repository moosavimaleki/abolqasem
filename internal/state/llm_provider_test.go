package state

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

type stateRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn stateRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func withTempLlmProviderStateDir(t *testing.T) {
	t.Helper()
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })
}

func TestResolveLlmProviderBaseURL(t *testing.T) {
	if got := ResolveLlmProviderBaseURL("openai", ""); got != LlmProviderOpenAIBaseURL {
		t.Fatalf("expected OpenAI URL, got %q", got)
	}
	if got := ResolveLlmProviderBaseURL("openrouter", ""); got != LlmProviderOpenRouterBaseURL {
		t.Fatalf("expected OpenRouter URL, got %q", got)
	}
	if got := ResolveLlmProviderBaseURL("custom", " https://example.com/v1 "); got != "https://example.com/v1" {
		t.Fatalf("expected custom URL to be trimmed, got %q", got)
	}
}

func TestNormalizeLlmProviderSnapshotValidCustom(t *testing.T) {
	snapshot := NormalizeLlmProviderSnapshot(map[string]any{
		"provider": "custom",
		"apiKey":   "  test-key  ",
		"model":    "  gpt-test  ",
		"baseUrl":  " https://example.com/v1 ",
	}, "/tmp/llm-provider.json")

	if snapshot.Provider != "custom" || snapshot.APIKey != "test-key" || snapshot.Model != "gpt-test" {
		t.Fatalf("unexpected normalized snapshot: %#v", snapshot)
	}
	if snapshot.ResolvedBaseURL != "https://example.com/v1" {
		t.Fatalf("expected resolved custom URL, got %q", snapshot.ResolvedBaseURL)
	}
	if !snapshot.Enabled {
		t.Fatal("expected custom provider to be enabled")
	}
	if snapshot.Warning != nil {
		t.Fatalf("expected no warning, got %q", *snapshot.Warning)
	}
}

func TestNormalizeLlmProviderSnapshotInvalidCustom(t *testing.T) {
	snapshot := NormalizeLlmProviderSnapshot(map[string]any{
		"provider": "custom",
		"apiKey":   "test-key",
		"model":    "gpt-test",
		"baseUrl":  "",
	}, "/tmp/llm-provider.json")

	if snapshot.Enabled {
		t.Fatal("expected invalid custom provider to be disabled")
	}
	if snapshot.Warning == nil || !containsText(*snapshot.Warning, "custom provider requires a baseUrl") {
		t.Fatalf("expected custom baseUrl warning, got %#v", snapshot.Warning)
	}
}

func TestLoadLlmProviderSnapshotMissingAndInvalidJSON(t *testing.T) {
	withTempLlmProviderStateDir(t)

	missing, err := LoadLlmProviderSnapshot()
	if err != nil {
		t.Fatalf("LoadLlmProviderSnapshot returned error: %v", err)
	}
	if missing.Provider != "openai" || missing.Model != DefaultOpenAISDKModel || missing.Warning != nil {
		t.Fatalf("unexpected default snapshot: %#v", missing)
	}

	if err := os.WriteFile(GetLlmProviderFilePath(), []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write invalid provider file: %v", err)
	}
	invalid, err := LoadLlmProviderSnapshot()
	if err != nil {
		t.Fatalf("LoadLlmProviderSnapshot returned error: %v", err)
	}
	if invalid.Warning == nil || !containsText(*invalid.Warning, "invalid JSON") {
		t.Fatalf("expected invalid JSON warning, got %#v", invalid.Warning)
	}
}

func TestSaveLlmProviderSnapshotWritesNormalizedConfig(t *testing.T) {
	withTempLlmProviderStateDir(t)

	snapshot, err := SaveLlmProviderSnapshot(map[string]any{
		"provider": "openrouter",
		"apiKey":   " test-key ",
		"model":    " openrouter/model ",
		"baseUrl":  "ignored",
	})
	if err != nil {
		t.Fatalf("SaveLlmProviderSnapshot returned error: %v", err)
	}
	if snapshot.Provider != "openrouter" || snapshot.ResolvedBaseURL != LlmProviderOpenRouterBaseURL {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	data, err := os.ReadFile(GetLlmProviderFilePath())
	if err != nil {
		t.Fatalf("expected provider file to exist: %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("persisted provider file is invalid JSON: %v", err)
	}
	if persisted["baseUrl"] != nil {
		t.Fatalf("expected named provider baseUrl to be persisted as null, got %#v", persisted["baseUrl"])
	}
}

func TestValidateLlmProviderCredentialsConfigError(t *testing.T) {
	result := ValidateLlmProviderCredentials(map[string]any{
		"provider": "custom",
		"apiKey":   "test-key",
		"model":    "gpt-test",
		"baseUrl":  "",
	})
	if result.OK {
		t.Fatal("expected validation to fail")
	}
}

func TestValidateLlmProviderCredentialsUsesResponsesEndpoint(t *testing.T) {
	previousClient := llmProviderHTTPClient
	llmProviderHTTPClient = &http.Client{Transport: stateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/responses" {
			t.Fatalf("expected /v1/responses request, got %q", req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", req.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       http.NoBody,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	t.Cleanup(func() {
		llmProviderHTTPClient = previousClient
	})

	result := ValidateLlmProviderCredentials(map[string]any{
		"provider": "custom",
		"apiKey":   "test-key",
		"model":    "gpt-test",
		"baseUrl":  "https://example.com/v1",
	})
	if !result.OK || result.Error != nil {
		t.Fatalf("expected validation success, got %#v", result)
	}
}

func containsText(value string, needle string) bool {
	return strings.Contains(value, needle)
}
