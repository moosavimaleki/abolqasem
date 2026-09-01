package custom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type Config struct {
	ID      string
	Name    string
	BaseURL string
	WireAPI string
	Headers map[string]string
	Models  []Model
}

type Client struct {
	HTTPClient *http.Client
}

func (c Client) Discover(ctx context.Context, config Config, apiKey string) ([]Model, error) {
	endpoint, err := modelEndpoint(config.BaseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(req, config.Headers, apiKey)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover provider models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("discover provider models: HTTP %d", response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode provider model catalog: %w", err)
	}
	models := make([]Model, 0, len(payload.Data))
	seen := map[string]struct{}{}
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, Model{ID: id, UpstreamID: id})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func ResolveModel(config Config, uiModelID string) (Model, error) {
	uiModelID = strings.TrimSpace(uiModelID)
	if uiModelID == "" {
		return Model{}, errors.New("model ID is required")
	}
	for _, model := range config.Models {
		if model.ID != uiModelID {
			continue
		}
		if strings.TrimSpace(model.UpstreamID) == "" {
			return Model{}, fmt.Errorf("model %q has no upstream ID", uiModelID)
		}
		return model, nil
	}
	// No mapping means exact passthrough, intentionally unlike Manager mode.
	return Model{ID: uiModelID, UpstreamID: uiModelID}, nil
}

// Validate rejects ambiguous mappings before a provider is saved. An empty
// Models list deliberately remains valid: callers may use passthrough IDs.
func Validate(config Config) error {
	if strings.TrimSpace(config.ID) == "" || strings.TrimSpace(config.Name) == "" {
		return errors.New("custom provider ID and name are required")
	}
	if _, err := modelEndpoint(config.BaseURL); err != nil {
		return err
	}
	if wireAPI := strings.TrimSpace(strings.ToLower(config.WireAPI)); wireAPI != "" && wireAPI != "responses" {
		return errors.New("custom provider must use the Responses API")
	}
	seenUI := map[string]struct{}{}
	seenUpstream := map[string]struct{}{}
	for _, model := range config.Models {
		model.ID = strings.TrimSpace(model.ID)
		model.UpstreamID = strings.TrimSpace(model.UpstreamID)
		if model.ID == "" || model.UpstreamID == "" {
			return errors.New("custom provider model mappings require both IDs")
		}
		if _, exists := seenUI[model.ID]; exists {
			return fmt.Errorf("duplicate UI model ID %q", model.ID)
		}
		if _, exists := seenUpstream[model.UpstreamID]; exists {
			return fmt.Errorf("duplicate upstream model ID %q", model.UpstreamID)
		}
		seenUI[model.ID] = struct{}{}
		seenUpstream[model.UpstreamID] = struct{}{}
		for _, effort := range model.ReasoningEfforts {
			effort = strings.TrimSpace(strings.ToLower(effort))
			switch effort {
			case "low", "medium", "high", "xhigh", "max":
			default:
				return fmt.Errorf("unsupported reasoning effort %q for model %q", effort, model.ID)
			}
		}
	}
	return nil
}

func modelEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("custom provider base URL must be HTTP(S)")
	}
	parsed.Path = path.Join(parsed.Path, "models")
	return parsed.String(), nil
}

func applyHeaders(request *http.Request, headers map[string]string, apiKey string) {
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			request.Header.Set(key, value)
		}
	}
	if strings.TrimSpace(apiKey) != "" && request.Header.Get("Authorization") == "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
}
