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
	PID     int    `json:"pid,omitempty"`
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
	return SaveServerRuntime(baseURL, 0)
}

func LoadServerPID() int {
	data, err := os.ReadFile(GetServerConfigPath())
	if err != nil {
		return 0
	}

	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0
	}
	if cfg.PID < 0 {
		return 0
	}
	return cfg.PID
}

func SaveServerRuntime(baseURL string, pid int) error {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL(DefaultPort)
	}
	if pid < 0 {
		pid = 0
	}
	data, err := json.MarshalIndent(ServerConfig{BaseURL: baseURL, PID: pid}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetServerConfigPath(), data, 0644)
}
