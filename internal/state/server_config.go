package state

import (
	"ai-agent-manager/internal/appinfo"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultPort    = 9092
	BaseURLEnvName = appinfo.EnvPrefix + "_BASE_URL"
)

const legacyBaseURLEnvName = appinfo.LegacyEnvPrefix + "_BASE_URL"

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

func normalizeLoopbackBaseURL(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "http") || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}

	host := parsed.Hostname()
	if !isLoopbackHost(host) {
		return "", false
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 || port > 65535 {
		return "", false
	}

	return DefaultBaseURL(port), true
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func LoadServerBaseURL() string {
	if value := strings.TrimSpace(os.Getenv(BaseURLEnvName)); value != "" {
		if normalized, ok := normalizeLoopbackBaseURL(value); ok {
			return normalized
		}
		return DefaultBaseURL(DefaultPort)
	}
	if value := strings.TrimSpace(os.Getenv(legacyBaseURLEnvName)); value != "" {
		if normalized, ok := normalizeLoopbackBaseURL(value); ok {
			return normalized
		}
		return DefaultBaseURL(DefaultPort)
	}

	data, err := os.ReadFile(GetServerConfigPath())
	if err != nil {
		return DefaultBaseURL(DefaultPort)
	}

	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultBaseURL(DefaultPort)
	}
	if normalized, ok := normalizeLoopbackBaseURL(cfg.BaseURL); ok {
		return normalized
	}
	return DefaultBaseURL(DefaultPort)
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
	normalizedBaseURL, ok := normalizeLoopbackBaseURL(baseURL)
	if !ok {
		normalizedBaseURL = DefaultBaseURL(DefaultPort)
	}
	if pid < 0 {
		pid = 0
	}
	data, err := json.MarshalIndent(ServerConfig{BaseURL: normalizedBaseURL, PID: pid}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetServerConfigPath(), data, 0644)
}
