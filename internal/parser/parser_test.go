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

func TestParseMessagesCodexDedupesImageBoundaryResponseItem(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"timestamp":"2026-08-24T14:05:07.286Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# Files mentioned by the user:\n\n## image.png: /tmp/image.png\n\n## My request for Codex:\n\nپادشاه این جاست؟"},{"type":"input_text","text":"<image name=[Image #1] path=\"/tmp/image.png\">"},{"type":"input_image","image_url":"data:image/png;base64,do-not-render"},{"type":"input_text","text":"</image>"}]}}`,
		`{"timestamp":"2026-08-24T14:05:07.286Z","type":"event_msg","payload":{"type":"user_message","message":"# Files mentioned by the user:\n\n## image.png: /tmp/image.png\n\n## My request for Codex:\n\nپادشاه این جاست؟","local_images":["/tmp/image.png"]}}`,
	}, "\n"))

	result, err := ParseMessages("codex", "session-1", path, ParseOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ParseMessages returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one deduped image prompt, got %d: %#v", len(result.Items), result.Items)
	}
	if strings.Contains(result.Items[0].Text, "<image") || strings.Contains(result.Items[0].Text, "</image>") {
		t.Fatalf("expected image transport markup hidden, got %#v", result.Items[0])
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

func TestStreamSearchableMessagesCodexKeepsCustomExecEvents(t *testing.T) {
	path := writeTranscript(t, strings.Join([]string{
		`{"timestamp":"2026-08-24T12:23:00.756Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call_2UlvN95bZTPPJwl0c0cMtOmo","name":"exec","input":"const r = await tools.exec_command({cmd:\"rtk rg -l --hidden --glob '!.git/**' 'سلام' .\",workdir:\"/home/h-mousavi/Projects/Hamed/aistudio-api\",yield_time_ms:10000,max_output_tokens:2000}); text(r.output);"}}`,
		`{"timestamp":"2026-08-24T12:23:00.927Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_2UlvN95bZTPPJwl0c0cMtOmo","output":[{"type":"input_text","text":"Script completed\nWall time 0.2 seconds\nOutput:\n"},{"type":"input_text","text":"./examples/text_basic.py\n./artifacts/nct/026-introduction.md\n"}]}}`,
	}, "\n"))

	messages := []SearchableMessage{}
	if err := StreamSearchableMessages("codex", "session-1", path, func(message SearchableMessage) bool {
		messages = append(messages, message)
		return true
	}); err != nil {
		t.Fatalf("StreamSearchableMessages returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected command start and completion, got %d: %#v", len(messages), messages)
	}
	if messages[0].Kind != "command_execution" || messages[1].Kind != "command_execution" {
		t.Fatalf("expected native command events, got %#v", messages)
	}
	if messages[0].Fields["command"] != `rtk rg -l --hidden --glob '!.git/**' 'سلام' .` || messages[0].Fields["cwd"] != "/home/h-mousavi/Projects/Hamed/aistudio-api" {
		t.Fatalf("expected command and cwd fields, got %#v", messages[0])
	}
	if messages[1].Fields["status"] != "completed" || !strings.Contains(stringValue(messages[1].Fields["aggregatedOutput"]), "./examples/text_basic.py") {
		t.Fatalf("expected completed command output, got %#v", messages[1])
	}
}

func TestStreamSearchableMessagesCodexMapsUpdatePlanToNativePlan(t *testing.T) {
	path := writeTranscript(t, `{"timestamp":"2026-08-24T12:23:00.756Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call-plan","name":"exec","input":"const p = await tools.update_plan({explanation:\"Ship safely\",plan:[{step:\"Inspect\",status:\"completed\"},{step:\"Test\",status:\"inProgress\"}]}); text(p);","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}}`+"\n")
	messages := []SearchableMessage{}
	if err := StreamSearchableMessages("codex", "session-1", path, func(message SearchableMessage) bool {
		messages = append(messages, message)
		return true
	}); err != nil {
		t.Fatalf("StreamSearchableMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].Kind != "turn_plan" || messages[0].Fields["turnId"] != "turn-1" {
		t.Fatalf("expected native plan event, got %#v", messages)
	}
	plan, ok := messages[0].Fields["plan"].([]map[string]string)
	if !ok || len(plan) != 2 || plan[1]["status"] != "inProgress" {
		t.Fatalf("expected parsed plan steps, got %#v", messages[0].Fields)
	}
}

func TestStreamSearchableMessagesCodexMapsPatchApplyEndToFileChange(t *testing.T) {
	path := writeTranscript(t, `{"timestamp":"2026-08-24T14:01:29.578Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"exec-1","turn_id":"turn-1","success":true,"status":"completed","changes":{"/workspace/renamed.md":{"type":"update","unified_diff":"@@ -1 +1 @@\n-old\n+new\n","move_path":"/workspace/final.md"},"/workspace/new.go":{"type":"add","unified_diff":"@@ -0,0 +1 @@\n+package main\n","move_path":null}}}}`+"\n")
	messages := []SearchableMessage{}
	if err := StreamSearchableMessages("codex", "session-1", path, func(message SearchableMessage) bool {
		messages = append(messages, message)
		return true
	}); err != nil {
		t.Fatalf("StreamSearchableMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].Kind != "file_change" {
		t.Fatalf("expected native file-change event, got %#v", messages)
	}
	if messages[0].Fields["itemId"] != "exec-1" || messages[0].Fields["status"] != "completed" {
		t.Fatalf("expected file-change identity and status, got %#v", messages[0].Fields)
	}
	changes, ok := messages[0].Fields["changes"].([]map[string]any)
	if !ok || len(changes) != 2 || changes[0]["path"] != "/workspace/new.go" || changes[1]["movedToPath"] != "/workspace/final.md" {
		t.Fatalf("expected sorted changes with diff metadata, got %#v", messages[0].Fields["changes"])
	}
	if !strings.Contains(stringValue(changes[1]["diff"]), "+new") {
		t.Fatalf("expected unified diff, got %#v", changes[1])
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
