package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMessagesCodex(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"سلام دنیا"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"| a | b |\n| - | - |\n| 1 | 2 |"}}`,
		`{"type":"event_msg","payload":{"type":"command_output","output":"ls -la"}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Items))
	}
	if result.Items[0].Role != "user" {
		t.Fatalf("expected first role user, got %s", result.Items[0].Role)
	}
	if !strings.Contains(result.Items[1].HTML, "<table>") {
		t.Fatalf("expected markdown table html, got %s", result.Items[1].HTML)
	}
	if result.Items[2].Role != "tool" {
		t.Fatalf("expected tool role, got %s", result.Items[2].Role)
	}
}

func TestParseMessagesClaudeArrayContent(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
		`{"message":{"role":"assistant","content":[{"type":"text","text":"سلام از کلود"},{"type":"tool_use","name":"bash","arguments":{"command":"pwd"}}]}}`,
	}, "\n"))

	result, err := ParseMessages("claude", "claude-session", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Items))
	}
	if !strings.Contains(result.Items[1].Text, "سلام از کلود") {
		t.Fatalf("expected assistant text, got %q", result.Items[1].Text)
	}
}

func TestParseMessagesGeminiAndPagination(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"role":"user","prompt":"hello"}`,
		`{"role":"assistant","response":"one"}`,
		`{"role":"assistant","response":"two"}`,
	}, "\n"))

	result, err := ParseMessages("gemini", "gemini-session", path, ParseOptions{Limit: 1, Before: "3"})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Text != "one" {
		t.Fatalf("unexpected paginated result: %+v", result.Items)
	}
}

func TestParseMessagesGeminiStructuredJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-2026-05-14.json")
	body := `{"history":[{"role":"user","parts":[{"text":"hello"}]},{"role":"model","parts":[{"text":"answer"}]}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	result, err := ParseMessages("gemini", "gemini-session", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Items))
	}
	if result.Items[1].Role != "assistant" || result.Items[1].Text != "answer" {
		t.Fatalf("unexpected structured message: %+v", result.Items[1])
	}
}

func TestParseMessagesMetadataOnly(t *testing.T) {
	result, err := ParseMessages("gemini", "missing", "", ParseOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected transcript unavailable error")
	}
	if result == nil || result.Status != "metadata_only" {
		t.Fatalf("expected metadata_only status, got %+v", result)
	}
}

func TestGetSessionSummaryIncludesFirstPreview(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"first prompt text"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"assistant answer"}}`,
	}, "\n"))

	summary, err := GetSessionSummary("codex", "session-1", path)
	if err != nil {
		t.Fatalf("GetSessionSummary returned error: %v", err)
	}
	if summary.FirstPreview != "first prompt text" {
		t.Fatalf("expected first preview, got %q", summary.FirstPreview)
	}
	if summary.LastPreview != "assistant answer" {
		t.Fatalf("expected last preview, got %q", summary.LastPreview)
	}
}

func TestGetSessionSummaryPrefersFirstUserPreview(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"agent_message","message":"assistant preface"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"first user prompt"}}`,
	}, "\n"))

	summary, err := GetSessionSummary("codex", "session-1", path)
	if err != nil {
		t.Fatalf("GetSessionSummary returned error: %v", err)
	}
	if summary.FirstPreview != "first user prompt" {
		t.Fatalf("expected first user preview, got %q", summary.FirstPreview)
	}
}

func TestSearchMessagesStreamsMatches(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"first prompt text"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"assistant has needle inside a longer answer"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"another needle answer"}}`,
	}, "\n"))

	result, err := SearchMessages("codex", "session-1", path, SearchOptions{
		Query:        "needle",
		Limit:        1,
		SnippetRunes: 40,
	})
	if err != nil {
		t.Fatalf("SearchMessages returned error: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	if result.Matches[0].Role != "assistant" || !strings.Contains(result.Matches[0].Snippet, "needle") {
		t.Fatalf("unexpected match: %+v", result.Matches[0])
	}
}

func writeTranscript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}
