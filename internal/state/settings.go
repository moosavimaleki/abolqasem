package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ai-agent-manager/internal/providers/catalog"
)

const (
	HookFollowAuto   = "auto"
	HookFollowNotice = "notice"
	HookFollowOff    = "off"

	ProviderProxyModeNone   = "none"
	ProviderProxyModeCustom = "custom"
)

type AppSettings struct {
	HookUpdates                     bool                                     `json:"hook_updates"`
	HookFollowMode                  string                                   `json:"hook_follow_mode"`
	IgnoreHookNavigationWhileTyping bool                                     `json:"ignore_hook_navigation_while_typing"`
	FilesystemDiscovery             bool                                     `json:"filesystem_discovery"`
	Locale                          string                                   `json:"locale"`
	Theme                           string                                   `json:"theme"`
	BrowserSettingsMigrated         bool                                     `json:"browser_settings_migrated"`
	ChatSoundPreference             string                                   `json:"chat_sound_preference"`
	ChatSoundID                     string                                   `json:"chat_sound_id"`
	Terminal                        TerminalSettings                         `json:"terminal"`
	Editor                          EditorSettings                           `json:"editor"`
	ProviderProxy                   ProviderProxySettings                    `json:"provider_proxy"`
	DefaultProvider                 string                                   `json:"default_provider"`
	ProviderDefaults                map[string]ProviderPreference            `json:"provider_defaults"`
	ProviderModelCatalog            catalog.ProviderModelInventoryByProvider `json:"provider_model_catalog"`
	ProviderExecutables             map[string]string                        `json:"provider_executables"`
	TmuxCommands                    map[string]string                        `json:"tmux_commands"`
	CommitMessageGenerator          CommitMessageGeneratorSettings           `json:"commit_message_generator"`
	DefaultAgent                    string                                   `json:"default_agent"`
	AgentModels                     map[string]string                        `json:"agent_models"`
}

type TerminalSettings struct {
	ScrollbackLines int `json:"scrollback_lines"`
	MinColumnWidth  int `json:"min_column_width"`
}

type EditorSettings struct {
	Preset          string `json:"preset"`
	CommandTemplate string `json:"command_template"`
}

type ProviderProxySettings struct {
	Mode      string `json:"mode"`
	HTTPProxy string `json:"http_proxy"`
	NoProxy   string `json:"no_proxy"`
}

type ProviderPreference struct {
	Model        string         `json:"model"`
	ModelOptions map[string]any `json:"model_options"`
	PlanMode     bool           `json:"plan_mode"`
}

type AppSettingsPatch struct {
	BrowserSettingsMigrated *bool                                  `json:"browserSettingsMigrated"`
	Locale                  string                                 `json:"locale"`
	Theme                   string                                 `json:"theme"`
	ChatSoundPreference     string                                 `json:"chatSoundPreference"`
	ChatSoundID             string                                 `json:"chatSoundId"`
	Terminal                *TerminalSettingsPatch                 `json:"terminal"`
	Editor                  *EditorSettingsPatch                   `json:"editor"`
	ProviderProxy           *ProviderProxySettingsPatch            `json:"providerProxy"`
	DefaultProvider         string                                 `json:"defaultProvider"`
	ProviderDefaults        map[string]ProviderPreferencePatch     `json:"providerDefaults"`
	ProviderModelCatalog    map[string]ProviderModelInventoryPatch `json:"providerModelCatalog"`
	ProviderExecutables     map[string]string                      `json:"providerExecutables"`
	TmuxCommands            map[string]string                      `json:"tmuxCommands"`
	CommitMessageGenerator  *CommitMessageGeneratorPatch           `json:"commitMessageGenerator"`
}

type TerminalSettingsPatch struct {
	ScrollbackLines *int `json:"scrollbackLines"`
	MinColumnWidth  *int `json:"minColumnWidth"`
}

type EditorSettingsPatch struct {
	Preset          *string `json:"preset"`
	CommandTemplate *string `json:"commandTemplate"`
}

type ProviderProxySettingsPatch struct {
	Mode      *string `json:"mode"`
	HTTPProxy *string `json:"httpProxy"`
	NoProxy   *string `json:"noProxy"`
}

type ProviderPreferencePatch struct {
	Model        *string        `json:"model"`
	ModelOptions map[string]any `json:"modelOptions"`
	PlanMode     *bool          `json:"planMode"`
}

type ProviderModelInventoryPatch struct {
	CatalogModels *[]catalog.ProviderModelOption `json:"catalogModels"`
	CustomModels  *[]catalog.ProviderModelOption `json:"customModels"`
}

type CommitMessageGeneratorSettings struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type CommitMessageGeneratorPatch struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		HookUpdates:                     true,
		HookFollowMode:                  HookFollowAuto,
		IgnoreHookNavigationWhileTyping: true,
		FilesystemDiscovery:             true,
		Locale:                          "fa",
		Theme:                           "system",
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
		ProviderProxy:   defaultProviderProxySettings(),
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
				Model: catalog.CodexRuntimeDefaultModel(),
				ModelOptions: map[string]any{
					"reasoningEffort": catalog.DefaultCodexReasoningEffort,
					"fastMode":        false,
				},
				PlanMode: false,
			},
			"gemini": {
				Model:        catalog.DefaultGeminiModel,
				ModelOptions: map[string]any{},
				PlanMode:     false,
			},
		},
		ProviderModelCatalog: catalog.ProviderModelInventoryByProvider{},
		ProviderExecutables:  map[string]string{},
		TmuxCommands:         map[string]string{},
		CommitMessageGenerator: CommitMessageGeneratorSettings{
			Provider: "codex",
			Model:    catalog.CodexRuntimeDefaultModel(),
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
	settings.ProviderProxy = normalizeProviderProxySettings(settings.ProviderProxy)
	settings.DefaultProvider = normalizeDefaultProvider(settings.DefaultProvider, defaults.DefaultProvider)
	settings.ProviderModelCatalog = normalizeProviderModelCatalog(settings.ProviderModelCatalog)
	settings.ProviderExecutables = normalizeProviderExecutables(settings.ProviderExecutables)
	settings.TmuxCommands = normalizeTmuxCommands(settings.TmuxCommands)
	settings.CommitMessageGenerator = normalizeCommitMessageGenerator(settings.CommitMessageGenerator, settings.ProviderModelCatalog)
	settings.ProviderDefaults = normalizeProviderDefaults(settings.ProviderDefaults, defaults.ProviderDefaults, settings.ProviderModelCatalog)
	settings.AgentModels = normalizeAgentModels(settings.AgentModels)
	return settings
}

func ApplySettingsPatch(settings AppSettings, patch AppSettingsPatch) AppSettings {
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
	if patch.ProviderProxy != nil {
		if patch.ProviderProxy.Mode != nil {
			settings.ProviderProxy.Mode = *patch.ProviderProxy.Mode
		}
		if patch.ProviderProxy.HTTPProxy != nil {
			settings.ProviderProxy.HTTPProxy = *patch.ProviderProxy.HTTPProxy
		}
		if patch.ProviderProxy.NoProxy != nil {
			settings.ProviderProxy.NoProxy = *patch.ProviderProxy.NoProxy
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
	if len(patch.ProviderModelCatalog) > 0 {
		if settings.ProviderModelCatalog == nil {
			settings.ProviderModelCatalog = catalog.ProviderModelInventoryByProvider{}
		}
		for provider, modelPatch := range patch.ProviderModelCatalog {
			provider = normalizeWorkspaceProvider(provider)
			if provider == "" {
				continue
			}
			current := settings.ProviderModelCatalog[provider]
			if modelPatch.CatalogModels != nil {
				current.CatalogModels = append([]catalog.ProviderModelOption(nil), (*modelPatch.CatalogModels)...)
			}
			if modelPatch.CustomModels != nil {
				current.CustomModels = append([]catalog.ProviderModelOption(nil), (*modelPatch.CustomModels)...)
			}
			settings.ProviderModelCatalog[provider] = current
		}
	}
	if len(patch.ProviderExecutables) > 0 {
		if settings.ProviderExecutables == nil {
			settings.ProviderExecutables = map[string]string{}
		}
		for provider, executable := range patch.ProviderExecutables {
			provider = normalizeWorkspaceProvider(provider)
			if provider == "" {
				continue
			}
			settings.ProviderExecutables[provider] = strings.TrimSpace(executable)
		}
	}
	if len(patch.TmuxCommands) > 0 {
		if settings.TmuxCommands == nil {
			settings.TmuxCommands = map[string]string{}
		}
		for provider, command := range patch.TmuxCommands {
			provider = normalizeWorkspaceProvider(provider)
			if provider == "" {
				continue
			}
			settings.TmuxCommands[provider] = strings.TrimSpace(command)
		}
	}
	if patch.CommitMessageGenerator != nil {
		if provider := normalizeWorkspaceProvider(patch.CommitMessageGenerator.Provider); provider != "" {
			settings.CommitMessageGenerator.Provider = provider
		}
		if model := strings.TrimSpace(patch.CommitMessageGenerator.Model); model != "" {
			settings.CommitMessageGenerator.Model = model
		}
	}
	return NormalizeSettings(settings)
}

func CurrentProviderProxyEnv() []string {
	settings, err := LoadSettings()
	if err != nil {
		settings = DefaultAppSettings()
	}
	return ApplyProviderProxyEnv(os.Environ(), settings)
}

func CurrentProviderProxyEnvWithOverrides(overrides []string) []string {
	return MergeEnvOverrides(CurrentProviderProxyEnv(), overrides)
}

func MergeEnvOverrides(base []string, overrides []string) []string {
	next := append([]string(nil), base...)
	if len(overrides) == 0 {
		return next
	}

	indexByKey := make(map[string]int, len(next))
	for index, entry := range next {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		indexByKey[envLookupKey(key)] = index
	}
	for _, entry := range overrides {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		lookupKey := envLookupKey(key)
		if index, ok := indexByKey[lookupKey]; ok {
			next[index] = entry
			continue
		}
		indexByKey[lookupKey] = len(next)
		next = append(next, entry)
	}
	return next
}

func ApplyProviderProxyEnv(env []string, settings AppSettings) []string {
	proxy := normalizeProviderProxySettings(settings.ProviderProxy)
	withoutProxy := make([]string, 0, len(env)+8)
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && isProviderProxyEnvKey(key) {
			continue
		}
		withoutProxy = append(withoutProxy, entry)
	}
	if proxy.Mode != ProviderProxyModeCustom || proxy.HTTPProxy == "" {
		return withoutProxy
	}
	withoutProxy = append(withoutProxy,
		"HTTP_PROXY="+proxy.HTTPProxy,
		"HTTPS_PROXY="+proxy.HTTPProxy,
		"ALL_PROXY="+proxy.HTTPProxy,
		"http_proxy="+proxy.HTTPProxy,
		"https_proxy="+proxy.HTTPProxy,
		"all_proxy="+proxy.HTTPProxy,
	)
	if proxy.NoProxy != "" {
		withoutProxy = append(withoutProxy,
			"NO_PROXY="+proxy.NoProxy,
			"no_proxy="+proxy.NoProxy,
		)
	}
	return withoutProxy
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
		model = strings.TrimSpace(model)
		if agent == "codex" && model != "" {
			model = normalizeCodexCLIDefaultModel(catalog.NormalizeServerModel("codex", model))
		}
		normalized[agent] = model
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

func defaultProviderProxySettings() ProviderProxySettings {
	return ProviderProxySettings{Mode: ProviderProxyModeNone}
}

func normalizeProviderProxySettings(settings ProviderProxySettings) ProviderProxySettings {
	settings.Mode = strings.TrimSpace(strings.ToLower(settings.Mode))
	if settings.Mode != ProviderProxyModeCustom {
		settings.Mode = ProviderProxyModeNone
	}
	settings.HTTPProxy = strings.TrimSpace(settings.HTTPProxy)
	settings.NoProxy = strings.TrimSpace(settings.NoProxy)
	return settings
}

func isProviderProxyEnvKey(key string) bool {
	switch key {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy":
		return true
	default:
		return false
	}
}

func envLookupKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func normalizeDefaultProvider(value string, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "last_used" || value == "claude" || value == "codex" || value == "gemini" {
		return value
	}
	return fallback
}

func normalizeWorkspaceProvider(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "claude" || value == "codex" || value == "gemini" {
		return value
	}
	return ""
}

func normalizeTmuxCommands(commands map[string]string) map[string]string {
	normalized := map[string]string{}
	for provider, command := range commands {
		provider = normalizeWorkspaceProvider(provider)
		command = strings.TrimSpace(command)
		if provider != "" && command != "" {
			normalized[provider] = command
		}
	}
	return normalized
}

func normalizeProviderExecutables(executables map[string]string) map[string]string {
	normalized := map[string]string{}
	for provider, executable := range executables {
		provider = normalizeWorkspaceProvider(provider)
		executable = strings.TrimSpace(executable)
		if provider != "" && executable != "" {
			normalized[provider] = executable
		}
	}
	return normalized
}

func normalizeProviderDefaults(settings map[string]ProviderPreference, defaults map[string]ProviderPreference, modelCatalog catalog.ProviderModelInventoryByProvider) map[string]ProviderPreference {
	normalized := map[string]ProviderPreference{}
	for provider, fallback := range defaults {
		current := settings[provider]
		if strings.TrimSpace(current.Model) == "" {
			current.Model = fallback.Model
		}
		if provider == "codex" {
			current.Model = normalizeCodexCLIDefaultModel(current.Model)
		}
		current.Model = catalog.NormalizeServerModelWithInventory(provider, current.Model, modelCatalog)
		current.ModelOptions = mergeMap(fallback.ModelOptions, current.ModelOptions)
		normalized[provider] = current
	}
	return normalized
}

func normalizeCommitMessageGenerator(generator CommitMessageGeneratorSettings, modelCatalog catalog.ProviderModelInventoryByProvider) CommitMessageGeneratorSettings {
	provider := normalizeWorkspaceProvider(generator.Provider)
	if provider == "" {
		provider = "codex"
	}
	model := strings.TrimSpace(generator.Model)
	if model == "" {
		model = catalog.GetOrDefaultWithInventory(provider, modelCatalog).DefaultModel
	}
	if provider == "codex" {
		model = normalizeCodexCLIDefaultModel(model)
	}
	model = catalog.NormalizeServerModelWithInventory(provider, model, modelCatalog)
	return CommitMessageGeneratorSettings{
		Provider: provider,
		Model:    model,
	}
}

func normalizeProviderModelCatalog(modelCatalog catalog.ProviderModelInventoryByProvider) catalog.ProviderModelInventoryByProvider {
	normalized := catalog.ProviderModelInventoryByProvider{}
	for provider, inventory := range modelCatalog {
		provider = normalizeWorkspaceProvider(provider)
		if provider == "" {
			continue
		}
		normalized[provider] = catalog.ProviderModelInventory{
			CatalogModels:    normalizeProviderModelOptions(provider, inventory.CatalogModels),
			DiscoveredModels: normalizeProviderModelOptions(provider, inventory.DiscoveredModels),
			CustomModels:     normalizeProviderModelOptions(provider, inventory.CustomModels),
			LastRefreshAt:    strings.TrimSpace(inventory.LastRefreshAt),
			LastError:        strings.TrimSpace(inventory.LastError),
		}
	}
	return normalized
}

func normalizeProviderModelOptions(provider string, models []catalog.ProviderModelOption) []catalog.ProviderModelOption {
	if models == nil {
		return nil
	}
	out := make([]catalog.ProviderModelOption, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		normalized, ok := catalog.NormalizeProviderModelOption(provider, model)
		if !ok || seen[normalized.ID] {
			continue
		}
		seen[normalized.ID] = true
		out = append(out, normalized)
	}
	return out
}

func normalizeCodexCLIDefaultModel(model string) string {
	model = strings.TrimSpace(model)
	if model == catalog.DefaultCodexModel && catalog.CodexRuntimeDefaultModel() != catalog.DefaultCodexModel {
		return catalog.CodexRuntimeDefaultModel()
	}
	return model
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
