package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"abolqasem/internal/codexmanager/storage"
	"abolqasem/internal/providers/custom"
	"abolqasem/internal/secrets"
	"abolqasem/internal/state"
)

func handleAPICustomProvider(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/custom-providers"), "/")
	switch action {
	case "":
		handleAPICustomProviderSave(w, r)
	case "test":
		handleAPICustomProviderTest(w, r)
	case "preview":
		handleAPICustomProviderPreview(w, r)
	default:
		http.NotFound(w, r)
	}
}

type customProviderRequest struct {
	Provider custom.Provider   `json:"provider"`
	Headers  map[string]string `json:"headers"`
	APIKey   string            `json:"apiKey,omitempty"`
	Discover bool              `json:"discover,omitempty"`
	ModelID  string            `json:"modelId,omitempty"`
}

func handleAPICustomProviderSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := decodeCustomProviderRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCustomProvider(payload.Provider, payload.Headers); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings, err := state.LoadSettings()
	if err != nil {
		http.Error(w, "Could not load settings", http.StatusInternalServerError)
		return
	}
	if settings.CodexBackend.CustomProviders == nil {
		settings.CodexBackend.CustomProviders = map[string]state.CustomProviderSettings{}
	}
	settings.CodexBackend.CustomProviders[payload.Provider.ID] = stateCustomProvider(payload.Provider, payload.Headers)
	if strings.TrimSpace(payload.APIKey) != "" {
		if err := secrets.Put(customProviderSecretName(payload.Provider.ID), payload.APIKey); err != nil {
			http.Error(w, "Could not store provider credential", http.StatusInternalServerError)
			return
		}
	}
	if err := state.SaveSettings(settings); err != nil {
		http.Error(w, "Could not save provider", http.StatusInternalServerError)
		return
	}
	writeJSON(w, customProviderSnapshot(state.NormalizeSettings(settings)))
}

func handleAPICustomProviderTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := decodeCustomProviderRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCustomProvider(payload.Provider, payload.Headers); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !payload.Discover {
		writeJSON(w, map[string]any{"ok": true, "models": payload.Provider.Models})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	models, err := (custom.Client{}).Discover(ctx, custom.Config{ID: payload.Provider.ID, Name: payload.Provider.Name, BaseURL: payload.Provider.BaseURL, WireAPI: payload.Provider.WireAPI, Headers: payload.Headers, Models: payload.Provider.Models}, payload.APIKey)
	if err != nil {
		http.Error(w, "Provider test failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "models": models})
}

func handleAPICustomProviderPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := decodeCustomProviderRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCustomProvider(payload.Provider, payload.Headers); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	model, err := custom.ResolveModel(custom.Config{Models: payload.Provider.Models}, payload.ModelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"uiModelId": model.ID, "upstreamModelId": model.UpstreamID, "headers": redactHeaders(payload.Headers), "credentialConfigured": strings.TrimSpace(payload.APIKey) != ""})
}

func decodeCustomProviderRequest(w http.ResponseWriter, r *http.Request) (customProviderRequest, error) {
	var payload customProviderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&payload); err != nil {
		return customProviderRequest{}, err
	}
	if payload.Headers == nil {
		payload.Headers = map[string]string{}
	}
	return payload, nil
}

func validateCustomProvider(provider custom.Provider, headers map[string]string) error {
	if _, err := storage.SanitizeAccountName(provider.ID); err != nil {
		return err
	}
	return custom.Validate(custom.Config{ID: provider.ID, Name: provider.Name, BaseURL: provider.BaseURL, WireAPI: provider.WireAPI, Headers: headers, Models: provider.Models})
}

func stateCustomProvider(provider custom.Provider, headers map[string]string) state.CustomProviderSettings {
	models := make([]state.CustomProviderModelSettings, 0, len(provider.Models))
	for _, model := range provider.Models {
		models = append(models, state.CustomProviderModelSettings{ID: model.ID, UpstreamID: model.UpstreamID, DisplayName: model.DisplayName, ReasoningEfforts: model.ReasoningEfforts, InputModalities: model.InputModalities})
	}
	return state.CustomProviderSettings{Name: provider.Name, BaseURL: provider.BaseURL, WireAPI: provider.WireAPI, EnvKey: customProviderAPIKeyEnv(provider.ID), Headers: headers, Models: models}
}

func redactHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for name := range headers {
		result[name] = "configured"
	}
	return result
}

func customProviderSnapshot(settings state.AppSettings) map[string]any {
	return map[string]any{"providers": customProviderSettingsSnapshot(settings.CodexBackend.CustomProviders)}
}

func customProviderSettingsSnapshot(configured map[string]state.CustomProviderSettings) map[string]any {
	providers := make(map[string]any, len(configured))
	for id, provider := range configured {
		models := make([]map[string]any, 0, len(provider.Models))
		for _, model := range provider.Models {
			models = append(models, map[string]any{
				"id":               model.ID,
				"upstreamId":       model.UpstreamID,
				"displayName":      model.DisplayName,
				"reasoningEfforts": model.ReasoningEfforts,
				"inputModalities":  model.InputModalities,
			})
		}
		providers[id] = map[string]any{
			"name":                 provider.Name,
			"baseUrl":              provider.BaseURL,
			"wireApi":              provider.WireAPI,
			"envKey":               provider.EnvKey,
			"headers":              redactHeaders(provider.Headers),
			"models":               models,
			"credentialConfigured": secrets.Configured(customProviderSecretName(id)),
		}
	}
	return providers
}
