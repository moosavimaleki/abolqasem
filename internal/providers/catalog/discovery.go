package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const providerModelDiscoveryTimeout = 8 * time.Second

type providerModelDiscovery struct {
	models []ProviderModelOption
	err    error
}

func DiscoverProviderModelInventory(ctx context.Context) ProviderModelInventoryByProvider {
	providers := []string{"claude", "codex", "gemini"}
	out := ProviderModelInventoryByProvider{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, provider := range providers {
		provider := provider
		wg.Add(1)
		go func() {
			defer wg.Done()
			providerCtx, cancel := context.WithTimeout(ctx, providerModelDiscoveryTimeout)
			defer cancel()
			discovery := discoverProviderModels(providerCtx, provider)
			inventory := ProviderModelInventory{
				LastRefreshAt: time.Now().UTC().Format(time.RFC3339),
			}
			if discovery.err != nil {
				inventory.LastError = discovery.err.Error()
			}
			if len(discovery.models) > 0 {
				inventory.DiscoveredModels = discovery.models
			}
			mu.Lock()
			out[provider] = inventory
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func discoverProviderModels(ctx context.Context, provider string) providerModelDiscovery {
	var models []ProviderModelOption
	var err error
	switch provider {
	case "claude":
		models, err = discoverClaudeModels(ctx)
	case "codex":
		models, err = discoverCodexModels(ctx)
	case "gemini":
		models, err = discoverGeminiModels(ctx)
	default:
		err = fmt.Errorf("unsupported provider: %s", provider)
	}
	if len(models) > 0 {
		return providerModelDiscovery{models: normalizeProviderModelOptions(provider, models)}
	}
	return providerModelDiscovery{err: err}
}

func discoverCodexModels(ctx context.Context) ([]ProviderModelOption, error) {
	runtime := probeCodexRuntime(ctx)
	if len(runtime.Models) > 0 {
		return runtime.Models, nil
	}
	if runtime.Error != "" {
		return nil, errors.New(runtime.Error)
	}
	return nil, errors.New("codex model list is unavailable")
}

func discoverClaudeModels(ctx context.Context) ([]ProviderModelOption, error) {
	if strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")) != "" {
		if !useClaudeGatewayModelDiscovery() {
			return sourceProviderModels("claude"), nil
		}
		endpoint, err := anthropicModelsEndpoint(os.Getenv("ANTHROPIC_BASE_URL"))
		if err != nil {
			return nil, err
		}
		models, err := discoverAnthropicModelsEndpoint(ctx, endpoint, os.Getenv("ANTHROPIC_API_KEY"))
		if len(models) > 0 {
			return models, nil
		}
		if err != nil {
			return nil, fmt.Errorf("Claude gateway /v1/models: %w", err)
		}
		return nil, errors.New("Claude gateway /v1/models returned no models")
	}

	if apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); apiKey != "" {
		models, err := discoverAnthropicAPIModels(ctx, apiKey)
		if len(models) > 0 {
			return models, nil
		}
		if err != nil {
			return nil, fmt.Errorf("Anthropic API: %w", err)
		}
		return nil, errors.New("Anthropic API returned no models")
	}

	return sourceProviderModels("claude"), nil
}

func discoverGeminiModels(ctx context.Context) ([]ProviderModelOption, error) {
	if apiKey := firstEnv("GEMINI_API_KEY", "GOOGLE_API_KEY"); apiKey != "" {
		models, err := discoverGeminiAPIModels(ctx, apiKey)
		if len(models) > 0 {
			return models, nil
		}
		if err != nil {
			return nil, fmt.Errorf("Gemini API: %w", err)
		}
		return nil, errors.New("Gemini API returned no models")
	}

	return sourceProviderModels("gemini"), nil
}

func discoverAnthropicAPIModels(ctx context.Context, apiKey string) ([]ProviderModelOption, error) {
	endpoint, err := anthropicModelsEndpoint("https://api.anthropic.com")
	if err != nil {
		return nil, err
	}
	return discoverAnthropicModelsEndpoint(ctx, endpoint, apiKey)
}

func discoverAnthropicModelsEndpoint(ctx context.Context, endpoint string, apiKey string) ([]ProviderModelOption, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		request.Header.Set("x-api-key", apiKey)
	}
	request.Header.Set("anthropic-version", "2023-06-01")
	request.Header.Set("accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with %s", response.Status)
	}
	var payload struct {
		Data []struct {
			ID               string `json:"id"`
			DisplayName      string `json:"display_name"`
			DisplayNameCamel string `json:"displayName"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	models := make([]ProviderModelOption, 0, len(payload.Data))
	for _, item := range payload.Data {
		if !providerModelIDLooksValid("claude", item.ID) {
			continue
		}
		label := firstNonEmpty(item.DisplayName, item.DisplayNameCamel, item.ID)
		models = append(models, ProviderModelOption{ID: item.ID, Label: label})
	}
	return models, nil
}

func anthropicModelsEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid ANTHROPIC_BASE_URL: %s", baseURL)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path += "/models"
	} else {
		parsed.Path += "/v1/models"
	}
	query := parsed.Query()
	if query.Get("limit") == "" {
		query.Set("limit", "1000")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func useClaudeGatewayModelDiscovery() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func discoverGeminiAPIModels(ctx context.Context, apiKey string) ([]ProviderModelOption, error) {
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1000&key=" + url.QueryEscape(apiKey)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with %s", response.Status)
	}
	var payload struct {
		Models []struct {
			Name                       string   `json:"name"`
			BaseModelID                string   `json:"baseModelId"`
			DisplayName                string   `json:"displayName"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	models := make([]ProviderModelOption, 0, len(payload.Models))
	for _, item := range payload.Models {
		if len(item.SupportedGenerationMethods) > 0 && !containsString(item.SupportedGenerationMethods, "generateContent") {
			continue
		}
		id := firstNonEmpty(item.BaseModelID, strings.TrimPrefix(item.Name, "models/"))
		if !providerModelIDLooksValid("gemini", id) {
			continue
		}
		models = append(models, ProviderModelOption{ID: id, Label: firstNonEmpty(item.DisplayName, id)})
	}
	return models, nil
}

func providerModelIDLooksValid(provider string, id string) bool {
	id = strings.TrimSpace(id)
	switch provider {
	case "claude":
		return strings.HasPrefix(id, "claude-")
	case "codex":
		return strings.HasPrefix(id, "gpt-")
	case "gemini":
		return id == "auto" || strings.HasPrefix(id, "gemini-") || strings.HasPrefix(id, "gemma-")
	default:
		return false
	}
}

func stringSliceField(item map[string]any, key string) ([]string, bool) {
	raw, ok := item[key]
	if !ok {
		return nil, false
	}
	switch typed := raw.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, value := range typed {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sourceProviderModels(providerID string) []ProviderModelOption {
	for _, provider := range serverProviders {
		if provider.ID == providerID {
			return cloneProvider(provider).Models
		}
	}
	return nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
