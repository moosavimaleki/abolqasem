package claude

import (
	"strings"
	"testing"
)

func TestBuildArgsMatchesClaudeStreamMode(t *testing.T) {
	args := NewAdapter("claude").BuildArgs(PromptRequest{
		Model:        "claude-sonnet-4-6",
		Effort:       "high",
		PlanMode:     true,
		SessionToken: "session-1",
		ForkSession:  true,
		Prompt:       "hello",
	})

	expected := []string{
		"--print",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--model", "claude-sonnet-4-6",
		"--effort", "high",
		"--permission-mode", "plan",
		"--resume", "session-1",
		"--fork-session",
		"hello",
	}
	if !equalStringSlices(args, expected) {
		t.Fatalf("expected args %#v, got %#v", expected, args)
	}
}

func TestParseStreamMapsAssistantAndResult(t *testing.T) {
	stream := strings.NewReader(strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"result","result":"done","duration_ms":12,"session_id":"new-session"}`,
	}, "\n"))

	result, err := ParseStreamResult(stream)
	if err != nil {
		t.Fatalf("ParseStream returned error: %v", err)
	}
	entries := result.Entries
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %#v", entries)
	}
	if result.SessionToken != "new-session" {
		t.Fatalf("expected session token from result event, got %q", result.SessionToken)
	}
	if entries[0]["kind"] != "assistant_text" || entries[0]["text"] != "hello" {
		t.Fatalf("unexpected assistant entry: %#v", entries[0])
	}
	if entries[1]["kind"] != "result" || entries[1]["result"] != "done" {
		t.Fatalf("unexpected result entry: %#v", entries[1])
	}
}

func equalStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
