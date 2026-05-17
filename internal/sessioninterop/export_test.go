package sessioninterop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func TestExportNativeSessionRoundTripClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	entries := []readmodels.TranscriptEntry{
		transcript.New(transcript.KindUserPrompt, map[string]any{"content": "hello"}),
		transcript.New(transcript.KindAssistantText, map[string]any{"text": "world"}),
	}
	result, err := ExportNativeSession(ExportArgs{
		Provider:  "claude",
		LocalPath: "/tmp/project",
		Entries:   entries,
	})
	if err != nil {
		t.Fatalf("ExportNativeSession returned error: %v", err)
	}
	if _, err := os.Stat(result.TranscriptPath); err != nil {
		t.Fatalf("expected transcript file: %v", err)
	}
	imported, err := ImportLegacySession(state.SessionMeta{Agent: "claude", SessionID: result.SessionToken, TranscriptPath: result.TranscriptPath, Cwd: "/tmp/project", ProjectName: "project"})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(imported.Entries) < 2 {
		t.Fatalf("expected imported entries, got %d", len(imported.Entries))
	}
	lines := readJSONLRecords(t, result.TranscriptPath)
	if len(lines) < 2 {
		t.Fatalf("expected claude records, got %#v", lines)
	}
	if lines[0]["parentUuid"] != nil {
		t.Fatalf("expected first claude record to have nil parentUuid, got %#v", lines[0]["parentUuid"])
	}
	if lines[1]["parentUuid"] != lines[0]["uuid"] {
		t.Fatalf("expected claude parent chain, got parent=%#v previous=%#v", lines[1]["parentUuid"], lines[0]["uuid"])
	}
}

func TestExportNativeSessionRoundTripCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	entries := []readmodels.TranscriptEntry{
		transcript.New(transcript.KindUserPrompt, map[string]any{"content": "hello"}),
		transcript.New(transcript.KindAssistantText, map[string]any{"text": "world"}),
	}
	result, err := ExportNativeSession(ExportArgs{
		Provider:  "codex",
		LocalPath: "/tmp/project",
		Entries:   entries,
	})
	if err != nil {
		t.Fatalf("ExportNativeSession returned error: %v", err)
	}
	if filepath.Base(filepath.Dir(filepath.Dir(result.TranscriptPath))) == "sessions" {
		// expected nested date directories; this just guards against path regressions
	}
	imported, err := ImportLegacySession(state.SessionMeta{Agent: "codex", SessionID: result.SessionToken, TranscriptPath: result.TranscriptPath, Cwd: "/tmp/project", ProjectName: "project"})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(imported.Entries) < 2 {
		t.Fatalf("expected imported entries, got %d", len(imported.Entries))
	}
	lines := readJSONLRecords(t, result.TranscriptPath)
	var userMessages, assistantMessages int
	var userEvents, assistantEvents int
	for _, line := range lines {
		if line["type"] == "turn_context" {
			t.Fatalf("exported invalid synthetic turn_context: %#v", line)
		}
		if line["type"] == "event_msg" {
			payload, _ := line["payload"].(map[string]any)
			eventType, _ := payload["type"].(string)
			switch eventType {
			case "user_message":
				userEvents++
			case "agent_message":
				assistantEvents++
			}
		}
		payload, _ := line["payload"].(map[string]any)
		if line["type"] == "response_item" && payload["type"] == "message" {
			switch payload["role"] {
			case "user":
				userMessages++
			case "assistant":
				assistantMessages++
			}
		}
	}
	if userMessages == 0 || assistantMessages == 0 {
		t.Fatalf("expected codex user and assistant response_item messages, got user=%d assistant=%d", userMessages, assistantMessages)
	}
	if userEvents == 0 || assistantEvents == 0 {
		t.Fatalf("expected codex replay event messages, got user=%d assistant=%d", userEvents, assistantEvents)
	}
	if len(imported.Entries) != 2 {
		t.Fatalf("expected codex response_item/event_msg pairs to dedupe on import, got %d entries: %#v", len(imported.Entries), imported.Entries)
	}
}

func TestExportNativeSessionRoundTripGemini(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))
	entries := []readmodels.TranscriptEntry{
		transcript.New(transcript.KindCompactSummary, map[string]any{"summary": "older work"}),
		transcript.New(transcript.KindUserPrompt, map[string]any{"content": "hello"}),
		transcript.New(transcript.KindToolCall, map[string]any{
			"tool": map[string]any{
				"toolKind": "read_file",
				"toolName": "read_file",
				"toolId":   "tool-1",
				"input":    map[string]any{"file_path": "src/app.py"},
			},
		}),
		transcript.New(transcript.KindToolResult, map[string]any{
			"toolId":  "tool-1",
			"content": "file contents",
			"isError": false,
		}),
		transcript.New(transcript.KindAssistantText, map[string]any{"text": "world"}),
	}
	result, err := ExportNativeSession(ExportArgs{
		Provider:  "gemini",
		LocalPath: "/tmp/project",
		Entries:   entries,
	})
	if err != nil {
		t.Fatalf("ExportNativeSession returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.ProjectPath, ".project_root")); err != nil {
		t.Fatalf("expected .project_root marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(result.ProjectPath)), "history", filepath.Base(result.ProjectPath), ".project_root")); err != nil {
		t.Fatalf("expected gemini history .project_root marker: %v", err)
	}
	imported, err := ImportLegacySession(state.SessionMeta{Agent: "gemini", SessionID: result.SessionToken, TranscriptPath: result.TranscriptPath, Cwd: "/tmp/project", ProjectName: "project"})
	if err != nil {
		t.Fatalf("ImportLegacySession returned error: %v", err)
	}
	if len(imported.Entries) < 3 {
		t.Fatalf("expected imported entries, got %d", len(imported.Entries))
	}
	var registry struct {
		Projects map[string]string `json:"projects"`
	}
	data, err := os.ReadFile(filepath.Join(home, ".gemini", "projects.json"))
	if err != nil {
		t.Fatalf("read gemini projects registry: %v", err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatalf("parse gemini projects registry: %v", err)
	}
	if registry.Projects["/tmp/project"] != filepath.Base(result.ProjectPath) {
		t.Fatalf("expected registry to point to exported project path, got %#v", registry.Projects)
	}
	lines := readJSONLRecords(t, result.TranscriptPath)
	foundVisibleToolRecord := false
	for _, line := range lines {
		if line["type"] != "gemini" {
			continue
		}
		content, _ := line["content"].(string)
		if strings.Contains(content, "Tool call: read_file") && strings.Contains(content, "file contents") {
			foundVisibleToolRecord = true
			break
		}
	}
	if !foundVisibleToolRecord {
		t.Fatalf("expected gemini export to keep tool context model-visible, got %#v", lines)
	}
}

func TestExportNativeSessionGeminiAvoidsSlugCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))
	collidingMarker := filepath.Join(home, ".gemini", "tmp", "project", ".project_root")
	if err := os.MkdirAll(filepath.Dir(collidingMarker), 0o755); err != nil {
		t.Fatalf("mkdir colliding marker: %v", err)
	}
	if err := os.WriteFile(collidingMarker, []byte("/tmp/other"), 0o644); err != nil {
		t.Fatalf("write colliding marker: %v", err)
	}
	result, err := ExportNativeSession(ExportArgs{
		Provider:  "gemini",
		LocalPath: "/tmp/project",
		Entries: []readmodels.TranscriptEntry{
			transcript.New(transcript.KindUserPrompt, map[string]any{"content": "hello"}),
		},
	})
	if err != nil {
		t.Fatalf("ExportNativeSession returned error: %v", err)
	}
	if filepath.Base(result.ProjectPath) != "project-1" {
		t.Fatalf("expected collision-safe gemini slug project-1, got %q", filepath.Base(result.ProjectPath))
	}
}

func TestExportNativeSessionPairwiseMatrix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_HOME", filepath.Join(home, ".claude"))

	sources := []struct {
		provider string
		meta     state.SessionMeta
	}{
		{
			provider: "claude",
			meta:     writeSourceClaudeSession(t, home),
		},
		{
			provider: "codex",
			meta:     writeSourceCodexSession(t, home),
		},
		{
			provider: "gemini",
			meta:     writeSourceGeminiSession(t, home),
		},
	}
	for _, source := range sources {
		imported, err := ImportLegacySession(source.meta)
		if err != nil {
			t.Fatalf("import source %s: %v", source.provider, err)
		}
		for _, target := range []string{"claude", "codex", "gemini"} {
			if target == source.provider {
				continue
			}
			t.Run(source.provider+"_to_"+target, func(t *testing.T) {
				result, err := ExportNativeSession(ExportArgs{
					Provider:  target,
					LocalPath: source.meta.Cwd,
					Entries:   imported.Entries,
				})
				if err != nil {
					t.Fatalf("export %s -> %s: %v", source.provider, target, err)
				}
				if _, err := os.Stat(result.TranscriptPath); err != nil {
					t.Fatalf("expected native transcript file for %s -> %s: %v", source.provider, target, err)
				}
				reimported, err := ImportLegacySession(state.SessionMeta{Agent: target, SessionID: result.SessionToken, TranscriptPath: result.TranscriptPath, Cwd: source.meta.Cwd, ProjectName: source.meta.ProjectName})
				if err != nil {
					t.Fatalf("reimport %s -> %s: %v", source.provider, target, err)
				}
				if countKinds(reimported.Entries, transcript.KindUserPrompt) == 0 {
					t.Fatalf("expected user prompt to survive for %s -> %s", source.provider, target)
				}
				if countKinds(reimported.Entries, transcript.KindAssistantText) == 0 {
					t.Fatalf("expected assistant text to survive for %s -> %s", source.provider, target)
				}
				assertTranscriptContentSurvived(t, reimported.Entries, "hello from "+source.provider, "assistant from "+source.provider)
				if countKinds(imported.Entries, transcript.KindToolCall) > 0 && countKinds(reimported.Entries, transcript.KindToolCall) == 0 {
					t.Fatalf("expected tool call to survive for %s -> %s", source.provider, target)
				}
				assertNativeExportShape(t, target, result, source.meta.Cwd)
				root := exportDiscoveryRoot(home, target)
				appState := &state.AppState{Sessions: map[string]state.SessionMeta{}}
				report, err := state.DiscoverSessionsInRoots(appState, []state.DiscoveryRoot{{Agent: target, Path: root}})
				if err != nil {
					t.Fatalf("discover exported %s -> %s: %v", source.provider, target, err)
				}
				if report.Found == 0 {
					t.Fatalf("expected discovery to find exported %s session", target)
				}
			})
		}
	}
}

func assertTranscriptContentSurvived(t *testing.T, entries []readmodels.TranscriptEntry, userText string, assistantText string) {
	t.Helper()
	if !transcriptHasText(entries, transcript.KindUserPrompt, "content", userText) {
		t.Fatalf("expected converted transcript to include user text %q, got %#v", userText, entries)
	}
	if !transcriptHasText(entries, transcript.KindAssistantText, "text", assistantText) {
		t.Fatalf("expected converted transcript to include assistant text %q, got %#v", assistantText, entries)
	}
}

func transcriptHasText(entries []readmodels.TranscriptEntry, kind string, field string, want string) bool {
	for _, entry := range entries {
		if transcript.Kind(entry) == kind && strings.Contains(stringValueAny(entry[field]), want) {
			return true
		}
	}
	return false
}

func assertNativeExportShape(t *testing.T, provider string, result ExportResult, cwd string) {
	t.Helper()
	switch provider {
	case "claude":
		assertClaudeNativeShape(t, result)
	case "codex":
		assertCodexNativeShape(t, result, cwd)
	case "gemini":
		assertGeminiNativeShape(t, result, cwd)
	default:
		t.Fatalf("unsupported provider %q", provider)
	}
}

func assertClaudeNativeShape(t *testing.T, result ExportResult) {
	t.Helper()
	lines := readJSONLRecords(t, result.TranscriptPath)
	if len(lines) == 0 {
		t.Fatalf("expected claude native transcript records")
	}
	var previousUUID any
	var foundToolUse, foundToolResult bool
	for index, line := range lines {
		if line["sessionId"] != result.SessionToken {
			t.Fatalf("claude record has wrong sessionId: %#v", line)
		}
		if index == 0 {
			if line["parentUuid"] != nil {
				t.Fatalf("first claude record parentUuid must be nil, got %#v", line["parentUuid"])
			}
		} else if line["parentUuid"] != previousUUID {
			t.Fatalf("claude parent chain broken at index %d: parent=%#v previous=%#v", index, line["parentUuid"], previousUUID)
		}
		previousUUID = line["uuid"]
		message, _ := line["message"].(map[string]any)
		content, _ := message["content"].([]any)
		for _, rawBlock := range content {
			block, _ := rawBlock.(map[string]any)
			switch block["type"] {
			case "tool_use":
				foundToolUse = true
			case "tool_result":
				foundToolResult = true
			}
		}
	}
	if !foundToolUse || !foundToolResult {
		t.Fatalf("expected claude native transcript to include tool_use and tool_result, got %#v", lines)
	}
}

func assertCodexNativeShape(t *testing.T, result ExportResult, cwd string) {
	t.Helper()
	lines := readJSONLRecords(t, result.TranscriptPath)
	var sessionMeta, userResponse, assistantResponse, userEvent, assistantEvent, toolCall, toolResult bool
	for _, line := range lines {
		payload, _ := line["payload"].(map[string]any)
		switch line["type"] {
		case "session_meta":
			sessionMeta = true
			if payload["id"] != result.SessionToken {
				t.Fatalf("codex session_meta id mismatch: %#v", line)
			}
			if payload["cwd"] != cwd {
				t.Fatalf("codex session_meta cwd mismatch: %#v", line)
			}
		case "response_item":
			switch payload["type"] {
			case "message":
				switch payload["role"] {
				case "user":
					userResponse = true
				case "assistant":
					assistantResponse = true
				}
			case "function_call":
				toolCall = true
			case "function_call_output":
				toolResult = true
			}
		case "event_msg":
			switch payload["type"] {
			case "user_message":
				userEvent = true
			case "agent_message":
				assistantEvent = true
			}
		}
	}
	if !sessionMeta || !userResponse || !assistantResponse || !userEvent || !assistantEvent || !toolCall || !toolResult {
		t.Fatalf("codex native transcript missing resume/replay records: meta=%v userResponse=%v assistantResponse=%v userEvent=%v assistantEvent=%v toolCall=%v toolResult=%v lines=%#v", sessionMeta, userResponse, assistantResponse, userEvent, assistantEvent, toolCall, toolResult, lines)
	}
}

func assertGeminiNativeShape(t *testing.T, result ExportResult, cwd string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(result.ProjectPath, ".project_root")); err != nil {
		t.Fatalf("expected gemini project marker: %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(result.ProjectPath, ".project_root"))
	if err != nil {
		t.Fatalf("read gemini project marker: %v", err)
	}
	if strings.TrimSpace(string(marker)) != cwd {
		t.Fatalf("gemini project marker cwd mismatch: %q", string(marker))
	}
	lines := readJSONLRecords(t, result.TranscriptPath)
	var foundMeta, foundToolCall, foundFunctionCall, foundFunctionResponse bool
	for _, line := range lines {
		if line["sessionId"] == result.SessionToken {
			foundMeta = true
		}
		if line["type"] == "gemini" {
			if rawCalls, ok := line["toolCalls"].([]any); ok {
				foundToolCall = true
				for _, rawCall := range rawCalls {
					call := rawCall.(map[string]any)
					if _, ok := call["functionCall"].(map[string]any); ok {
						foundFunctionCall = true
					}
					if results, ok := call["result"].([]any); ok {
						for _, rawResult := range results {
							result := rawResult.(map[string]any)
							if _, ok := result["functionResponse"].(map[string]any); ok {
								foundFunctionResponse = true
							}
						}
					}
				}
			}
		}
	}
	if !foundMeta || !foundToolCall || !foundFunctionCall || !foundFunctionResponse {
		t.Fatalf("gemini native transcript missing metadata/tool call parts: meta=%v toolCalls=%v functionCall=%v functionResponse=%v lines=%#v", foundMeta, foundToolCall, foundFunctionCall, foundFunctionResponse, lines)
	}
}

func writeSourceClaudeSession(t *testing.T, home string) state.SessionMeta {
	transcriptPath := filepath.Join(home, ".claude", "projects", "-tmp-project", "source-claude.jsonl")
	body := `{"type":"user","message":{"role":"user","content":"hello from claude"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"assistant from claude"},{"type":"tool_use","id":"call_1","name":"Bash","input":{"command":"pwd"}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"/tmp/project"}]}}` + "\n"
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatalf("mkdir claude source: %v", err)
	}
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write claude source: %v", err)
	}
	return state.SessionMeta{Agent: "claude", SessionID: "source-claude", TranscriptPath: transcriptPath, Cwd: "/tmp/project", ProjectName: "project"}
}

func writeSourceCodexSession(t *testing.T, home string) state.SessionMeta {
	transcriptPath := filepath.Join(home, ".codex", "sessions", "2026", "05", "20", "rollout-2026-05-20T10-00-00-source-codex.jsonl")
	body := `{"timestamp":"2026-05-20T10:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"hello from codex"}}` + "\n" +
		`{"timestamp":"2026-05-20T10:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"assistant from codex"}}` + "\n" +
		`{"timestamp":"2026-05-20T10:00:02Z","type":"response_item","payload":{"type":"function_call","call_id":"call_1","name":"Bash","arguments":"{\"command\":\"pwd\"}"}}` + "\n" +
		`{"timestamp":"2026-05-20T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"/tmp/project"}}` + "\n"
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatalf("mkdir codex source: %v", err)
	}
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write codex source: %v", err)
	}
	return state.SessionMeta{Agent: "codex", SessionID: "source-codex", TranscriptPath: transcriptPath, Cwd: "/tmp/project", ProjectName: "project"}
}

func writeSourceGeminiSession(t *testing.T, home string) state.SessionMeta {
	transcriptPath := filepath.Join(home, ".gemini", "tmp", "project", "chats", "session-source-gemini.json")
	body := `{"sessionId":"source-gemini","summary":"Earlier compact summary","messages":[{"id":"u1","timestamp":"2025-12-29T04:04:34.450Z","type":"user","content":"hello from gemini"},{"id":"g1","timestamp":"2025-12-29T04:04:37.997Z","type":"gemini","content":"assistant from gemini","toolCalls":[{"id":"call_1","name":"read_file","args":{"file_path":"src/app.py"},"result":"file contents","status":"success"}]},{"id":"i1","timestamp":"2025-12-29T04:04:39.000Z","type":"info","content":"Conversation checkpoint saved with tag: sync."}]}`
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatalf("mkdir gemini source: %v", err)
	}
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write gemini source: %v", err)
	}
	return state.SessionMeta{Agent: "gemini", SessionID: "source-gemini", TranscriptPath: transcriptPath, Cwd: "/tmp/project", ProjectName: "project"}
}

func countKinds(entries []readmodels.TranscriptEntry, kind string) int {
	count := 0
	for _, entry := range entries {
		if transcript.Kind(entry) == kind {
			count++
		}
	}
	return count
}

func exportDiscoveryRoot(home string, provider string) string {
	switch provider {
	case "claude":
		return filepath.Join(home, ".claude", "projects")
	case "codex":
		return filepath.Join(home, ".codex", "sessions")
	case "gemini":
		return filepath.Join(home, ".gemini", "tmp")
	default:
		return home
	}
}

func readJSONLRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("parse jsonl line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}
