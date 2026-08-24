package state

import (
	"os"
	"strings"
	"testing"

	"abolqasem/internal/providers/catalog"
)

func TestApplySettingsPatchPersistsAbolqasemSettings(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	settings := DefaultAppSettings()
	scrollback := 9000
	minColumnWidth := 12
	editorPreset := "vscode"
	editorCommand := "code -g {{file}}:{{line}}"
	codexModel := "gpt-5.4"
	proxyMode := ProviderProxyModeCustom
	httpProxy := " http://127.0.0.1:7890 "
	noProxy := " localhost,127.0.0.1 "
	fastMode := true
	planMode := true
	commitProvider := "codex"
	commitModel := "gpt-5.4"
	codexExecutable := " /home/user/.bun/bin/codex "
	settings = ApplySettingsPatch(settings, AppSettingsPatch{
		Locale:      "fa",
		Theme:       "dark",
		ChatSoundID: "chime",
		Terminal: &TerminalSettingsPatch{
			ScrollbackLines: &scrollback,
			MinColumnWidth:  &minColumnWidth,
		},
		Editor: &EditorSettingsPatch{
			Preset:          &editorPreset,
			CommandTemplate: &editorCommand,
		},
		ProviderProxy: &ProviderProxySettingsPatch{
			Mode:      &proxyMode,
			HTTPProxy: &httpProxy,
			NoProxy:   &noProxy,
		},
		DefaultProvider: "codex",
		ProviderDefaults: map[string]ProviderPreferencePatch{
			"codex": {
				Model: &codexModel,
				ModelOptions: map[string]any{
					"fastMode": fastMode,
				},
				PlanMode: &planMode,
			},
		},
		TmuxCommands: map[string]string{
			"Codex":   " codex --yolo ",
			"invalid": "ignored",
		},
		ProviderExecutables: map[string]string{
			"Codex":   codexExecutable,
			"invalid": "ignored",
		},
		CommitMessageGenerator: &CommitMessageGeneratorPatch{
			Provider: commitProvider,
			Model:    commitModel,
		},
	})
	if err := SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings returned error: %v", err)
	}
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}

	if loaded.Locale != "fa" || loaded.Theme != "dark" {
		t.Fatalf("unexpected basic settings: %#v", loaded)
	}
	if loaded.Terminal.ScrollbackLines != 9000 || loaded.Terminal.MinColumnWidth != 12 {
		t.Fatalf("unexpected terminal settings: %#v", loaded.Terminal)
	}
	if loaded.Editor.Preset != "vscode" || loaded.Editor.CommandTemplate != editorCommand {
		t.Fatalf("unexpected editor settings: %#v", loaded.Editor)
	}
	if loaded.ProviderProxy.Mode != ProviderProxyModeCustom || loaded.ProviderProxy.HTTPProxy != "http://127.0.0.1:7890" || loaded.ProviderProxy.NoProxy != "localhost,127.0.0.1" {
		t.Fatalf("unexpected provider proxy settings: %#v", loaded.ProviderProxy)
	}
	if loaded.DefaultProvider != "codex" {
		t.Fatalf("unexpected default provider: %q", loaded.DefaultProvider)
	}
	codex := loaded.ProviderDefaults["codex"]
	if codex.Model != "gpt-5.4" || !codex.PlanMode || codex.ModelOptions["fastMode"] != true {
		t.Fatalf("unexpected codex defaults: %#v", codex)
	}
	if loaded.CommitMessageGenerator.Provider != commitProvider || loaded.CommitMessageGenerator.Model != commitModel {
		t.Fatalf("unexpected commit message generator: %#v", loaded.CommitMessageGenerator)
	}
	if loaded.TmuxCommands["codex"] != "codex --yolo" {
		t.Fatalf("unexpected codex tmux command: %#v", loaded.TmuxCommands)
	}
	if loaded.ProviderExecutables["codex"] != "/home/user/.bun/bin/codex" {
		t.Fatalf("unexpected codex executable: %#v", loaded.ProviderExecutables)
	}
	if _, ok := loaded.TmuxCommands["invalid"]; ok {
		t.Fatalf("expected invalid tmux command provider to be ignored, got %#v", loaded.TmuxCommands)
	}
	if _, ok := loaded.ProviderExecutables["invalid"]; ok {
		t.Fatalf("expected invalid executable provider to be ignored, got %#v", loaded.ProviderExecutables)
	}
}

func TestLoadSettingsNormalizesCorruptOrInvalidValues(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	if err := os.WriteFile(GetSettingsFilePath(), []byte(`{
		"locale": "invalid",
		"theme": "purple",
		"terminal": {"scrollback_lines": 10, "min_column_width": 1},
		"default_provider": "gemini",
		"provider_defaults": {"codex": {"model": "gpt-5.5"}}
	}`), 0o644); err != nil {
		t.Fatalf("write settings failed: %v", err)
	}
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}
	defaults := DefaultAppSettings()
	if loaded.Locale != defaults.Locale || loaded.Theme != defaults.Theme {
		t.Fatalf("expected defaults for invalid locale/theme, got %#v", loaded)
	}
	if loaded.Terminal != defaults.Terminal {
		t.Fatalf("expected default terminal settings, got %#v", loaded.Terminal)
	}
	if loaded.DefaultProvider != defaults.DefaultProvider {
		t.Fatalf("expected removed provider to fall back to %q, got %q", defaults.DefaultProvider, loaded.DefaultProvider)
	}
	if loaded.ProviderDefaults["codex"].Model != catalog.CodexRuntimeDefaultModel() {
		t.Fatalf("expected runtime codex model default, got %q", loaded.ProviderDefaults["codex"].Model)
	}
}

func TestApplySettingsPatchPersistsEditableProviderModelCatalog(t *testing.T) {
	settings := DefaultAppSettings()
	settings = ApplySettingsPatch(settings, AppSettingsPatch{
		ProviderModelCatalog: map[string]ProviderModelInventoryPatch{
			"codex": {
				CatalogModels: &[]catalog.ProviderModelOption{
					{ID: " gpt-5.99-a ", Label: " Custom A "},
					{ID: "gpt-5.99-a", Label: "Duplicate"},
					{ID: "gpt-5.99-b"},
				},
				CustomModels: &[]catalog.ProviderModelOption{},
			},
		},
	})

	models := settings.ProviderModelCatalog["codex"].CatalogModels
	if len(models) != 2 {
		t.Fatalf("expected two normalized catalog models, got %#v", models)
	}
	if models[0].ID != "gpt-5.99-a" || models[0].Label != "Custom A" {
		t.Fatalf("unexpected first catalog model: %#v", models[0])
	}
	if models[1].ID != "gpt-5.99-b" || models[1].Label != "gpt-5.99-b" {
		t.Fatalf("unexpected second catalog model: %#v", models[1])
	}
	if got := catalog.NormalizeServerModelWithInventory("codex", "", settings.ProviderModelCatalog); got != "gpt-5.99-a" {
		t.Fatalf("expected editable catalog default fallback, got %q", got)
	}
}

func TestApplyProviderProxyEnvRemovesInheritedProxyByDefault(t *testing.T) {
	env := []string{
		"PATH=/bin",
		"HTTP_PROXY=http://old-proxy",
		"HTTPS_PROXY=http://old-proxy",
		"NO_PROXY=old.local",
		"HOME=/tmp",
	}
	next := ApplyProviderProxyEnv(env, DefaultAppSettings())
	if !containsEnv(next, "PATH=/bin") || !containsEnv(next, "HOME=/tmp") {
		t.Fatalf("expected non-proxy env to remain, got %#v", next)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if containsEnvKey(next, key) {
			t.Fatalf("expected %s to be removed, got %#v", key, next)
		}
	}
}

func TestApplyProviderProxyEnvSetsCustomProxy(t *testing.T) {
	settings := DefaultAppSettings()
	settings.ProviderProxy = ProviderProxySettings{
		Mode:      ProviderProxyModeCustom,
		HTTPProxy: "http://127.0.0.1:7890",
		NoProxy:   "localhost,127.0.0.1",
	}
	next := ApplyProviderProxyEnv([]string{"PATH=/bin", "http_proxy=http://old"}, settings)
	for _, entry := range []string{
		"HTTP_PROXY=http://127.0.0.1:7890",
		"HTTPS_PROXY=http://127.0.0.1:7890",
		"ALL_PROXY=http://127.0.0.1:7890",
		"http_proxy=http://127.0.0.1:7890",
		"https_proxy=http://127.0.0.1:7890",
		"all_proxy=http://127.0.0.1:7890",
		"NO_PROXY=localhost,127.0.0.1",
		"no_proxy=localhost,127.0.0.1",
	} {
		if !containsEnv(next, entry) {
			t.Fatalf("expected %q in env, got %#v", entry, next)
		}
	}
}

func TestMergeEnvOverridesPreservesBaseEnvAndAppliesOverrides(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"SystemRoot=C:\\Windows",
		"CLAUDE_HOME=/home/original/.claude",
	}
	next := MergeEnvOverrides(base, []string{
		"CLAUDE_HOME=/tmp/isolated/.claude",
		"CLAUDE_CONFIG_DIR=/tmp/isolated/.claude",
	})

	if envTestValue(next, "PATH") != "/usr/bin" {
		t.Fatalf("expected PATH to be preserved, got %#v", next)
	}
	if envTestValue(next, "SystemRoot") != "C:\\Windows" {
		t.Fatalf("expected SystemRoot to be preserved, got %#v", next)
	}
	if envTestValue(next, "CLAUDE_HOME") != "/tmp/isolated/.claude" {
		t.Fatalf("expected CLAUDE_HOME override, got %#v", next)
	}
	if envTestValue(next, "CLAUDE_CONFIG_DIR") != "/tmp/isolated/.claude" {
		t.Fatalf("expected CLAUDE_CONFIG_DIR override, got %#v", next)
	}
}

func TestNormalizeProviderProxySettingsKeepsCustomModeWithoutHttpProxy(t *testing.T) {
	settings := normalizeProviderProxySettings(ProviderProxySettings{
		Mode:    ProviderProxyModeCustom,
		NoProxy: " localhost ",
	})
	if settings.Mode != ProviderProxyModeCustom {
		t.Fatalf("expected custom mode to be preserved, got %#v", settings)
	}
	if settings.HTTPProxy != "" || settings.NoProxy != "localhost" {
		t.Fatalf("unexpected normalization result: %#v", settings)
	}
}

func containsEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}

func envTestValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func containsEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
