package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
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
	var failures []string
	if apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); apiKey != "" {
		models, err := discoverAnthropicAPIModels(ctx, apiKey)
		if len(models) > 0 {
			return models, nil
		}
		if err != nil {
			failures = append(failures, "Anthropic API: "+err.Error())
		}
	} else {
		failures = append(failures, "Anthropic API: ANTHROPIC_API_KEY is not set")
	}

	models, err := discoverCLIModels(ctx, "claude", [][]string{
		{"claude", "models", "list", "--format", "json"},
		{"claude", "model", "list", "--format", "json"},
		{"claude", "models", "list"},
		{"claude", "--list-models"},
	})
	if len(models) > 0 {
		return models, nil
	}
	if err != nil {
		failures = append(failures, "Claude CLI: "+err.Error())
	}
	return nil, errors.New(strings.Join(failures, "; "))
}

func discoverGeminiModels(ctx context.Context) ([]ProviderModelOption, error) {
	var failures []string
	if apiKey := firstEnv("GEMINI_API_KEY", "GOOGLE_API_KEY"); apiKey != "" {
		models, err := discoverGeminiAPIModels(ctx, apiKey)
		if len(models) > 0 {
			return models, nil
		}
		if err != nil {
			failures = append(failures, "Gemini API: "+err.Error())
		}
	} else {
		failures = append(failures, "Gemini API: GEMINI_API_KEY/GOOGLE_API_KEY is not set")
	}

	models, err := discoverCLIModels(ctx, "gemini", [][]string{
		{"gemini", "models", "list", "--format", "json"},
		{"gemini", "models", "list", "--json"},
		{"gemini", "models", "list"},
		{"gemini", "--list-models"},
	})
	if len(models) > 0 {
		return models, nil
	}
	if err != nil {
		failures = append(failures, "Gemini CLI: "+err.Error())
	}
	return nil, errors.New(strings.Join(failures, "; "))
}

func discoverAnthropicAPIModels(ctx context.Context, apiKey string) ([]ProviderModelOption, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models?limit=1000", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-api-key", apiKey)
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

func discoverCLIModels(ctx context.Context, provider string, commands [][]string) ([]ProviderModelOption, error) {
	var failures []string
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		cmd.Env = os.Environ()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		models := parseModelListOutput(provider, stdout.Bytes())
		if len(models) > 0 {
			return models, nil
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" && err != nil {
			message = err.Error()
		}
		if message != "" {
			failures = append(failures, strings.Join(command, " ")+": "+message)
		}
	}
	if len(failures) == 0 {
		return nil, errors.New("no model list command returned models")
	}
	return nil, errors.New(strings.Join(failures, "; "))
}

func parseModelListOutput(provider string, data []byte) []ProviderModelOption {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	var raw any
	if json.Unmarshal(data, &raw) == nil {
		models := collectModelOptions(provider, raw)
		if len(models) > 0 {
			return models
		}
	}
	return modelOptionsFromText(provider, string(data))
}

func collectModelOptions(provider string, raw any) []ProviderModelOption {
	out := []ProviderModelOption{}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			if model := modelOptionFromMap(provider, typed); model.ID != "" {
				out = append(out, model)
				return
			}
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(raw)
	return out
}

func modelOptionFromMap(provider string, item map[string]any) ProviderModelOption {
	if provider == "gemini" {
		if methods, ok := stringSliceField(item, "supportedGenerationMethods"); ok && len(methods) > 0 && !containsString(methods, "generateContent") {
			return ProviderModelOption{}
		}
	}
	id := firstNonEmpty(
		stringMapField(item, "id"),
		stringMapField(item, "model"),
		stringMapField(item, "baseModelId"),
		strings.TrimPrefix(stringMapField(item, "name"), "models/"),
	)
	if !providerModelIDLooksValid(provider, id) {
		return ProviderModelOption{}
	}
	return ProviderModelOption{
		ID:    id,
		Label: firstNonEmpty(stringMapField(item, "displayName"), stringMapField(item, "display_name"), stringMapField(item, "label"), id),
	}
}

func modelOptionsFromText(provider string, text string) []ProviderModelOption {
	pattern := providerModelPattern(provider)
	matches := pattern.FindAllString(text, -1)
	out := make([]ProviderModelOption, 0, len(matches))
	seen := map[string]bool{}
	for _, id := range matches {
		if seen[id] || !providerModelIDLooksValid(provider, id) {
			continue
		}
		seen[id] = true
		out = append(out, ProviderModelOption{ID: id, Label: id})
	}
	return out
}

func providerModelPattern(provider string) *regexp.Regexp {
	switch provider {
	case "claude":
		return regexp.MustCompile(`claude-[a-zA-Z0-9_.-]+`)
	case "gemini":
		return regexp.MustCompile(`gemini-[a-zA-Z0-9_.-]+|auto`)
	default:
		return regexp.MustCompile(`gpt-[a-zA-Z0-9_.-]+`)
	}
}

func providerModelIDLooksValid(provider string, id string) bool {
	id = strings.TrimSpace(id)
	switch provider {
	case "claude":
		return strings.HasPrefix(id, "claude-")
	case "codex":
		return strings.HasPrefix(id, "gpt-")
	case "gemini":
		return id == "auto" || strings.HasPrefix(id, "gemini-")
	default:
		return false
	}
}

func stringMapField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return strings.TrimSpace(value)
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

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
