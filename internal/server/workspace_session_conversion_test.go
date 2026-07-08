package server

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

func TestWorkspaceExportChatTranscriptWritesNativeTranscript(t *testing.T) {
	withWorkspaceComposerStore(t)
	t.Setenv("CODEX_HOME", t.TempDir())
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatal(err)
	}
	chatID := "chat-export-transcript"
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatCreated, time.Now().UnixMilli(), map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Export Me",
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatProviderSet, time.Now().UnixMilli(), map[string]any{
		"chatId":   chatID,
		"provider": "codex",
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, time.Now().UnixMilli(), map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":     "m1",
			"kind":    transcript.KindUserPrompt,
			"content": "export prompt",
		},
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, time.Now().UnixMilli(), map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":  "m2",
			"kind": transcript.KindAssistantText,
			"text": "export answer",
		},
	})

	raw, _ := json.Marshal(map[string]any{"chatId": chatID, "targetProvider": "codex"})
	result, err := workspaceExportChatTranscript(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "codex" || result.ImportedMessageCount != 2 || result.SessionToken == "" {
		t.Fatalf("unexpected export result: %#v", result)
	}
	if _, err := os.Stat(result.TranscriptPath); err != nil {
		t.Fatalf("expected exported transcript file: %v", err)
	}
}
