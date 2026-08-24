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

func TestParseMessagesCodexDedupesResponseItemEventPairs(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"timestamp":"2026-07-04T10:39:08.589Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"سلام خوبی؟"}]}}`,
		`{"timestamp":"2026-07-04T10:39:08.589Z","type":"event_msg","payload":{"type":"user_message","message":"سلام خوبی؟","images":[]}}`,
		`{"timestamp":"2026-07-04T10:39:09.100Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"سلام، خوبم."}]}}`,
		`{"timestamp":"2026-07-04T10:39:09.100Z","type":"event_msg","payload":{"type":"agent_message","message":"سلام، خوبم."}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 deduped messages, got %d: %#v", len(result.Items), result.Items)
	}
	if result.Items[0].Role != "user" || result.Items[0].Text != "سلام خوبی؟" {
		t.Fatalf("unexpected user message: %#v", result.Items[0])
	}
	if result.Items[1].Role != "assistant" || result.Items[1].Text != "سلام، خوبم." {
		t.Fatalf("unexpected assistant message: %#v", result.Items[1])
	}

	search, err := SearchMessages("codex", "session-1", path, SearchOptions{Query: "سلام", Limit: 10})
	if err != nil {
		t.Fatalf("SearchMessages returned error: %v", err)
	}
	if len(search.Matches) != 2 {
		t.Fatalf("expected search to see deduped messages, got %d: %#v", len(search.Matches), search.Matches)
	}
}

func TestParseMessagesCodexKeepsRealRepeatedUserPrompts(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"timestamp":"2026-07-04T10:39:08.589Z","type":"event_msg","payload":{"type":"user_message","message":"سلام خوبی؟"}}`,
		`{"timestamp":"2026-07-04T10:39:21.864Z","type":"event_msg","payload":{"type":"user_message","message":"سلام خوبی؟"}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected repeated real prompts to remain, got %d: %#v", len(result.Items), result.Items)
	}
}

func TestParseMessagesCodexSkipsEncryptedAndImageContentBlocks(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"response_item","payload":{"type":"agent_message","content":[{"type":"input_text","text":"پاسخ قابل نمایش"},{"type":"encrypted_content","encrypted_content":"gAAAA-should-not-render"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"تصویر را بررسی کن"},{"type":"input_image","image_url":"data:image/png;base64,should-not-render"},{"type":"input_text","text":"data:image/png;base64,also-should-not-render"}]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"gAAAA-reasoning-should-not-render"}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected visible text messages only, got %d: %#v", len(result.Items), result.Items)
	}
	if result.Items[0].Role != "assistant" || result.Items[0].Text != "پاسخ قابل نمایش" {
		t.Fatalf("unexpected assistant message: %#v", result.Items[0])
	}
	if result.Items[1].Role != "user" || result.Items[1].Text != "تصویر را بررسی کن" {
		t.Fatalf("unexpected user message: %#v", result.Items[1])
	}
	for _, item := range result.Items {
		if strings.Contains(item.Text, "should-not-render") || strings.Contains(item.Text, "encrypted_content") {
			t.Fatalf("internal Codex payload leaked into transcript: %#v", item)
		}
	}
}

func TestParseMessagesCodexSkipsInterAgentMessages(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"agent_message","message":"پاسخ واقعی دستیار"}}`,
		`{"type":"response_item","payload":{"type":"agent_message","author":"/root/reviewer","recipient":"/root","content":[{"type":"input_text","text":"Message Type: FINAL_ANSWER\nPayload: خروجی داخلی"}]}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected only the user-facing assistant answer, got %d: %#v", len(result.Items), result.Items)
	}
	if result.Items[0].Role != "assistant" || result.Items[0].Text != "پاسخ واقعی دستیار" {
		t.Fatalf("unexpected visible message: %#v", result.Items[0])
	}
}

func TestParseMessagesCodexCurrentResponseItemFormatKeepsOnlyConversation(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"internal runtime instruction"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"پرسش واقعی کاربر"},{"type":"input_image","image_url":"data:image/png;base64,do-not-render"}]}}`,
		`{"type":"response_item","payload":{"type":"reasoning","encrypted_content":"gAAAA-do-not-render"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"پاسخ واقعی دستیار"}]}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"reasoning","encrypted_content":"gAAAA-do-not-render"}}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected only user and assistant messages, got %d: %#v", len(result.Items), result.Items)
	}
	if result.Items[0].Role != "user" || result.Items[0].Text != "پرسش واقعی کاربر" {
		t.Fatalf("unexpected user message: %#v", result.Items[0])
	}
	if result.Items[1].Role != "assistant" || result.Items[1].Text != "پاسخ واقعی دستیار" {
		t.Fatalf("unexpected assistant message: %#v", result.Items[1])
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
