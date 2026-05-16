package eventstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-agent-manager/internal/workspace/events"
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
