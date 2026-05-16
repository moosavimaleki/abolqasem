package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempKeybindingsStateDir(t *testing.T) {
	t.Helper()
	original := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = original })
}

func TestNormalizeKeybindingsFallsBackToDefaultsForInvalidEntries(t *testing.T) {
	snapshot := NormalizeKeybindings(map[string]any{
		"toggleEmbeddedTerminal": []any{},
		"toggleRightSidebar":     "Ctrl+B",
	}, "/tmp/keybindings.json")

	if got := snapshot.Bindings["toggleEmbeddedTerminal"][0]; got != "cmd+j" {
		t.Fatalf("expected default embedded terminal binding, got %q", got)
	}
	if snapshot.Warning == nil {
		t.Fatal("expected warning for invalid entries")
	}
}

func TestNormalizeKeybindingsKeepsValidShortcutArrays(t *testing.T) {
	snapshot := NormalizeKeybindings(map[string]any{
		"toggleEmbeddedTerminal":     []any{" Cmd+K ", "Ctrl+`"},
		"toggleRightSidebar":         []any{"Ctrl+Shift+B"},
		"openInFinder":               []any{"Cmd+Alt+F"},
		"openInEditor":               []any{"Cmd+Shift+O"},
		"addSplitTerminal":           []any{"Cmd+Shift+J"},
		"jumpToSidebarChat":          []any{"Cmd+Alt"},
		"createChatInCurrentProject": []any{"Cmd+Alt+N"},
		"openAddProject":             []any{"Cmd+Alt+O"},
	}, "/tmp/keybindings.json")

	if got := snapshot.Bindings["toggleEmbeddedTerminal"]; len(got) != 2 || got[0] != "cmd+k" || got[1] != "ctrl+`" {
		t.Fatalf("unexpected normalized binding: %#v", got)
	}
	if snapshot.Warning != nil {
		t.Fatalf("expected no warning, got %q", *snapshot.Warning)
	}
}

func TestLoadKeybindingsSnapshotCreatesDefaultsWhenMissing(t *testing.T) {
	withTempKeybindingsStateDir(t)

	snapshot, err := LoadKeybindingsSnapshot()
	if err != nil {
		t.Fatalf("LoadKeybindingsSnapshot returned error: %v", err)
	}
	if snapshot.Warning != nil {
		t.Fatalf("expected no warning, got %q", *snapshot.Warning)
	}
	data, err := os.ReadFile(GetKeybindingsFilePath())
	if err != nil {
		t.Fatalf("expected keybindings file to be created: %v", err)
	}
	var persisted map[string][]string
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("persisted keybindings are invalid JSON: %v", err)
	}
	if persisted["toggleRightSidebar"][0] != "cmd+b" {
		t.Fatalf("expected defaults on disk, got %#v", persisted["toggleRightSidebar"])
	}
}

func TestSaveKeybindingsWritesNormalizedBindings(t *testing.T) {
	withTempKeybindingsStateDir(t)

	snapshot, err := SaveKeybindings(map[string][]string{
		"toggleEmbeddedTerminal":     {"Cmd+K"},
		"toggleRightSidebar":         {"Ctrl+Shift+B"},
		"openInFinder":               {"Cmd+Alt+F"},
		"openInEditor":               {"Cmd+Shift+O"},
		"addSplitTerminal":           {"Cmd+Shift+J"},
		"jumpToSidebarChat":          {"Cmd+Alt"},
		"createChatInCurrentProject": {"Cmd+Alt+N"},
		"openAddProject":             {"Cmd+Alt+O"},
	})
	if err != nil {
		t.Fatalf("SaveKeybindings returned error: %v", err)
	}
	if got := snapshot.Bindings["toggleEmbeddedTerminal"][0]; got != "cmd+k" {
		t.Fatalf("expected normalized binding cmd+k, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "keybindings.json")); err != nil {
		t.Fatalf("expected keybindings file to exist: %v", err)
	}
}
