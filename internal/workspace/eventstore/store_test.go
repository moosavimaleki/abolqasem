package eventstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/readmodels"
)

func TestAppendReplay(t *testing.T) {
	store := New(t.TempDir())

	event, err := events.NewAt(events.TypeProjectOpened, 1234, map[string]string{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "project",
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}

	if err := store.Append(events.StreamProjects, event); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	replayed, err := store.Replay(events.StreamProjects)
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(replayed) != 1 {
		t.Fatalf("expected 1 event, got %d", len(replayed))
	}
	if replayed[0].Type != events.TypeProjectOpened {
		t.Fatalf("expected %q, got %q", events.TypeProjectOpened, replayed[0].Type)
	}
	if replayed[0].Timestamp != 1234 {
		t.Fatalf("expected timestamp 1234, got %d", replayed[0].Timestamp)
	}
	var data map[string]string
	if err := replayed[0].DecodeData(&data); err != nil {
		t.Fatalf("DecodeData returned error: %v", err)
	}
	if data["projectId"] != "project-1" {
		t.Fatalf("expected project id, got %#v", data)
	}

	raw, err := os.ReadFile(filepath.Join(store.Dir(), events.StreamProjects+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	line := strings.TrimSpace(string(raw))
	if !strings.Contains(line, `"projectId":"project-1"`) {
		t.Fatalf("expected Abolqasem-style top-level projectId, got %s", line)
	}
	if strings.Contains(line, `"data"`) {
		t.Fatalf("event should not wrap fields in data: %s", line)
	}
}

func TestReplayMissingStream(t *testing.T) {
	store := New(t.TempDir())
	replayed, err := store.Replay(events.StreamProjects)
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if len(replayed) != 0 {
		t.Fatalf("expected no events, got %d", len(replayed))
	}
}

func TestInvalidStream(t *testing.T) {
	store := New(t.TempDir())
	event, err := events.NewAt(events.TypeProjectOpened, 1234, map[string]string{"projectId": "project-1"})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	err = store.Append("bad", event)
	if !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("expected ErrInvalidStream, got %v", err)
	}
}

func TestCompactWritesSnapshotAndClearsLogs(t *testing.T) {
	store := New(t.TempDir())
	state := readmodels.EmptyState()
	state.ProjectsByID["project-1"] = readmodels.ProjectRecord{
		ID:        "project-1",
		LocalPath: "/tmp/project",
		Title:     "project",
		CreatedAt: 100,
		UpdatedAt: 200,
	}
	state.ChatsByID["chat-1"] = readmodels.ChatRecord{
		ID:        "chat-1",
		ProjectID: "project-1",
		Title:     "New Chat",
		CreatedAt: 101,
		UpdatedAt: 201,
		Unread:    true,
	}

	event, err := events.NewAt(events.TypeProjectOpened, 100, map[string]string{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "project",
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(events.StreamProjects, event); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	if err := store.Compact(state); err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(store.Dir(), SnapshotFileName))
	if err != nil {
		t.Fatalf("ReadFile snapshot returned error: %v", err)
	}
	var snapshot SnapshotFile
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("Unmarshal snapshot returned error: %v", err)
	}
	if snapshot.V != events.Version {
		t.Fatalf("expected snapshot version %d, got %d", events.Version, snapshot.V)
	}
	if len(snapshot.Projects) != 1 || snapshot.Projects[0].ID != "project-1" {
		t.Fatalf("unexpected projects snapshot: %#v", snapshot.Projects)
	}
	if len(snapshot.Chats) != 1 || snapshot.Chats[0].ID != "chat-1" {
		t.Fatalf("unexpected chats snapshot: %#v", snapshot.Chats)
	}

	logData, err := os.ReadFile(filepath.Join(store.Dir(), events.StreamProjects+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile log returned error: %v", err)
	}
	if len(logData) != 0 {
		t.Fatalf("expected compacted log to be empty, got %q", string(logData))
	}
}

func TestCompactSkipsFullTranscriptForTmuxChats(t *testing.T) {
	store := New(t.TempDir())
	state := readmodels.EmptyState()
	state.ProjectsByID["project-1"] = readmodels.ProjectRecord{
		ID:        "project-1",
		LocalPath: "/tmp/project",
		Title:     "project",
		CreatedAt: 100,
		UpdatedAt: 200,
	}
	state.ChatsByID["chat-1"] = readmodels.ChatRecord{
		ID:            "chat-1",
		ProjectID:     "project-1",
		Title:         "Tmux Chat",
		TmuxSession:   "abolqasem-chat-1",
		HasMessages:   true,
		LastMessageAt: 300,
		CreatedAt:     101,
		UpdatedAt:     300,
	}
	messageAppended, err := events.NewAt(events.TypeMessageAppended, 300, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "msg-1",
			"kind":      "user_prompt",
			"content":   "this should stay out of compacted events",
			"createdAt": int64(300),
		},
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(events.StreamMessages, messageAppended); err != nil {
		t.Fatalf("Append message returned error: %v", err)
	}

	if err := store.Compact(state); err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}
	replayed, err := store.Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay messages returned error: %v", err)
	}
	if len(replayed) != 0 {
		t.Fatalf("expected compacted tmux chat to omit transcript events, got %#v", replayed)
	}
	archives, err := filepath.Glob(filepath.Join(store.Dir(), events.StreamMessages+".jsonl.archived-*"))
	if err != nil {
		t.Fatalf("Glob archives returned error: %v", err)
	}
	if len(archives) != 1 {
		t.Fatalf("expected one archived messages stream, got %#v", archives)
	}
	archived, err := os.ReadFile(archives[0])
	if err != nil {
		t.Fatalf("ReadFile archive returned error: %v", err)
	}
	if !strings.Contains(string(archived), "this should stay out of compacted events") {
		t.Fatalf("expected archive to preserve original messages stream, got %s", string(archived))
	}
}

func TestLoadStateAppliesSnapshotThenEvents(t *testing.T) {
	store := New(t.TempDir())
	state := readmodels.EmptyState()
	state.ProjectsByID["project-1"] = readmodels.ProjectRecord{
		ID:        "project-1",
		LocalPath: "/tmp/project",
		Title:     "project",
		CreatedAt: 100,
		UpdatedAt: 100,
	}
	state.ChatsByID["chat-1"] = readmodels.ChatRecord{
		ID:        "chat-1",
		ProjectID: "project-1",
		Title:     "New Chat",
		CreatedAt: 101,
		UpdatedAt: 101,
	}
	if err := store.Compact(state); err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}

	renamed, err := events.NewAt(events.TypeChatRenamed, 200, map[string]string{
		"chatId": "chat-1",
		"title":  "Renamed Chat",
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(events.StreamChats, renamed); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	loaded, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	chat := loaded.ChatsByID["chat-1"]
	if chat.Title != "Renamed Chat" {
		t.Fatalf("expected replayed title, got %q", chat.Title)
	}
	if chat.UpdatedAt != 200 {
		t.Fatalf("expected replayed updatedAt 200, got %d", chat.UpdatedAt)
	}
}

func TestLoadStateLightSkipsMessageReplay(t *testing.T) {
	store := New(t.TempDir())

	projectOpened, err := events.NewAt(events.TypeProjectOpened, 100, map[string]string{
		"projectId": "project-1",
		"localPath": "/tmp/project",
		"title":     "project",
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(events.StreamProjects, projectOpened); err != nil {
		t.Fatalf("Append project returned error: %v", err)
	}

	chatCreated, err := events.NewAt(events.TypeChatCreated, 110, map[string]string{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "New Chat",
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(events.StreamChats, chatCreated); err != nil {
		t.Fatalf("Append chat returned error: %v", err)
	}

	messageAppended, err := events.NewAt(events.TypeMessageAppended, 120, map[string]any{
		"chatId": "chat-1",
		"entry": readmodels.TranscriptEntry{
			"_id":       "msg-1",
			"kind":      "user_prompt",
			"content":   "hello",
			"createdAt": int64(120),
		},
	})
	if err != nil {
		t.Fatalf("NewAt returned error: %v", err)
	}
	if err := store.Append(events.StreamMessages, messageAppended); err != nil {
		t.Fatalf("Append message returned error: %v", err)
	}

	loaded, err := store.LoadStateLight()
	if err != nil {
		t.Fatalf("LoadStateLight returned error: %v", err)
	}
	chat := loaded.ChatsByID["chat-1"]
	if chat.LastMessageAt != 0 {
		t.Fatalf("expected light load to skip message replay, got lastMessageAt=%d", chat.LastMessageAt)
	}
}

func TestReplayMessagesForChatFiltersEvents(t *testing.T) {
	store := New(t.TempDir())

	appendMessage := func(chatID string, entryID string, timestamp int64) {
		t.Helper()
		event, err := events.NewAt(events.TypeMessageAppended, timestamp, map[string]any{
			"chatId": chatID,
			"entry": readmodels.TranscriptEntry{
				"_id":       entryID,
				"kind":      "user_prompt",
				"content":   entryID,
				"createdAt": timestamp,
			},
		})
		if err != nil {
			t.Fatalf("NewAt returned error: %v", err)
		}
		if err := store.Append(events.StreamMessages, event); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	appendMessage("chat-1", "msg-1", 100)
	appendMessage("chat-2", "msg-2", 101)
	appendMessage("chat-1", "msg-3", 102)

	replayed, err := store.ReplayMessagesForChat("chat-1")
	if err != nil {
		t.Fatalf("ReplayMessagesForChat returned error: %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("expected 2 events, got %d", len(replayed))
	}
	for _, event := range replayed {
		var data struct {
			ChatID string `json:"chatId"`
		}
		if err := event.DecodeData(&data); err != nil {
			t.Fatalf("DecodeData returned error: %v", err)
		}
		if data.ChatID != "chat-1" {
			t.Fatalf("expected only chat-1 events, got %#v", data)
		}
	}
}

func TestReplayMessagesForChatDropsSupersededCheckpointRestores(t *testing.T) {
	store := New(t.TempDir())

	appendEvent := func(eventType string, timestamp int64, data map[string]any) {
		t.Helper()
		event, err := events.NewAt(eventType, timestamp, data)
		if err != nil {
			t.Fatalf("NewAt returned error: %v", err)
		}
		if err := store.Append(events.StreamMessages, event); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	entry := func(id string, timestamp int64) readmodels.TranscriptEntry {
		return readmodels.TranscriptEntry{
			"_id":       id,
			"kind":      "user_prompt",
			"content":   id,
			"createdAt": timestamp,
		}
	}

	appendEvent(events.TypeMessageAppended, 100, map[string]any{"chatId": "chat-1", "entry": entry("before", 100)})
	appendEvent(events.TypeChatRestoredToCheckpoint, 101, map[string]any{"chatId": "chat-1", "messages": []readmodels.TranscriptEntry{entry("old-restore", 101)}})
	appendEvent(events.TypeMessageAppended, 102, map[string]any{"chatId": "chat-2", "entry": entry("other-chat", 102)})
	appendEvent(events.TypeChatRestoredToCheckpoint, 103, map[string]any{"chatId": "chat-1", "messages": []readmodels.TranscriptEntry{entry("new-restore", 103)}})
	appendEvent(events.TypeMessageAppended, 104, map[string]any{"chatId": "chat-1", "entry": entry("after", 104)})

	replayed, err := store.ReplayMessagesForChat("chat-1")
	if err != nil {
		t.Fatalf("ReplayMessagesForChat returned error: %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("expected latest restore and following append, got %d events", len(replayed))
	}
	if replayed[0].Type != events.TypeChatRestoredToCheckpoint || replayed[1].Type != events.TypeMessageAppended {
		t.Fatalf("unexpected replayed event types: %#v", []string{replayed[0].Type, replayed[1].Type})
	}
	var restored struct {
		Messages []readmodels.TranscriptEntry `json:"messages"`
	}
	if err := replayed[0].DecodeData(&restored); err != nil {
		t.Fatalf("DecodeData returned error: %v", err)
	}
	if restored.Messages[0]["_id"] != "new-restore" {
		t.Fatalf("expected latest restore, got %#v", restored.Messages)
	}
}

func TestReplayTranscriptEntriesForChatHonorsTailLimitAfterRestore(t *testing.T) {
	store := New(t.TempDir())

	appendEvent := func(eventType string, timestamp int64, data map[string]any) {
		t.Helper()
		event, err := events.NewAt(eventType, timestamp, data)
		if err != nil {
			t.Fatalf("NewAt returned error: %v", err)
		}
		if err := store.Append(events.StreamMessages, event); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	entry := func(id string, timestamp int64) readmodels.TranscriptEntry {
		return readmodels.TranscriptEntry{
			"_id":       id,
			"kind":      "user_prompt",
			"content":   id,
			"createdAt": timestamp,
		}
	}

	appendEvent(events.TypeMessageAppended, 100, map[string]any{"chatId": "chat-1", "entry": entry("before", 100)})
	appendEvent(events.TypeChatRestoredToCheckpoint, 101, map[string]any{
		"chatId": "chat-1",
		"messages": []readmodels.TranscriptEntry{
			entry("restore-1", 101),
			entry("restore-2", 102),
			entry("restore-3", 103),
		},
	})
	appendEvent(events.TypeMessageAppended, 104, map[string]any{"chatId": "chat-2", "entry": entry("other-chat", 104)})
	appendEvent(events.TypeMessageAppended, 105, map[string]any{"chatId": "chat-1", "entry": entry("after", 105)})

	allEntries, err := store.ReplayTranscriptEntriesForChat("chat-1", 0)
	if err != nil {
		t.Fatalf("ReplayTranscriptEntriesForChat returned error: %v", err)
	}
	assertTranscriptEntryIDs(t, allEntries, []string{"restore-1", "restore-2", "restore-3", "after"})

	tailEntries, err := store.ReplayTranscriptEntriesForChat("chat-1", 3)
	if err != nil {
		t.Fatalf("ReplayTranscriptEntriesForChat tail returned error: %v", err)
	}
	assertTranscriptEntryIDs(t, tailEntries, []string{"restore-2", "restore-3", "after"})
}

func TestLastMessageEventForChatUsesNewestMatchingEvent(t *testing.T) {
	store := New(t.TempDir())

	appendEvent := func(eventType string, chatID string, timestamp int64) {
		t.Helper()
		event, err := events.NewAt(eventType, timestamp, map[string]any{
			"chatId": chatID,
			"entry": readmodels.TranscriptEntry{
				"_id":       chatID,
				"kind":      "user_prompt",
				"createdAt": timestamp,
			},
			"messages": []readmodels.TranscriptEntry{{
				"_id":       chatID + "-restore",
				"kind":      "user_prompt",
				"createdAt": timestamp,
			}},
		})
		if err != nil {
			t.Fatalf("NewAt returned error: %v", err)
		}
		if err := store.Append(events.StreamMessages, event); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	appendEvent(events.TypeChatRestoredToCheckpoint, "chat-1", 100)
	appendEvent(events.TypeMessageAppended, "chat-2", 200)
	appendEvent(events.TypeMessageAppended, "chat-1", 300)

	eventType, timestamp, err := store.LastMessageEventForChat("chat-1")
	if err != nil {
		t.Fatalf("LastMessageEventForChat returned error: %v", err)
	}
	if eventType != events.TypeMessageAppended || timestamp != 300 {
		t.Fatalf("expected latest chat-1 append at 300, got type=%q timestamp=%d", eventType, timestamp)
	}
}

func assertTranscriptEntryIDs(t *testing.T, entries []readmodels.TranscriptEntry, expected []string) {
	t.Helper()
	if len(entries) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %#v", len(expected), len(entries), entries)
	}
	for index, entry := range entries {
		if entry["_id"] != expected[index] {
			t.Fatalf("expected entry %d to be %q, got %#v", index, expected[index], entry)
		}
	}
}
