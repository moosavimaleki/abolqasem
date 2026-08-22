package sessioninterop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/transcript"
)

func TestImportLegacySessionCodexCurrentFormatSkipsInternalAndImageBlocks(t *testing.T) {
	root := t.TempDir()
	transcriptPath := filepath.Join(root, "codex-current.jsonl")
	body := strings.Join([]string{
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"internal instruction"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"تصویر را بررسی کن"},{"type":"input_image","image_url":"data:image/png;base64,do-not-render"},{"type":"input_text","text":"data:image/png;base64,also-do-not-render"}]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"gAAAA-do-not-render"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"تصویر را بررسی کردم"}]}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	result, err := ImportLegacySession(state.SessionMeta{Agent: "codex", SessionID: "codex-current", TranscriptPath: transcriptPath})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected only user and assistant entries, got %d: %#v", len(result.Entries), result.Entries)
	}
	if result.Entries[0]["content"] != "تصویر را بررسی کن" {
		t.Fatalf("unexpected user entry: %#v", result.Entries[0])
	}
	if result.Entries[1]["text"] != "تصویر را بررسی کردم" {
		t.Fatalf("unexpected assistant entry: %#v", result.Entries[1])
	}
}

func TestImportLegacySessionGeminiStructuredPreservesMessagesAndTools(t *testing.T) {
	root := t.TempDir()
	transcriptPath := filepath.Join(root, "session.json")
	body := `{
  "sessionId": "gem-session",
  "summary": "Earlier work was compacted.",
  "messages": [
    {"id":"u1","timestamp":"2025-12-29T04:04:34.450Z","type":"user","content":"hello"},
    {"id":"g1","timestamp":"2025-12-29T04:04:37.997Z","type":"gemini","content":"I am reading the file","model":"gemini-3-flash-preview","toolCalls":[{"id":"read-1","name":"read_file","args":{"file_path":"src/app.py"},"result":"file contents","status":"success"}]},
    {"id":"i1","timestamp":"2025-12-29T04:04:39.000Z","type":"info","content":"Conversation checkpoint saved with tag: sync."}
  ]
}`
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	result, err := ImportLegacySession(state.SessionMeta{
		Agent:          "gemini",
		SessionID:      "gem-session",
		TranscriptPath: transcriptPath,
		Cwd:            "/tmp/project",
		ProjectName:    "project",
	})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(result.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(result.Entries))
	}
	if transcript.Kind(result.Entries[0]) != transcript.KindCompactSummary {
		t.Fatalf("expected compact summary first, got %s", transcript.Kind(result.Entries[0]))
	}
	if transcript.Kind(result.Entries[1]) != transcript.KindUserPrompt {
		t.Fatalf("expected user prompt second, got %s", transcript.Kind(result.Entries[1]))
	}
	if transcript.Kind(result.Entries[2]) != transcript.KindAssistantText {
		t.Fatalf("expected assistant text third, got %s", transcript.Kind(result.Entries[2]))
	}
	if transcript.Kind(result.Entries[3]) != transcript.KindToolCall {
		t.Fatalf("expected tool call fourth, got %s", transcript.Kind(result.Entries[3]))
	}
	if transcript.Kind(result.Entries[4]) != transcript.KindToolResult {
		t.Fatalf("expected tool result fifth, got %s", transcript.Kind(result.Entries[4]))
	}
	if transcript.Kind(result.Entries[5]) != transcript.KindCompactBoundary {
		t.Fatalf("expected compact boundary sixth, got %s", transcript.Kind(result.Entries[5]))
	}
}

func TestImportLegacySessionClaudeDerivesTitleAndPreservesToolBlocks(t *testing.T) {
	root := t.TempDir()
	transcriptPath := filepath.Join(root, "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699.jsonl")
	body := `{"type":"user","message":{"role":"user","content":"ببین این claude-flow چیه؟"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"دارم بررسی می‌کنم"},{"type":"tool_use","id":"call_1","name":"TaskCreate","input":{"description":"Locate claude-flow artifacts"}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"Task #1 created successfully"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	result, err := ImportLegacySession(state.SessionMeta{
		Agent:          "claude",
		SessionID:      "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		SessionName:    "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		TranscriptPath: transcriptPath,
		Cwd:            "/tmp/project",
		ProjectName:    "project",
	})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if result.SessionName != "ببین این claude-flow چیه" {
		t.Fatalf("expected derived title from first user prompt, got %q", result.SessionName)
	}
	if len(result.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(result.Entries))
	}
	if transcript.Kind(result.Entries[0]) != transcript.KindUserPrompt {
		t.Fatalf("expected user prompt first, got %s", transcript.Kind(result.Entries[0]))
	}
	if transcript.Kind(result.Entries[1]) != transcript.KindAssistantText {
		t.Fatalf("expected assistant text second, got %s", transcript.Kind(result.Entries[1]))
	}
	if transcript.Kind(result.Entries[2]) != transcript.KindToolCall {
		t.Fatalf("expected tool call third, got %s", transcript.Kind(result.Entries[2]))
	}
	if result.Entries[2]["tool"] == nil {
		t.Fatalf("expected structured tool payload, got %#v", result.Entries[2])
	}
	if transcript.Kind(result.Entries[3]) != transcript.KindToolResult {
		t.Fatalf("expected tool result fourth, got %s", transcript.Kind(result.Entries[3]))
	}
}
