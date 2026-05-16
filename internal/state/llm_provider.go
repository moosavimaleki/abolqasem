package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LlmProviderOpenAIBaseURL     = "https://api.openai.com/v1"
	LlmProviderOpenRouterBaseURL = "https://openrouter.ai/api/v1"
	DefaultOpenAISDKModel        = "gpt-5.4-mini"
	DefaultOpenRouterSDKModel    = "moonshotai/kimi-k2.5:nitro"
	defaultLlmProvider           = "openai"
)

var llmProviderHTTPClient = &http.Client{Timeout: 15 * time.Second}

type LlmProviderSnapshot struct {
	Provider        string  `json:"provider"`
	APIKey          string  `json:"apiKey"`
	Model           string  `json:"model"`
	BaseURL         string  `json:"baseUrl"`
	ResolvedBaseURL string  `json:"resolvedBaseUrl"`
	Enabled         bool    `json:"enabled"`
	Warning         *string `json:"warning"`
	FilePathDisplay string  `json:"filePathDisplay"`
}

type LlmProviderValidationResult struct {
	OK    bool `json:"ok"`
	Error any  `json:"error"`
}

func GetLlmProviderFilePath() string {
	return filepath.Join(stateDir, "llm-provider.json")
}

func ResolveLlmProviderBaseURL(provider string, baseURL string) string {
	switch provider {
	case "openai":
		return LlmProviderOpenAIBaseURL
	case "openrouter":
		return LlmProviderOpenRouterBaseURL
	default:
		return strings.TrimSpace(baseURL)
	}
}

func ResolveLlmProviderDefaultModel(provider string) string {
	switch provider {
	case "openai":
		return DefaultOpenAISDKModel
	case "openrouter":
		return DefaultOpenRouterSDKModel
	default:
		return ""
	}
}

func LoadLlmProviderSnapshot() (LlmProviderSnapshot, error) {
	path := GetLlmProviderFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return createDefaultLlmProviderSnapshot(path, nil), nil
		}
		return LlmProviderSnapshot{}, err
	}
	if strings.TrimSpace(string(data)) == "" {
		warning := "LLM provider file was empty. Using defaults."
		return createDefaultLlmProviderSnapshot(path, &warning), nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		warning := "LLM provider file is invalid JSON. Using defaults."
		return createDefaultLlmProviderSnapshot(path, &warning), nil
	}
	return NormalizeLlmProviderSnapshot(parsed, path), nil
}

func SaveLlmProviderSnapshot(value map[string]any) (LlmProviderSnapshot, error) {
	path := GetLlmProviderFilePath()
	snapshot := NormalizeLlmProviderSnapshot(value, path)
	payload := map[string]any{
		"provider": snapshot.Provider,
		"apiKey":   snapshot.APIKey,
		"model":    snapshot.Model,
		"baseUrl":  nil,
	}
	if snapshot.Provider == "custom" {
		payload["baseUrl"] = snapshot.BaseURL
	}
	if err := writeLlmProviderFile(path, payload); err != nil {
		return LlmProviderSnapshot{}, err
	}
	return snapshot, nil
}

func NormalizeLlmProviderSnapshot(value map[string]any, path string) LlmProviderSnapshot {
	if value == nil {
		return createDefaultLlmProviderSnapshot(path, nil)
	}
	warnings := []string{}
	provider := normalizeLlmProviderKind(value["provider"])
	apiKey := normalizeLlmString(value["apiKey"])
	model := normalizeLlmString(value["model"])
	baseURL := normalizeLlmString(value["baseUrl"])

	if provider == "" {
		warnings = append(warnings, "provider must be one of openai, openrouter, or custom")
		provider = defaultLlmProvider
	}
	if raw, ok := value["apiKey"]; ok && raw != nil {
		if _, stringOK := raw.(string); !stringOK {
			warnings = append(warnings, "apiKey must be a string")
		}
	}
	if raw, ok := value["model"]; ok && raw != nil {
		if _, stringOK := raw.(string); !stringOK {
			warnings = append(warnings, "model must be a string")
		}
	}
	if raw, ok := value["baseUrl"]; ok && raw != nil {
		if _, stringOK := raw.(string); !stringOK {
			warnings = append(warnings, "baseUrl must be a string or null")
		}
	}
	if provider == "custom" && baseURL == "" {
		warnings = append(warnings, "custom provider requires a baseUrl")
	}

	resolvedModel := model
	if resolvedModel == "" {
		resolvedModel = ResolveLlmProviderDefaultModel(provider)
	}
	resolvedBaseURL := ResolveLlmProviderBaseURL(provider, baseURL)
	enabled := len(warnings) == 0 && apiKey != "" && resolvedModel != "" && resolvedBaseURL != ""

	var warning *string
	if len(warnings) > 0 {
		text := "Some LLM provider settings are invalid: " + strings.Join(warnings, "; ")
		warning = &text
	}
	return LlmProviderSnapshot{
		Provider:        provider,
		APIKey:          apiKey,
		Model:           resolvedModel,
		BaseURL:         baseURL,
		ResolvedBaseURL: resolvedBaseURL,
		Enabled:         enabled,
		Warning:         warning,
		FilePathDisplay: formatKeybindingsDisplayPath(path),
	}
}

func ValidateLlmProviderCredentials(value map[string]any) LlmProviderValidationResult {
	snapshot := NormalizeLlmProviderSnapshot(value, GetLlmProviderFilePath())
	if !snapshot.Enabled {
		message := "LLM provider configuration is incomplete."
		if snapshot.Warning != nil {
			message = *snapshot.Warning
		}
		return LlmProviderValidationResult{
			OK: false,
			Error: map[string]any{
				"type":    "config_error",
				"message": message,
			},
		}
	}

	payload, err := json.Marshal(map[string]any{
		"model":             snapshot.Model,
		"input":             "Reply with ok.",
		"max_output_tokens": 5,
	})
	if err != nil {
		return LlmProviderValidationResult{OK: false, Error: err.Error()}
	}
	url := strings.TrimRight(snapshot.ResolvedBaseURL, "/") + "/responses"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return LlmProviderValidationResult{OK: false, Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+snapshot.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := llmProviderHTTPClient.Do(req)
	if err != nil {
		return LlmProviderValidationResult{OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return LlmProviderValidationResult{OK: true, Error: nil}
	}
	var responseBody any
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		responseBody = resp.Status
	}
	return LlmProviderValidationResult{
		OK: false,
		Error: map[string]any{
			"type":   "http_error",
			"status": resp.Status,
			"body":   responseBody,
		},
	}
}

func writeLlmProviderFile(path string, payload map[string]any) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	tempPath := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func createDefaultLlmProviderSnapshot(path string, warning *string) LlmProviderSnapshot {
	return LlmProviderSnapshot{
		Provider:        defaultLlmProvider,
		APIKey:          "",
		Model:           DefaultOpenAISDKModel,
		BaseURL:         "",
		ResolvedBaseURL: LlmProviderOpenAIBaseURL,
		Enabled:         false,
		Warning:         warning,
		FilePathDisplay: formatKeybindingsDisplayPath(path),
	}
}

func normalizeLlmProviderKind(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(strings.ToLower(text))
	switch text {
	case "openai", "openrouter", "custom":
		return text
	default:
		return ""
	}
}

func normalizeLlmString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
