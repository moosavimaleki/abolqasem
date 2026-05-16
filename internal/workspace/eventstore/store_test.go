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
		t.Fatalf("expected Kanna-style top-level projectId, got %s", line)
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
