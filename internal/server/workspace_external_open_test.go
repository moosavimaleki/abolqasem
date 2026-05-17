package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTokenizeExternalCommandTemplateKeepsQuotedArgumentsTogether(t *testing.T) {
	tokens, err := tokenizeExternalCommandTemplate(`code --reuse-window "{path}"`)
	if err != nil {
		t.Fatalf("tokenizeExternalCommandTemplate returned error: %v", err)
	}
	expected := []string{"code", "--reuse-window", "{path}"}
	if !reflect.DeepEqual(tokens, expected) {
		t.Fatalf("unexpected tokens: %#v", tokens)
	}
}

func TestBuildExternalEditorCommandBuildsPresetGotoCommand(t *testing.T) {
	withExternalCommandMocks(t, externalCommandMocks{})
	command, err := buildExternalEditorCommand(externalEditorCommandArgs{
		LocalPath:   "/Users/jake/Projects/abolqasem/src/client/app/App.tsx",
		IsDirectory: false,
		Line:        12,
		Column:      3,
		Editor: workspaceEditorOpenSettings{
			Preset:          editorPresetVSCode,
			CommandTemplate: "code {path}",
		},
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("buildExternalEditorCommand returned error: %v", err)
	}
	expected := externalCommandSpec{
		Command: "code",
		Args:    []string{"--goto", "/Users/jake/Projects/abolqasem/src/client/app/App.tsx:12:3"},
	}
	if !reflect.DeepEqual(command, expected) {
		t.Fatalf("unexpected editor command: %#v", command)
	}
}

func TestBuildExternalEditorCommandBuildsDirectoryProjectCommand(t *testing.T) {
	withExternalCommandMocks(t, externalCommandMocks{})
	command, err := buildExternalEditorCommand(externalEditorCommandArgs{
		LocalPath:   "/Users/jake/Projects/abolqasem",
		IsDirectory: true,
		Editor: workspaceEditorOpenSettings{
			Preset:          editorPresetCursor,
			CommandTemplate: "cursor {path}",
		},
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("buildExternalEditorCommand returned error: %v", err)
	}
	expected := externalCommandSpec{Command: "cursor", Args: []string{"/Users/jake/Projects/abolqasem"}}
	if !reflect.DeepEqual(command, expected) {
		t.Fatalf("unexpected editor command: %#v", command)
	}
}

func TestBuildExternalEditorCommandUsesCustomTemplate(t *testing.T) {
	command, err := buildExternalEditorCommand(externalEditorCommandArgs{
		LocalPath:   "/Users/jake/Projects/abolqasem/src/client/app/App.tsx",
		IsDirectory: false,
		Line:        12,
		Column:      1,
		Editor: workspaceEditorOpenSettings{
			Preset:          editorPresetCustom,
			CommandTemplate: `my-editor "{path}" --line {line}`,
		},
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("buildExternalEditorCommand returned error: %v", err)
	}
	expected := externalCommandSpec{
		Command: "my-editor",
		Args:    []string{"/Users/jake/Projects/abolqasem/src/client/app/App.tsx", "--line", "12"},
	}
	if !reflect.DeepEqual(command, expected) {
		t.Fatalf("unexpected editor command: %#v", command)
	}
}

func TestBuildExternalEditorCommandBuildsXcodeLineCommand(t *testing.T) {
	withExternalCommandMocks(t, externalCommandMocks{})
	command, err := buildExternalEditorCommand(externalEditorCommandArgs{
		LocalPath:   "/Users/jake/Projects/abolqasem/App.swift",
		IsDirectory: false,
		Line:        24,
		Column:      2,
		Editor: workspaceEditorOpenSettings{
			Preset:          editorPresetXcode,
			CommandTemplate: "xed {path}",
		},
		Platform: "linux",
	})
	if err != nil {
		t.Fatalf("buildExternalEditorCommand returned error: %v", err)
	}
	expected := externalCommandSpec{Command: "xed", Args: []string{"-l", "24", "/Users/jake/Projects/abolqasem/App.swift"}}
	if !reflect.DeepEqual(command, expected) {
		t.Fatalf("unexpected editor command: %#v", command)
	}
}

func TestBuildExternalPreviewCommand(t *testing.T) {
	command, err := buildExternalPreviewCommand("/Users/jake/Projects/abolqasem/mock.png", false, "darwin")
	if err != nil {
		t.Fatalf("buildExternalPreviewCommand returned error: %v", err)
	}
	expected := externalCommandSpec{Command: "open", Args: []string{"-a", "Preview", "/Users/jake/Projects/abolqasem/mock.png"}}
	if !reflect.DeepEqual(command, expected) {
		t.Fatalf("unexpected preview command: %#v", command)
	}
	if _, err := buildExternalPreviewCommand("/Users/jake/Projects/abolqasem/mock.png", false, "linux"); err == nil || !strings.Contains(err.Error(), "Preview is only available on macOS") {
		t.Fatalf("expected non-macOS preview error, got %v", err)
	}
}

func TestBuildExternalDefaultOpenCommand(t *testing.T) {
	mac := buildExternalDefaultOpenCommand("/Users/jake/Projects/abolqasem/mock.png", "darwin")
	if !reflect.DeepEqual(mac, externalCommandSpec{Command: "open", Args: []string{"/Users/jake/Projects/abolqasem/mock.png"}}) {
		t.Fatalf("unexpected mac command: %#v", mac)
	}
	linux := buildExternalDefaultOpenCommand("/tmp/mock.png", "linux")
	if !reflect.DeepEqual(linux, externalCommandSpec{Command: "xdg-open", Args: []string{"/tmp/mock.png"}}) {
		t.Fatalf("unexpected linux command: %#v", linux)
	}
}

func TestWorkspaceOpenExternalRunsEditorCommandWithLineColumn(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "example file.go")
	writeTestFile(t, filePath, "package main\n")

	var ran externalCommandSpec
	withExternalCommandMocks(t, externalCommandMocks{
		run: func(command string, args []string) error {
			ran = externalCommandSpec{Command: command, Args: append([]string{}, args...)}
			return nil
		},
	})

	raw, _ := json.Marshal(map[string]any{
		"type":      "system.openExternal",
		"localPath": filePath,
		"action":    openExternalEditor,
		"line":      31,
		"column":    4,
		"editor": map[string]any{
			"preset":          editorPresetCustom,
			"commandTemplate": `safe-editor "{path}" --line {line} --column {column}`,
		},
	})
	if err := workspaceOpenExternal(raw); err != nil {
		t.Fatalf("workspaceOpenExternal returned error: %v", err)
	}
	expected := externalCommandSpec{Command: "safe-editor", Args: []string{filePath, "--line", "31", "--column", "4"}}
	if !reflect.DeepEqual(ran, expected) {
		t.Fatalf("unexpected executed command: %#v", ran)
	}
}

func TestResolveOpenExternalLocalPathRejectsNonLocalTargets(t *testing.T) {
	if _, err := resolveOpenExternalLocalPath("https://example.com/file.go"); err == nil || !strings.Contains(err.Error(), "Only local filesystem paths") {
		t.Fatalf("expected URL path rejection, got %v", err)
	}
	if _, err := resolveOpenExternalLocalPath("safe\x00name.go"); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("expected control character rejection, got %v", err)
	}
}

type externalCommandMocks struct {
	run       func(string, []string) error
	has       func(string) bool
	canMacApp func(string) bool
	platform  func() string
}

func withExternalCommandMocks(t *testing.T, mocks externalCommandMocks) {
	t.Helper()
	previousRun := runExternalCommand
	previousHas := hasExternalCommand
	previousCanMacApp := canOpenExternalMacApp
	previousPlatform := externalRuntimePlatform
	runExternalCommand = func(command string, args []string) error {
		if mocks.run != nil {
			return mocks.run(command, args)
		}
		return nil
	}
	hasExternalCommand = func(command string) bool {
		if mocks.has != nil {
			return mocks.has(command)
		}
		return false
	}
	canOpenExternalMacApp = func(app string) bool {
		if mocks.canMacApp != nil {
			return mocks.canMacApp(app)
		}
		return false
	}
	externalRuntimePlatform = func() string {
		if mocks.platform != nil {
			return mocks.platform()
		}
		return "linux"
	}
	t.Cleanup(func() {
		runExternalCommand = previousRun
		hasExternalCommand = previousHas
		canOpenExternalMacApp = previousCanMacApp
		externalRuntimePlatform = previousPlatform
	})
}

func writeTestFile(t *testing.T, filePath string, content string) {
	t.Helper()
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}
