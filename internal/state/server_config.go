package state

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultPort    = 9090
	BaseURLEnvName = "AI_AGENT_MANAGER_BASE_URL"
)

type ServerConfig struct {
	BaseURL string `json:"base_url"`
}

func GetServerConfigPath() string {
	return filepath.Join(stateDir, "server.json")
}

func DefaultBaseURL(port int) string {
	if port <= 0 {
		port = DefaultPort
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func LoadServerBaseURL() string {
	if value := strings.TrimSpace(os.Getenv(BaseURLEnvName)); value != "" {
		if _, err := url.ParseRequestURI(value); err == nil {
			return value
		}
	}

	data, err := os.ReadFile(GetServerConfigPath())
	if err != nil {
		return DefaultBaseURL(DefaultPort)
	}

	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultBaseURL(DefaultPort)
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return DefaultBaseURL(DefaultPort)
	}
	return cfg.BaseURL
}

func SaveServerBaseURL(baseURL string) error {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL(DefaultPort)
	}
	data, err := json.MarshalIndent(ServerConfig{BaseURL: baseURL}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetServerConfigPath(), data, 0644)
}
