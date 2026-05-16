package state

import (
	"os"
	"testing"
)

func TestApplySettingsPatchPersistsKannaSettings(t *testing.T) {
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })

	settings := DefaultAppSettings()
	scrollback := 9000
	minColumnWidth := 12
	editorPreset := "vscode"
	editorCommand := "code -g {{file}}:{{line}}"
	codexModel := "gpt-5.4"
	fastMode := true
	planMode := true
	analytics := true
	settings = ApplySettingsPatch(settings, AppSettingsPatch{
		AnalyticsEnabled: &analytics,
		Locale:           "fa",
		Theme:            "dark",
		ChatSoundID:      "chime",
		Terminal: &TerminalSettingsPatch{
			ScrollbackLines: &scrollback,
			MinColumnWidth:  &minColumnWidth,
		},
		Editor: &EditorSettingsPatch{
			Preset:          &editorPreset,
			CommandTemplate: &editorCommand,
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
	})
	if err := SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings returned error: %v", err)
	}
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}

	if loaded.Locale != "fa" || loaded.Theme != "dark" || !loaded.AnalyticsEnabled {
		t.Fatalf("unexpected basic settings: %#v", loaded)
	}
	if loaded.Terminal.ScrollbackLines != 9000 || loaded.Terminal.MinColumnWidth != 12 {
		t.Fatalf("unexpected terminal settings: %#v", loaded.Terminal)
	}
	if loaded.Editor.Preset != "vscode" || loaded.Editor.CommandTemplate != editorCommand {
		t.Fatalf("unexpected editor settings: %#v", loaded.Editor)
	}
	if loaded.DefaultProvider != "codex" {
		t.Fatalf("unexpected default provider: %q", loaded.DefaultProvider)
	}
	codex := loaded.ProviderDefaults["codex"]
	if codex.Model != "gpt-5.4" || !codex.PlanMode || codex.ModelOptions["fastMode"] != true {
		t.Fatalf("unexpected codex defaults: %#v", codex)
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
		"provider_defaults": {"codex": {"model": ""}}
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
		t.Fatalf("expected default provider, got %q", loaded.DefaultProvider)
	}
}
