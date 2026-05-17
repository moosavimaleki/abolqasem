package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeProjectQuickActions(t *testing.T) {
	result := normalizeProjectQuickActions(map[string]any{
		"quickActions": []any{
			map[string]any{"id": "dev", "label": "Dev", "command": "bun run dev"},
			map[string]any{"id": "dev", "label": "Duplicate", "command": "echo duplicate"},
			map[string]any{"id": "empty", "label": "Empty", "command": " "},
			map[string]any{"id": "test", "command": "bun test"},
		},
	})

	expected := []projectQuickAction{
		{ID: "dev", Label: "Dev", Command: "bun run dev"},
		{ID: "test", Label: "bun test", Command: "bun test"},
	}
	if !quickActionsEqual(result, expected) {
		t.Fatalf("unexpected normalized quick actions: %#v", result)
	}
}

func TestProjectQuickActionsTruncateAndLimit(t *testing.T) {
	longLabel := strings.Repeat("ل", maxQuickActionLabelLength+5)
	longCommand := strings.Repeat("x", maxQuickActionCommandLength+5)
	raw := make([]any, 0, maxProjectQuickActions+2)
	raw = append(raw, map[string]any{"id": "long", "label": longLabel, "command": longCommand})
	for index := 0; index < maxProjectQuickActions+1; index++ {
		raw = append(raw, map[string]any{
			"id":      "action-" + string(rune('a'+index)),
			"command": "echo ok",
		})
	}

	result := normalizeProjectQuickActions(raw)
	if len(result) != maxProjectQuickActions {
		t.Fatalf("expected %d actions, got %d", maxProjectQuickActions, len(result))
	}
	if len([]rune(result[0].Label)) != maxQuickActionLabelLength {
		t.Fatalf("expected label to be truncated to %d runes, got %d", maxQuickActionLabelLength, len([]rune(result[0].Label)))
	}
	if len([]rune(result[0].Command)) != maxQuickActionCommandLength {
		t.Fatalf("expected command to be truncated to %d runes, got %d", maxQuickActionCommandLength, len([]rune(result[0].Command)))
	}
}

func TestWorkspaceProjectQuickActionsPersistInAbolqasemDir(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := t.TempDir()
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}

	raw, _ := json.Marshal(map[string]any{
		"projectId": project.ID,
		"quickActions": []map[string]string{{
			"id":      "dev",
			"label":   "Dev",
			"command": "bun run dev",
		}},
	})
	written, err := workspaceWriteProjectQuickActions(raw)
	if err != nil {
		t.Fatalf("workspaceWriteProjectQuickActions returned error: %v", err)
	}
	expected := []projectQuickAction{{ID: "dev", Label: "Dev", Command: "bun run dev"}}
	if !quickActionsEqual(written, expected) {
		t.Fatalf("unexpected written quick actions: %#v", written)
	}

	filePath := filepath.Join(projectDir, ".abolqasem", "quick-actions.json")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected quick-actions file at %s: %v", filePath, err)
	}
	readRaw, _ := json.Marshal(map[string]any{"projectId": project.ID})
	read, err := workspaceReadProjectQuickActions(readRaw)
	if err != nil {
		t.Fatalf("workspaceReadProjectQuickActions returned error: %v", err)
	}
	if !quickActionsEqual(read, expected) {
		t.Fatalf("unexpected read quick actions: %#v", read)
	}
}

func TestWorkspaceProjectQuickActionsRequireProject(t *testing.T) {
	withWorkspaceComposerStore(t)
	raw, _ := json.Marshal(map[string]any{"projectId": "missing"})
	if _, err := workspaceReadProjectQuickActions(raw); err == nil {
		t.Fatal("expected missing project to return an error")
	}
}

func quickActionsEqual(left, right []projectQuickAction) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
