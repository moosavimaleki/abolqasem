package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	projectQuickActionsFileName = "quick-actions.json"
	maxProjectQuickActions      = 50
	maxQuickActionLabelLength   = 80
	maxQuickActionCommandLength = 2000
)

type projectQuickAction struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Command string `json:"command"`
}

func workspaceReadProjectQuickActions(raw json.RawMessage) ([]projectQuickAction, error) {
	projectPath, err := workspaceProjectPathFromCommand(raw)
	if err != nil {
		return nil, err
	}
	return readProjectQuickActions(projectPath)
}

func workspaceWriteProjectQuickActions(raw json.RawMessage) ([]projectQuickAction, error) {
	var payload struct {
		ProjectID    string `json:"projectId"`
		QuickActions any    `json:"quickActions"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	projectPath, err := workspaceProjectLocalPathRequired(payload.ProjectID)
	if err != nil {
		return nil, err
	}
	return writeProjectQuickActions(projectPath, normalizeProjectQuickActions(payload.QuickActions))
}

func workspaceProjectPathFromCommand(raw json.RawMessage) (string, error) {
	var payload struct {
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	return workspaceProjectLocalPathRequired(payload.ProjectID)
}

func workspaceProjectLocalPathRequired(projectID string) (string, error) {
	project, err := workspaceRuntimeProjectRequired(projectID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(project.LocalPath) == "" {
		return "", errors.New("project not found")
	}
	return resolveWorkspaceLocalPath(project.LocalPath), nil
}

func projectQuickActionsPath(projectPath string) string {
	return filepath.Join(resolveWorkspaceLocalPath(projectPath), ".abolqasem", projectQuickActionsFileName)
}

func readProjectQuickActions(projectPath string) ([]projectQuickAction, error) {
	data, err := os.ReadFile(projectQuickActionsPath(projectPath))
	if err != nil {
		if os.IsNotExist(err) {
			return []projectQuickAction{}, nil
		}
		return nil, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return []projectQuickAction{}, nil
	}
	return normalizeProjectQuickActions(value), nil
}

func writeProjectQuickActions(projectPath string, quickActions []projectQuickAction) ([]projectQuickAction, error) {
	normalized := normalizeProjectQuickActions(quickActions)
	filePath := projectQuickActionsPath(projectPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(map[string][]projectQuickAction{"quickActions": normalized}, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeProjectQuickActions(value any) []projectQuickAction {
	rawActions := projectQuickActionsArray(value)
	seenIDs := map[string]bool{}
	actions := make([]projectQuickAction, 0, len(rawActions))
	for _, rawAction := range rawActions {
		action, ok := normalizeProjectQuickAction(rawAction)
		if !ok || seenIDs[action.ID] {
			continue
		}
		seenIDs[action.ID] = true
		actions = append(actions, action)
		if len(actions) >= maxProjectQuickActions {
			break
		}
	}
	return actions
}

func projectQuickActionsArray(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []projectQuickAction:
		raw := make([]any, 0, len(typed))
		for _, action := range typed {
			raw = append(raw, action)
		}
		return raw
	case map[string]any:
		if actions, ok := typed["quickActions"].([]any); ok {
			return actions
		}
	}
	return nil
}

func normalizeProjectQuickAction(value any) (projectQuickAction, bool) {
	var id, label, command string
	switch typed := value.(type) {
	case projectQuickAction:
		id = typed.ID
		label = typed.Label
		command = typed.Command
	case map[string]any:
		id, _ = typed["id"].(string)
		label, _ = typed["label"].(string)
		command, _ = typed["command"].(string)
	default:
		return projectQuickAction{}, false
	}
	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	command = strings.TrimSpace(command)
	if id == "" || command == "" {
		return projectQuickAction{}, false
	}
	if label == "" {
		label = command
	}
	return projectQuickAction{
		ID:      id,
		Label:   truncateRunes(label, maxQuickActionLabelLength),
		Command: truncateRunes(command, maxQuickActionCommandLength),
	}, true
}

func resolveWorkspaceLocalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				path = home
			} else {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func truncateRunes(value string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return string(runes[:maxLength])
}
