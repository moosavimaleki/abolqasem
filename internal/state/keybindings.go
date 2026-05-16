package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var keybindingActions = []string{
	"toggleEmbeddedTerminal",
	"toggleRightSidebar",
	"openInFinder",
	"openInEditor",
	"addSplitTerminal",
	"jumpToSidebarChat",
	"createChatInCurrentProject",
	"openAddProject",
}

type KeybindingsSnapshot struct {
	Bindings        map[string][]string `json:"bindings"`
	Warning         *string             `json:"warning"`
	FilePathDisplay string              `json:"filePathDisplay"`
}

func DefaultKeybindings() map[string][]string {
	return cloneKeybindings(map[string][]string{
		"toggleEmbeddedTerminal":     {"cmd+j", "ctrl+`"},
		"toggleRightSidebar":         {"cmd+b", "ctrl+b"},
		"openInFinder":               {"cmd+alt+f", "ctrl+alt+f"},
		"openInEditor":               {"cmd+shift+o", "ctrl+shift+o"},
		"addSplitTerminal":           {"cmd+/", "ctrl+/"},
		"jumpToSidebarChat":          {"cmd+alt"},
		"createChatInCurrentProject": {"cmd+alt+n"},
		"openAddProject":             {"cmd+alt+o"},
	})
}

func GetKeybindingsFilePath() string {
	return filepath.Join(stateDir, "keybindings.json")
}

func LoadKeybindingsSnapshot() (KeybindingsSnapshot, error) {
	path := GetKeybindingsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if writeErr := writeKeybindingsFile(path, DefaultKeybindings()); writeErr != nil {
				return KeybindingsSnapshot{}, writeErr
			}
			return createKeybindingsSnapshot(DefaultKeybindings(), nil, path), nil
		}
		return KeybindingsSnapshot{}, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		warning := "Keybindings file was empty. Using defaults."
		return createKeybindingsSnapshot(DefaultKeybindings(), &warning, path), nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		warning := "Keybindings file is invalid JSON. Using defaults."
		return createKeybindingsSnapshot(DefaultKeybindings(), &warning, path), nil
	}
	return NormalizeKeybindings(parsed, path), nil
}

func SaveKeybindings(bindings map[string][]string) (KeybindingsSnapshot, error) {
	source := map[string]any{}
	for action, values := range bindings {
		copied := make([]any, 0, len(values))
		for _, value := range values {
			copied = append(copied, value)
		}
		source[action] = copied
	}
	snapshot := NormalizeKeybindings(source, GetKeybindingsFilePath())
	if err := writeKeybindingsFile(GetKeybindingsFilePath(), snapshot.Bindings); err != nil {
		return KeybindingsSnapshot{}, err
	}
	return snapshot, nil
}

func NormalizeKeybindings(value map[string]any, path string) KeybindingsSnapshot {
	if value == nil {
		warning := "Keybindings file must contain a JSON object. Using defaults."
		return createKeybindingsSnapshot(DefaultKeybindings(), &warning, path)
	}
	warnings := []string{}
	defaults := DefaultKeybindings()
	bindings := map[string][]string{}
	for _, action := range keybindingActions {
		rawValue, ok := value[action]
		rawList, listOK := rawValue.([]any)
		if !ok {
			bindings[action] = append([]string(nil), defaults[action]...)
			continue
		}
		if !listOK {
			bindings[action] = append([]string(nil), defaults[action]...)
			warnings = append(warnings, action+" must be an array of shortcut strings")
			continue
		}
		normalized := []string{}
		for _, entry := range rawList {
			text, ok := entry.(string)
			if !ok {
				continue
			}
			text = strings.ToLower(strings.TrimSpace(text))
			if text != "" {
				normalized = append(normalized, text)
			}
		}
		if len(normalized) == 0 {
			bindings[action] = append([]string(nil), defaults[action]...)
			if len(rawList) > 0 || ok {
				warnings = append(warnings, action+" did not contain any valid shortcut strings")
			}
			continue
		}
		bindings[action] = normalized
	}
	var warning *string
	if len(warnings) > 0 {
		text := "Some keybindings were reset to defaults: " + strings.Join(warnings, "; ")
		warning = &text
	}
	return createKeybindingsSnapshot(bindings, warning, path)
}

func writeKeybindingsFile(path string, bindings map[string][]string) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return err
	}
	tempPath := path + ".tmp." + time.Now().Format("20060102150405")
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func createKeybindingsSnapshot(bindings map[string][]string, warning *string, path string) KeybindingsSnapshot {
	return KeybindingsSnapshot{
		Bindings:        cloneKeybindings(bindings),
		Warning:         warning,
		FilePathDisplay: formatKeybindingsDisplayPath(path),
	}
}

func cloneKeybindings(bindings map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(bindings))
	for action, values := range bindings {
		cloned[action] = append([]string(nil), values...)
	}
	return cloned
}

func formatKeybindingsDisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + path[len(home):]
	}
	return path
}
