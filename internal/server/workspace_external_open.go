package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	openExternalFinder   = "open_finder"
	openExternalTerminal = "open_terminal"
	openExternalEditor   = "open_editor"
	openExternalPreview  = "open_preview"
	openExternalDefault  = "open_default"

	editorPresetCursor   = "cursor"
	editorPresetVSCode   = "vscode"
	editorPresetXcode    = "xcode"
	editorPresetWindsurf = "windsurf"
	editorPresetCustom   = "custom"
)

var defaultExternalEditorSettings = workspaceEditorOpenSettings{
	Preset:          editorPresetCursor,
	CommandTemplate: "cursor {path}",
}

var (
	runExternalCommand      = defaultRunExternalCommand
	hasExternalCommand      = defaultHasExternalCommand
	canOpenExternalMacApp   = defaultCanOpenExternalMacApp
	externalRuntimePlatform = func() string { return runtime.GOOS }
)

type workspaceOpenExternalCommand struct {
	Type      string                       `json:"type"`
	LocalPath string                       `json:"localPath"`
	Action    string                       `json:"action"`
	Line      int                          `json:"line,omitempty"`
	Column    int                          `json:"column,omitempty"`
	Editor    *workspaceEditorOpenSettings `json:"editor,omitempty"`
}

type workspaceEditorOpenSettings struct {
	Preset          string `json:"preset"`
	CommandTemplate string `json:"commandTemplate"`
}

type externalCommandSpec struct {
	Command string
	Args    []string
}

func workspaceOpenExternal(raw json.RawMessage) error {
	var command workspaceOpenExternalCommand
	if err := json.Unmarshal(raw, &command); err != nil {
		return err
	}
	return openWorkspaceExternal(command)
}

func openWorkspaceExternal(command workspaceOpenExternalCommand) error {
	resolvedPath, err := resolveOpenExternalLocalPath(command.LocalPath)
	if err != nil {
		return err
	}
	platform := externalRuntimePlatform()
	info, statErr := openExternalStat(command.Action, resolvedPath)
	if statErr != nil {
		return statErr
	}

	switch command.Action {
	case openExternalEditor:
		if info == nil {
			return fmt.Errorf("Path not found: %s", resolvedPath)
		}
		editor := defaultExternalEditorSettings
		if command.Editor != nil {
			editor = *command.Editor
		}
		spec, err := buildExternalEditorCommand(externalEditorCommandArgs{
			LocalPath:   resolvedPath,
			IsDirectory: info.IsDir(),
			Line:        command.Line,
			Column:      command.Column,
			Editor:      editor,
			Platform:    platform,
		})
		if err != nil {
			return err
		}
		return runExternalCommand(spec.Command, spec.Args)
	case openExternalDefault:
		if info == nil {
			return fmt.Errorf("Path not found: %s", resolvedPath)
		}
		spec := buildExternalDefaultOpenCommand(resolvedPath, platform)
		return runExternalCommand(spec.Command, spec.Args)
	case openExternalPreview:
		if info == nil {
			return fmt.Errorf("Path not found: %s", resolvedPath)
		}
		if platform == "darwin" && !canOpenExternalMacApp("Preview") {
			return errors.New("Preview is not installed")
		}
		spec, err := buildExternalPreviewCommand(resolvedPath, info.IsDir(), platform)
		if err != nil {
			return err
		}
		return runExternalCommand(spec.Command, spec.Args)
	case openExternalFinder:
		spec := buildExternalFinderCommand(resolvedPath, info != nil && info.IsDir(), platform)
		return runExternalCommand(spec.Command, spec.Args)
	case openExternalTerminal:
		spec, err := buildExternalTerminalCommand(resolvedPath, platform)
		if err != nil {
			return err
		}
		return runExternalCommand(spec.Command, spec.Args)
	default:
		return fmt.Errorf("unsupported external open action: %s", command.Action)
	}
}

func openExternalStat(action string, localPath string) (os.FileInfo, error) {
	switch action {
	case openExternalEditor, openExternalFinder, openExternalPreview, openExternalDefault:
		info, err := os.Stat(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		return info, nil
	default:
		return nil, nil
	}
}

func resolveOpenExternalLocalPath(localPath string) (string, error) {
	trimmed := strings.TrimSpace(localPath)
	if trimmed == "" {
		return "", errors.New("Project path is required")
	}
	for _, char := range trimmed {
		if char == 0 || char < 32 {
			return "", errors.New("Local path contains invalid control characters")
		}
	}
	if parsed, err := url.Parse(trimmed); err == nil && disallowedExternalPathScheme(parsed.Scheme) {
		return "", errors.New("Only local filesystem paths can be opened")
	}
	return resolveWorkspaceLocalPath(trimmed), nil
}

func disallowedExternalPathScheme(scheme string) bool {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http", "https", "ws", "wss", "ftp", "file", "data", "javascript":
		return true
	default:
		return false
	}
}

type externalEditorCommandArgs struct {
	LocalPath   string
	IsDirectory bool
	Line        int
	Column      int
	Editor      workspaceEditorOpenSettings
	Platform    string
}

func buildExternalEditorCommand(args externalEditorCommandArgs) (externalCommandSpec, error) {
	editor := normalizeExternalEditorSettings(args.Editor)
	if editor.Preset == editorPresetCustom {
		return buildCustomExternalEditorCommand(editor.CommandTemplate, args.LocalPath, args.Line, args.Column)
	}
	return buildPresetExternalEditorCommand(args, editor.Preset)
}

func buildExternalPreviewCommand(localPath string, isDirectory bool, platform string) (externalCommandSpec, error) {
	if platform != "darwin" {
		return externalCommandSpec{}, errors.New("Preview is only available on macOS")
	}
	if isDirectory {
		return externalCommandSpec{}, errors.New("Preview cannot open directories")
	}
	return externalCommandSpec{Command: "open", Args: []string{"-a", "Preview", localPath}}, nil
}

func buildExternalDefaultOpenCommand(localPath string, platform string) externalCommandSpec {
	switch platform {
	case "darwin":
		return externalCommandSpec{Command: "open", Args: []string{localPath}}
	case "windows", "win32":
		return externalCommandSpec{Command: "cmd", Args: []string{"/c", "start", "", localPath}}
	default:
		return externalCommandSpec{Command: "xdg-open", Args: []string{localPath}}
	}
}

func buildExternalFinderCommand(localPath string, isDirectory bool, platform string) externalCommandSpec {
	switch platform {
	case "darwin":
		if isDirectory {
			return externalCommandSpec{Command: "open", Args: []string{localPath}}
		}
		return externalCommandSpec{Command: "open", Args: []string{"-R", localPath}}
	case "windows", "win32":
		if isDirectory {
			return externalCommandSpec{Command: "explorer", Args: []string{localPath}}
		}
		return externalCommandSpec{Command: "explorer", Args: []string{"/select,", localPath}}
	default:
		target := filepath.Dir(localPath)
		if isDirectory {
			target = localPath
		}
		return externalCommandSpec{Command: "xdg-open", Args: []string{target}}
	}
}

func buildExternalTerminalCommand(localPath string, platform string) (externalCommandSpec, error) {
	switch platform {
	case "darwin":
		if !canOpenExternalMacApp("Terminal") {
			return externalCommandSpec{}, errors.New("Terminal is not installed")
		}
		return externalCommandSpec{Command: "open", Args: []string{"-a", "Terminal", localPath}}, nil
	case "windows", "win32":
		if hasExternalCommand("wt") {
			return externalCommandSpec{Command: "wt", Args: []string{"-d", localPath}}, nil
		}
		return externalCommandSpec{Command: "cmd", Args: []string{"/c", "start", "", "cmd", "/K", "cd", "/d", localPath}}, nil
	default:
		for _, terminalCommand := range []string{"x-terminal-emulator", "gnome-terminal", "konsole"} {
			if !hasExternalCommand(terminalCommand) {
				continue
			}
			switch terminalCommand {
			case "gnome-terminal":
				return externalCommandSpec{Command: terminalCommand, Args: []string{"--working-directory", localPath}}, nil
			case "konsole":
				return externalCommandSpec{Command: terminalCommand, Args: []string{"--workdir", localPath}}, nil
			default:
				return externalCommandSpec{Command: terminalCommand, Args: []string{"--working-directory", localPath}}, nil
			}
		}
		return externalCommandSpec{Command: "xdg-open", Args: []string{localPath}}, nil
	}
}

func buildPresetExternalEditorCommand(args externalEditorCommandArgs, preset string) (externalCommandSpec, error) {
	opener, err := resolveExternalEditorExecutable(preset, args.Platform)
	if err != nil {
		return externalCommandSpec{}, err
	}
	if preset == editorPresetXcode {
		if args.IsDirectory || args.Line <= 0 || opener.Command != "xed" {
			return externalCommandSpec{Command: opener.Command, Args: appendCopy(opener.Args, args.LocalPath)}, nil
		}
		return externalCommandSpec{Command: opener.Command, Args: appendCopy(opener.Args, "-l", fmt.Sprintf("%d", args.Line), args.LocalPath)}, nil
	}
	if args.IsDirectory || args.Line <= 0 {
		return externalCommandSpec{Command: opener.Command, Args: appendCopy(opener.Args, args.LocalPath)}, nil
	}
	line := args.Line
	column := args.Column
	if line <= 0 {
		line = 1
	}
	if column <= 0 {
		column = 1
	}
	gotoTarget := fmt.Sprintf("%s:%d:%d", args.LocalPath, line, column)
	return externalCommandSpec{Command: opener.Command, Args: appendCopy(opener.Args, "--goto", gotoTarget)}, nil
}

func resolveExternalEditorExecutable(preset string, platform string) (externalCommandSpec, error) {
	switch preset {
	case editorPresetCursor:
		if hasExternalCommand("cursor") {
			return externalCommandSpec{Command: "cursor"}, nil
		}
		if platform == "darwin" && canOpenExternalMacApp("Cursor") {
			return externalCommandSpec{Command: "open", Args: []string{"-a", "Cursor"}}, nil
		}
	case editorPresetVSCode:
		if hasExternalCommand("code") {
			return externalCommandSpec{Command: "code"}, nil
		}
		if platform == "darwin" && canOpenExternalMacApp("Visual Studio Code") {
			return externalCommandSpec{Command: "open", Args: []string{"-a", "Visual Studio Code"}}, nil
		}
	case editorPresetWindsurf:
		if hasExternalCommand("windsurf") {
			return externalCommandSpec{Command: "windsurf"}, nil
		}
		if platform == "darwin" && canOpenExternalMacApp("Windsurf") {
			return externalCommandSpec{Command: "open", Args: []string{"-a", "Windsurf"}}, nil
		}
	case editorPresetXcode:
		if hasExternalCommand("xed") {
			return externalCommandSpec{Command: "xed"}, nil
		}
		if platform == "darwin" && canOpenExternalMacApp("Xcode") {
			return externalCommandSpec{Command: "open", Args: []string{"-a", "Xcode"}}, nil
		}
	}

	if platform == "darwin" {
		switch preset {
		case editorPresetCursor:
			return externalCommandSpec{}, errors.New("Cursor is not installed")
		case editorPresetVSCode:
			return externalCommandSpec{}, errors.New("Visual Studio Code is not installed")
		case editorPresetWindsurf:
			return externalCommandSpec{}, errors.New("Windsurf is not installed")
		case editorPresetXcode:
			return externalCommandSpec{}, errors.New("Xcode is not installed")
		}
	}

	switch preset {
	case editorPresetVSCode:
		return externalCommandSpec{Command: "code"}, nil
	case editorPresetXcode:
		return externalCommandSpec{Command: "xed"}, nil
	case editorPresetCursor, editorPresetWindsurf:
		return externalCommandSpec{Command: preset}, nil
	default:
		return externalCommandSpec{Command: editorPresetCursor}, nil
	}
}

func buildCustomExternalEditorCommand(commandTemplate string, localPath string, line int, column int) (externalCommandSpec, error) {
	template := strings.TrimSpace(commandTemplate)
	if !strings.Contains(template, "{path}") {
		return externalCommandSpec{}, errors.New("Custom editor command must include {path}")
	}
	if line <= 0 {
		line = 1
	}
	if column <= 0 {
		column = 1
	}
	replaced := strings.ReplaceAll(template, "{path}", localPath)
	replaced = strings.ReplaceAll(replaced, "{line}", fmt.Sprintf("%d", line))
	replaced = strings.ReplaceAll(replaced, "{column}", fmt.Sprintf("%d", column))
	tokens, err := tokenizeExternalCommandTemplate(replaced)
	if err != nil {
		return externalCommandSpec{}, err
	}
	if len(tokens) == 0 {
		return externalCommandSpec{}, errors.New("Custom editor command is empty")
	}
	return externalCommandSpec{Command: tokens[0], Args: tokens[1:]}, nil
}

func tokenizeExternalCommandTemplate(template string) ([]string, error) {
	tokens := []string{}
	current := strings.Builder{}
	var quote rune
	runes := []rune(template)
	for index := 0; index < len(runes); index++ {
		char := runes[index]
		if char == '\\' && index+1 < len(runes) {
			current.WriteRune(runes[index+1])
			index++
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(char)
	}
	if quote != 0 {
		return nil, errors.New("Custom editor command has an unclosed quote")
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens, nil
}

func normalizeExternalEditorSettings(editor workspaceEditorOpenSettings) workspaceEditorOpenSettings {
	preset := normalizeExternalEditorPreset(editor.Preset)
	commandTemplate := strings.TrimSpace(editor.CommandTemplate)
	if commandTemplate == "" {
		commandTemplate = defaultExternalEditorSettings.CommandTemplate
	}
	return workspaceEditorOpenSettings{
		Preset:          preset,
		CommandTemplate: commandTemplate,
	}
}

func normalizeExternalEditorPreset(preset string) string {
	switch preset {
	case editorPresetVSCode, editorPresetXcode, editorPresetWindsurf, editorPresetCustom, editorPresetCursor:
		return preset
	default:
		return defaultExternalEditorSettings.Preset
	}
}

func appendCopy(base []string, values ...string) []string {
	result := make([]string, 0, len(base)+len(values))
	result = append(result, base...)
	result = append(result, values...)
	return result
}

func defaultRunExternalCommand(command string, args []string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return formatExternalSpawnError(command, err)
	}
	return cmd.Process.Release()
}

func formatExternalSpawnError(command string, err error) error {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Command not found: %s", command)
	}
	return fmt.Errorf("Failed to start %s: %w", command, err)
}

func defaultHasExternalCommand(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func defaultCanOpenExternalMacApp(appName string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return exec.Command("open", "-Ra", appName).Run() == nil
}
