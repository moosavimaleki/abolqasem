// Package configoverlay prepares an isolated Codex home for an app-server
// process without mutating the user's CODEX_HOME.
package configoverlay

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	ProviderID  = "codex_manager"
	APIKeyEnv   = "CODEX_MANAGER_GATEWAY_API_KEY"
	configName  = "config.toml"
	providerKey = "model_providers"
)

type Options struct {
	SourceHome  string
	RuntimeHome string
	BaseURL     string
}

// CustomOptions describes an explicitly configured OpenAI Responses-compatible
// upstream. Unlike the manager sidecar it may be remote, but it still receives
// credentials only through its declared environment variable.
type CustomOptions struct {
	SourceHome   string
	RuntimeHome  string
	ProviderID   string
	ProviderName string
	BaseURL      string
	APIKeyEnv    string
}

type Result struct {
	Home       string
	ProviderID string
	APIKeyEnv  string
	BaseURL    string
}

func Build(options Options) (Result, error) {
	if err := validateManager(options); err != nil {
		return Result{}, err
	}
	return build(options.SourceHome, options.RuntimeHome, providerConfig{
		ID: ProviderID, Name: "Codex Manager", BaseURL: options.BaseURL, APIKeyEnv: APIKeyEnv,
	})
}

func BuildCustom(options CustomOptions) (Result, error) {
	if err := validateCustom(options); err != nil {
		return Result{}, err
	}
	return build(options.SourceHome, options.RuntimeHome, providerConfig{
		ID: options.ProviderID, Name: options.ProviderName, BaseURL: options.BaseURL, APIKeyEnv: options.APIKeyEnv,
	})
}

type providerConfig struct {
	ID        string
	Name      string
	BaseURL   string
	APIKeyEnv string
}

func build(sourceHome, runtimeHome string, provider providerConfig) (Result, error) {
	if err := os.MkdirAll(runtimeHome, 0o700); err != nil {
		return Result{}, fmt.Errorf("create isolated CODEX_HOME: %w", err)
	}
	if err := copyUserConfiguration(sourceHome, runtimeHome); err != nil {
		return Result{}, err
	}
	if err := writeOverlay(runtimeHome, provider); err != nil {
		return Result{}, err
	}
	return Result{
		Home:       runtimeHome,
		ProviderID: provider.ID,
		APIKeyEnv:  provider.APIKeyEnv,
		BaseURL:    provider.BaseURL,
	}, nil
}

func (r Result) Environment(apiKey string) ([]string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("codex-manager gateway key is missing")
	}
	return r.OptionalEnvironment(apiKey), nil
}

func (r Result) OptionalEnvironment(apiKey string) []string {
	env := []string{"CODEX_HOME=" + r.Home}
	if strings.TrimSpace(r.APIKeyEnv) != "" && strings.TrimSpace(apiKey) != "" {
		env = append(env, r.APIKeyEnv+"="+apiKey)
	}
	return env
}

func validateManager(options Options) error {
	if strings.TrimSpace(options.RuntimeHome) == "" {
		return errors.New("isolated CODEX_HOME is required")
	}
	baseURL, err := url.Parse(strings.TrimSpace(options.BaseURL))
	if err != nil || baseURL.Scheme != "http" || !isLoopbackHost(baseURL.Hostname()) {
		return errors.New("codex-manager provider URL must be an HTTP loopback address")
	}
	return nil
}

func validateCustom(options CustomOptions) error {
	if strings.TrimSpace(options.RuntimeHome) == "" {
		return errors.New("isolated CODEX_HOME is required")
	}
	if !validProviderID(options.ProviderID) {
		return errors.New("custom provider ID must contain only letters, digits, and underscores")
	}
	if strings.TrimSpace(options.ProviderName) == "" {
		return errors.New("custom provider name is required")
	}
	baseURL, err := url.Parse(strings.TrimSpace(options.BaseURL))
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return errors.New("custom provider URL must be HTTP(S)")
	}
	return nil
}

func validProviderID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_') {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func copyUserConfiguration(sourceHome string, runtimeHome string) error {
	sourceHome = strings.TrimSpace(sourceHome)
	if sourceHome == "" {
		return nil
	}
	configPath := filepath.Join(sourceHome, configName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read existing Codex config: %w", err)
	}
	return os.WriteFile(filepath.Join(runtimeHome, configName), data, 0o600)
}

func writeOverlay(runtimeHome string, providerConfig providerConfig) error {
	path := filepath.Join(runtimeHome, configName)
	config := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse existing Codex config: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	providers, ok := config[providerKey].(map[string]any)
	if !ok || providers == nil {
		providers = map[string]any{}
		config[providerKey] = providers
	}
	provider := map[string]any{
		"name":     providerConfig.Name,
		"base_url": strings.TrimRight(providerConfig.BaseURL, "/"),
		"wire_api": "responses",
	}
	if strings.TrimSpace(providerConfig.APIKeyEnv) != "" {
		provider["env_key"] = providerConfig.APIKeyEnv
	}
	providers[providerConfig.ID] = provider
	config["model_provider"] = providerConfig.ID

	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode Codex provider overlay: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}
