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

func TestImportLegacySessionOpenCodeExportReadsUserAndAssistantText(t *testing.T) {
	root := t.TempDir()
	transcriptPath := filepath.Join(root, "ses_open.json")
	body := `{"info":{"id":"ses_open","directory":"/tmp/project"},"messages":[{"info":{"role":"user","time":{"created":1788012630706}},"parts":[{"type":"text","text":"hello"}]},{"info":{"role":"assistant","time":{"created":1788012630724}},"parts":[{"type":"reasoning","text":"private"},{"type":"text","text":"world"}]}]}`
	if err := os.WriteFile(transcriptPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	result, err := ImportLegacySession(state.SessionMeta{Agent: "opencode", SessionID: "ses_open", TranscriptPath: transcriptPath, Cwd: "/tmp/project"})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(result.Entries) != 2 || result.Entries[0]["content"] != "hello" || result.Entries[1]["text"] != "world" {
		t.Fatalf("entries = %#v", result.Entries)
	}
}

func TestImportLegacySessionCodexCustomExecAsCommandExecution(t *testing.T) {
	root := t.TempDir()
	transcriptPath := filepath.Join(root, "codex-custom-exec.jsonl")
	body := strings.Join([]string{
		`{"timestamp":"2026-08-24T12:23:00.756Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"call_2UlvN95bZTPPJwl0c0cMtOmo","name":"exec","input":"const r = await tools.exec_command({cmd:\"rtk rg -l --hidden --glob '!.git/**' 'سلام' .\",workdir:\"/home/h-mousavi/Projects/Hamed/aistudio-api\",yield_time_ms:10000,max_output_tokens:2000}); text(r.output);"}}`,
		`{"timestamp":"2026-08-24T12:23:00.927Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_2UlvN95bZTPPJwl0c0cMtOmo","output":[{"type":"input_text","text":"Script completed\nWall time 0.2 seconds\nOutput:\n"},{"type":"input_text","text":"./examples/text_basic.py\n./artifacts/nct/026-introduction.md\n"}]}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	result, err := ImportLegacySession(state.SessionMeta{Agent: "codex", SessionID: "codex-custom-exec", TranscriptPath: transcriptPath})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected command start and completion entries, got %d: %#v", len(result.Entries), result.Entries)
	}
	started := result.Entries[0]
	if transcript.Kind(started) != transcript.KindCommandExecution || started["itemId"] != "call_2UlvN95bZTPPJwl0c0cMtOmo" {
		t.Fatalf("unexpected command start entry: %#v", started)
	}
	if started["command"] != `rtk rg -l --hidden --glob '!.git/**' 'سلام' .` || started["cwd"] != "/home/h-mousavi/Projects/Hamed/aistudio-api" || started["status"] != "inProgress" {
		t.Fatalf("command details were not extracted: %#v", started)
	}
	completed := result.Entries[1]
	if transcript.Kind(completed) != transcript.KindCommandExecution || completed["itemId"] != started["itemId"] || completed["status"] != "completed" {
		t.Fatalf("unexpected command completion entry: %#v", completed)
	}
	if !strings.Contains(stringValue(completed["aggregatedOutput"]), "./examples/text_basic.py") {
		t.Fatalf("expected command output, got %#v", completed)
	}
}

func TestImportLegacySessionCodexMCPFunctionCall(t *testing.T) {
	root := t.TempDir()
	transcriptPath := filepath.Join(root, "codex-mcp.jsonl")
	body := strings.Join([]string{
		`{"timestamp":"2026-08-24T12:23:00.756Z","type":"response_item","payload":{"type":"function_call","call_id":"mcp-1","name":"mcp__filesystem__read_file","arguments":"{\"path\":\"/tmp/report.txt\"}"}}`,
		`{"timestamp":"2026-08-24T12:23:00.927Z","type":"response_item","payload":{"type":"function_call_output","call_id":"mcp-1","output":"hello"}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	result, err := ImportLegacySession(state.SessionMeta{Agent: "codex", SessionID: "codex-mcp", TranscriptPath: transcriptPath})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(result.Entries) != 2 || transcript.Kind(result.Entries[0]) != transcript.KindToolCall || transcript.Kind(result.Entries[1]) != transcript.KindToolResult {
		t.Fatalf("unexpected MCP entries: %#v", result.Entries)
	}
	tool := result.Entries[0]["tool"].(map[string]any)
	input := tool["input"].(map[string]any)
	if tool["toolKind"] != "mcp_generic" || input["server"] != "filesystem" || input["tool"] != "read_file" {
		t.Fatalf("unexpected MCP tool: %#v", tool)
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
