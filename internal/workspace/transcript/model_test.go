package transcript

import "testing"

func TestNewCreatesKannaTranscriptEntryShape(t *testing.T) {
	entry := New(KindAssistantText, map[string]any{"text": "hello"})
	if Kind(entry) != KindAssistantText {
		t.Fatalf("expected assistant kind, got %q", Kind(entry))
	}
	if entry["_id"] == "" {
		t.Fatalf("expected id, got %#v", entry)
	}
	if entry["createdAt"] == nil {
		t.Fatalf("expected createdAt, got %#v", entry)
	}
	if entry["text"] != "hello" {
		t.Fatalf("expected text field, got %#v", entry)
	}
}
