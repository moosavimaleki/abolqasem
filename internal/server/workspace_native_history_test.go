package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

func TestWorkspaceNativeHistoryDoesNotReadStoredMessagesStream(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := t.TempDir()
	nativePath := filepath.Join(t.TempDir(), "native.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"first native prompt"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"first native answer"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"second native prompt"}}` + "\n"
	if err := os.WriteFile(nativePath, []byte(body), 0o644); err != nil {
		t.Fatalf("write native transcript: %v", err)
	}
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	store := &workspaceEventStore{store: workspaceStore()}
	chat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("CreateChat returned error: %v", err)
	}
	if err := store.SetChatProvider(chat.ID, "codex"); err != nil {
		t.Fatalf("SetChatProvider returned error: %v", err)
	}
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatRuntimeSet, time.Now().UnixMilli(), map[string]any{
		"chatId":               chat.ID,
		"nativeSessionId":      "native-session",
		"nativeTranscriptPath": nativePath,
	}); err != nil {
		t.Fatalf("append runtime metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDataDir(), events.StreamMessages+".jsonl"), []byte("{bad json\n"), 0o644); err != nil {
		t.Fatalf("write bad messages stream: %v", err)
	}

	raw, _ := json.Marshal(map[string]any{"chatId": chat.ID})
	indexSnapshot, err := workspaceReadChatTranscriptIndex(raw)
	if err != nil {
		t.Fatalf("workspaceReadChatTranscriptIndex returned error: %v", err)
	}
	items := indexSnapshot["items"].([]workspaceTranscriptIndexItem)
	if len(items) != 3 || items[0].Preview != "first native prompt" {
		t.Fatalf("unexpected native transcript index: %#v", items)
	}

	history, err := workspaceLoadStoredChatHistory(chat.ID, "", 2)
	if err != nil {
		t.Fatalf("workspaceLoadStoredChatHistory returned error: %v", err)
	}
	messages := history["messages"].([]readmodels.TranscriptEntry)
	if len(messages) != 2 || transcript.Kind(messages[0]) != transcript.KindAssistantText || transcript.Kind(messages[1]) != transcript.KindUserPrompt {
		t.Fatalf("unexpected native history page: %#v", messages)
	}
	if history["hasOlder"] != true {
		t.Fatalf("expected native history to report older entries, got %#v", history)
	}

	around, err := workspaceLoadStoredChatHistoryAround(chat.ID, items[1].ID, 3)
	if err != nil {
		t.Fatalf("workspaceLoadStoredChatHistoryAround returned error: %v", err)
	}
	aroundMessages := around["messages"].([]readmodels.TranscriptEntry)
	if around["targetFound"] != true || len(aroundMessages) != 3 {
		t.Fatalf("unexpected native history around result: %#v", around)
	}
}
