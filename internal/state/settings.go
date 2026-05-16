package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	HookFollowAuto   = "auto"
	HookFollowNotice = "notice"
	HookFollowOff    = "off"
)

type AppSettings struct {
	HookUpdates                     bool              `json:"hook_updates"`
	HookFollowMode                  string            `json:"hook_follow_mode"`
	IgnoreHookNavigationWhileTyping bool              `json:"ignore_hook_navigation_while_typing"`
	FilesystemDiscovery             bool              `json:"filesystem_discovery"`
	DefaultAgent                    string            `json:"default_agent"`
	AgentModels                     map[string]string `json:"agent_models"`
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		HookUpdates:                     true,
		HookFollowMode:                  HookFollowAuto,
		IgnoreHookNavigationWhileTyping: true,
		FilesystemDiscovery:             true,
		DefaultAgent:                    "codex",
		AgentModels:                     map[string]string{"codex": ""},
	}
}

func GetSettingsFilePath() string {
	return filepath.Join(stateDir, "settings.json")
}

func LoadSettings() (AppSettings, error) {
	mu.Lock()
	defer mu.Unlock()

	settings := DefaultAppSettings()
	path := GetSettingsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		corruptPath := path + ".corrupt." + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(path, corruptPath); renameErr != nil {
			return DefaultAppSettings(), err
		}
		return DefaultAppSettings(), nil
	}
	return NormalizeSettings(settings), nil
}

func SaveSettings(settings AppSettings) error {
	mu.Lock()
	defer mu.Unlock()

	settings = NormalizeSettings(settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	path := GetSettingsFilePath()
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func NormalizeSettings(settings AppSettings) AppSettings {
	defaults := DefaultAppSettings()
	settings.HookFollowMode = strings.TrimSpace(strings.ToLower(settings.HookFollowMode))
	switch settings.HookFollowMode {
	case HookFollowAuto, HookFollowNotice, HookFollowOff:
	default:
		settings.HookFollowMode = HookFollowAuto
	}
	settings.DefaultAgent = normalizeAgentName(settings.DefaultAgent)
	if settings.DefaultAgent == "" {
		settings.DefaultAgent = defaults.DefaultAgent
	}
	settings.AgentModels = normalizeAgentModels(settings.AgentModels)
	return settings
}

func normalizeAgentModels(models map[string]string) map[string]string {
	normalized := map[string]string{
		"codex":  "",
		"claude": "",
		"gemini": "",
	}
	for agent, model := range models {
		agent = normalizeAgentName(agent)
		if agent == "" {
			continue
		}
		normalized[agent] = strings.TrimSpace(model)
	}
	return normalized
}

func normalizeAgentName(agent string) string {
	agent = strings.TrimSpace(strings.ToLower(agent))
	switch agent {
	case "codex", "claude", "gemini":
		return agent
	default:
		return ""
	}
}
