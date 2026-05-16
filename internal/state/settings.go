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
	HookUpdates                     bool                          `json:"hook_updates"`
	HookFollowMode                  string                        `json:"hook_follow_mode"`
	IgnoreHookNavigationWhileTyping bool                          `json:"ignore_hook_navigation_while_typing"`
	FilesystemDiscovery             bool                          `json:"filesystem_discovery"`
	Locale                          string                        `json:"locale"`
	Theme                           string                        `json:"theme"`
	AnalyticsEnabled                bool                          `json:"analytics_enabled"`
	BrowserSettingsMigrated         bool                          `json:"browser_settings_migrated"`
	ChatSoundPreference             string                        `json:"chat_sound_preference"`
	ChatSoundID                     string                        `json:"chat_sound_id"`
	Terminal                        TerminalSettings              `json:"terminal"`
	Editor                          EditorSettings                `json:"editor"`
	DefaultProvider                 string                        `json:"default_provider"`
	ProviderDefaults                map[string]ProviderPreference `json:"provider_defaults"`
	DefaultAgent                    string                        `json:"default_agent"`
	AgentModels                     map[string]string             `json:"agent_models"`
}

type TerminalSettings struct {
	ScrollbackLines int `json:"scrollback_lines"`
	MinColumnWidth  int `json:"min_column_width"`
}

type EditorSettings struct {
	Preset          string `json:"preset"`
	CommandTemplate string `json:"command_template"`
}

type ProviderPreference struct {
	Model        string         `json:"model"`
	ModelOptions map[string]any `json:"model_options"`
	PlanMode     bool           `json:"plan_mode"`
}

type AppSettingsPatch struct {
	AnalyticsEnabled        *bool                              `json:"analyticsEnabled"`
	BrowserSettingsMigrated *bool                              `json:"browserSettingsMigrated"`
	Locale                  string                             `json:"locale"`
	Theme                   string                             `json:"theme"`
	ChatSoundPreference     string                             `json:"chatSoundPreference"`
	ChatSoundID             string                             `json:"chatSoundId"`
	Terminal                *TerminalSettingsPatch             `json:"terminal"`
	Editor                  *EditorSettingsPatch               `json:"editor"`
	DefaultProvider         string                             `json:"defaultProvider"`
	ProviderDefaults        map[string]ProviderPreferencePatch `json:"providerDefaults"`
}

type TerminalSettingsPatch struct {
	ScrollbackLines *int `json:"scrollbackLines"`
	MinColumnWidth  *int `json:"minColumnWidth"`
}

type EditorSettingsPatch struct {
	Preset          *string `json:"preset"`
	CommandTemplate *string `json:"commandTemplate"`
}

type ProviderPreferencePatch struct {
	Model        *string        `json:"model"`
	ModelOptions map[string]any `json:"modelOptions"`
	PlanMode     *bool          `json:"planMode"`
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		HookUpdates:                     true,
		HookFollowMode:                  HookFollowAuto,
		IgnoreHookNavigationWhileTyping: true,
		FilesystemDiscovery:             true,
		Locale:                          "en",
		Theme:                           "system",
		AnalyticsEnabled:                false,
		BrowserSettingsMigrated:         true,
		ChatSoundPreference:             "unfocused",
		ChatSoundID:                     "pop",
		Terminal: TerminalSettings{
			ScrollbackLines: 5000,
			MinColumnWidth:  8,
		},
		Editor: EditorSettings{
			Preset:          "custom",
			CommandTemplate: "",
		},
		DefaultProvider: "last_used",
		ProviderDefaults: map[string]ProviderPreference{
			"claude": {
				Model: "claude-sonnet-4-6",
				ModelOptions: map[string]any{
					"reasoningEffort": "none",
					"contextWindow":   "200k",
				},
				PlanMode: false,
			},
			"codex": {
				Model: "gpt-5.5",
				ModelOptions: map[string]any{
					"reasoningEffort": "medium",
					"fastMode":        false,
				},
				PlanMode: false,
			},
		},
		DefaultAgent: "codex",
		AgentModels:  map[string]string{"codex": ""},
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
	settings.Locale = strings.TrimSpace(strings.ToLower(settings.Locale))
	if settings.Locale != "fa" && settings.Locale != "en" {
		settings.Locale = defaults.Locale
	}
	settings.Theme = normalizeChoice(settings.Theme, defaults.Theme, "system", "light", "dark")
	settings.ChatSoundPreference = normalizeChoice(settings.ChatSoundPreference, defaults.ChatSoundPreference, "always", "unfocused", "never")
	settings.ChatSoundID = normalizeChoice(settings.ChatSoundID, defaults.ChatSoundID, "pop", "ding", "chime", "none")
	settings.Terminal = normalizeTerminalSettings(settings.Terminal, defaults.Terminal)
	settings.Editor = normalizeEditorSettings(settings.Editor, defaults.Editor)
	settings.DefaultProvider = normalizeDefaultProvider(settings.DefaultProvider, defaults.DefaultProvider)
	settings.ProviderDefaults = normalizeProviderDefaults(settings.ProviderDefaults, defaults.ProviderDefaults)
	settings.AgentModels = normalizeAgentModels(settings.AgentModels)
	return settings
}

func ApplySettingsPatch(settings AppSettings, patch AppSettingsPatch) AppSettings {
	if patch.AnalyticsEnabled != nil {
		settings.AnalyticsEnabled = *patch.AnalyticsEnabled
	}
	if patch.BrowserSettingsMigrated != nil {
		settings.BrowserSettingsMigrated = *patch.BrowserSettingsMigrated
	}
	if patch.Locale != "" {
		settings.Locale = patch.Locale
	}
	if patch.Theme != "" {
		settings.Theme = patch.Theme
	}
	if patch.ChatSoundPreference != "" {
		settings.ChatSoundPreference = patch.ChatSoundPreference
	}
	if patch.ChatSoundID != "" {
		settings.ChatSoundID = patch.ChatSoundID
	}
	if patch.Terminal != nil {
		if patch.Terminal.ScrollbackLines != nil {
			settings.Terminal.ScrollbackLines = *patch.Terminal.ScrollbackLines
		}
		if patch.Terminal.MinColumnWidth != nil {
			settings.Terminal.MinColumnWidth = *patch.Terminal.MinColumnWidth
		}
	}
	if patch.Editor != nil {
		if patch.Editor.Preset != nil {
			settings.Editor.Preset = *patch.Editor.Preset
		}
		if patch.Editor.CommandTemplate != nil {
			settings.Editor.CommandTemplate = *patch.Editor.CommandTemplate
		}
	}
	if patch.DefaultProvider != "" {
		settings.DefaultProvider = patch.DefaultProvider
	}
	if len(patch.ProviderDefaults) > 0 {
		if settings.ProviderDefaults == nil {
			settings.ProviderDefaults = map[string]ProviderPreference{}
		}
		for provider, providerPatch := range patch.ProviderDefaults {
			provider = normalizeWorkspaceProvider(provider)
			if provider == "" {
				continue
			}
			current := settings.ProviderDefaults[provider]
			if providerPatch.Model != nil {
				current.Model = strings.TrimSpace(*providerPatch.Model)
			}
			if providerPatch.ModelOptions != nil {
				current.ModelOptions = mergeMap(current.ModelOptions, providerPatch.ModelOptions)
			}
			if providerPatch.PlanMode != nil {
				current.PlanMode = *providerPatch.PlanMode
			}
			settings.ProviderDefaults[provider] = current
		}
	}
	return NormalizeSettings(settings)
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

func normalizeChoice(value string, fallback string, allowed ...string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	for _, option := range allowed {
		if value == option {
			return value
		}
	}
	return fallback
}

func normalizeTerminalSettings(settings TerminalSettings, defaults TerminalSettings) TerminalSettings {
	if settings.ScrollbackLines < 100 {
		settings.ScrollbackLines = defaults.ScrollbackLines
	}
	if settings.ScrollbackLines > 200000 {
		settings.ScrollbackLines = 200000
	}
	if settings.MinColumnWidth < 4 {
		settings.MinColumnWidth = defaults.MinColumnWidth
	}
	if settings.MinColumnWidth > 40 {
		settings.MinColumnWidth = 40
	}
	return settings
}

func normalizeEditorSettings(settings EditorSettings, defaults EditorSettings) EditorSettings {
	settings.Preset = normalizeChoice(settings.Preset, defaults.Preset, "custom", "vscode", "cursor", "zed", "sublime", "webstorm", "vim", "neovim")
	settings.CommandTemplate = strings.TrimSpace(settings.CommandTemplate)
	if settings.Preset != "custom" && settings.CommandTemplate == "" {
		settings.CommandTemplate = defaults.CommandTemplate
	}
	return settings
}

func normalizeDefaultProvider(value string, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "last_used" || value == "claude" || value == "codex" {
		return value
	}
	return fallback
}

func normalizeWorkspaceProvider(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "claude" || value == "codex" {
		return value
	}
	return ""
}

func normalizeProviderDefaults(settings map[string]ProviderPreference, defaults map[string]ProviderPreference) map[string]ProviderPreference {
	normalized := map[string]ProviderPreference{}
	for provider, fallback := range defaults {
		current := settings[provider]
		if strings.TrimSpace(current.Model) == "" {
			current.Model = fallback.Model
		}
		current.ModelOptions = mergeMap(fallback.ModelOptions, current.ModelOptions)
		normalized[provider] = current
	}
	return normalized
}

func mergeMap(base map[string]any, patch map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range patch {
		merged[key] = value
	}
	return merged
}
