package legacyimport

import (
	"testing"
	"time"

	"ai-agent-manager/internal/parser"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/transcript"
)

func TestImportSessionMapsLegacyParserMessagesToReadOnlyChat(t *testing.T) {
	created := time.UnixMilli(1_700_000_000_000)
	updated := created.Add(2 * time.Minute)
	meta := state.SessionMeta{
		Key:            "codex:session-1",
		Agent:          "codex",
		SessionID:      "session-1",
		TranscriptPath: "/tmp/project/session.jsonl",
		Cwd:            "/tmp/project",
		ProjectName:    "project",
		UpdatedAt:      updated,
	}
	messages := []parser.Message{
		{ID: "m1", Role: "user", Text: "سلام", Index: 1, CreatedAt: &created},
		{ID: "m2", Role: "assistant", Text: "پاسخ", Index: 2, CreatedAt: &updated},
	}

	imported := ImportSession(meta, messages, ImportOptions{})

	if !imported.ReadOnly {
		t.Fatal("expected legacy import to be read-only")
	}
	if imported.Project.Title != "project" || imported.Project.LocalPath != "/tmp/project" {
		t.Fatalf("unexpected project: %#v", imported.Project)
	}
	if imported.Chat.Title != "سلام" {
		t.Fatalf("expected chat title from first message, got %q", imported.Chat.Title)
	}
	if imported.Chat.Provider == nil || *imported.Chat.Provider != "codex" {
		t.Fatalf("expected codex provider, got %#v", imported.Chat.Provider)
	}
	if len(imported.Transcript.Messages) != 2 {
		t.Fatalf("expected 2 transcript entries, got %d", len(imported.Transcript.Messages))
	}
	if transcript.Kind(imported.Transcript.Messages[0]) != transcript.KindUserPrompt {
		t.Fatalf("expected user prompt, got %#v", imported.Transcript.Messages[0])
	}
	if imported.Transcript.Messages[0]["content"] != "سلام" {
		t.Fatalf("expected user content, got %#v", imported.Transcript.Messages[0]["content"])
	}
	if transcript.Kind(imported.Transcript.Messages[1]) != transcript.KindAssistantText {
		t.Fatalf("expected assistant text, got %#v", imported.Transcript.Messages[1])
	}
	if imported.Transcript.Messages[1]["text"] != "پاسخ" {
		t.Fatalf("expected assistant text, got %#v", imported.Transcript.Messages[1]["text"])
	}
}

func TestLegacyChatIDDeduplicatesByAgentAndTranscriptPath(t *testing.T) {
	first := state.SessionMeta{
		Key:            "codex:rollout-1",
		Agent:          "codex",
		SessionID:      "rollout-1",
		TranscriptPath: "/tmp/project/session.jsonl",
	}
	second := state.SessionMeta{
		Key:            "codex:session-1",
		Agent:          "codex",
		SessionID:      "session-1",
		TranscriptPath: "/tmp/project/session.jsonl",
	}

	if LegacyChatID(first) != LegacyChatID(second) {
		t.Fatalf("expected same legacy chat id for same transcript path")
	}
}

func TestImportSessionRecentLimitKeepsNewestEntries(t *testing.T) {
	meta := state.SessionMeta{
		Key:            "claude:session-1",
		Agent:          "claude",
		SessionID:      "session-1",
		TranscriptPath: "/tmp/project/session.jsonl",
		Cwd:            "/tmp/project",
		ProjectName:    "project",
	}
	messages := []parser.Message{
		{ID: "m1", Role: "user", Text: "one", Index: 1},
		{ID: "m2", Role: "assistant", Text: "two", Index: 2},
		{ID: "m3", Role: "user", Text: "three", Index: 3},
	}

	imported := ImportSession(meta, messages, ImportOptions{RecentLimit: 2})

	if !imported.Transcript.History.HasOlder {
		t.Fatal("expected older history marker")
	}
	if imported.Transcript.History.OlderCursor == nil || *imported.Transcript.History.OlderCursor == "" {
		t.Fatalf("expected older cursor, got %#v", imported.Transcript.History.OlderCursor)
	}
	if len(imported.Transcript.Messages) != 2 {
		t.Fatalf("expected 2 recent entries, got %d", len(imported.Transcript.Messages))
	}
	if imported.Transcript.Messages[0]["messageId"] != "m2" {
		t.Fatalf("expected newest slice to start at m2, got %#v", imported.Transcript.Messages[0])
	}
}
