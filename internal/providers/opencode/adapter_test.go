package opencode

import (
	"strings"
	"testing"
)

func TestBuildArgsResumesAndForksNativeOpenCodeSession(t *testing.T) {
	adapter := NewAdapter("opencode")
	args := adapter.BuildArgs(PromptRequest{
		CWD:          "/work/project",
		Model:        "opencode/nemotron-3.5-lightning-free",
		Effort:       "high",
		SessionToken: "ses_source",
		ForkSession:  true,
		Prompt:       "continue",
	})
	got := strings.Join(args, " ")
	for _, expected := range []string{"run", "--format json", "--dir /work/project", "--model opencode/nemotron-3.5-lightning-free", "--variant high", "--session ses_source", "--fork", "continue"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("args %q missing %q", got, expected)
		}
	}
}

func TestParseStreamResultCapturesSessionAndAssistantText(t *testing.T) {
	result, err := ParseStreamResult(strings.NewReader(strings.Join([]string{
		`{"type":"session.created","properties":{"sessionID":"ses_new"}}`,
		`{"type":"message.part.updated","properties":{"part":{"type":"text","text":"hello from OpenCode"}}}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseStreamResult returned error: %v", err)
	}
	if result.SessionToken != "ses_new" {
		t.Fatalf("session token = %q", result.SessionToken)
	}
	if len(result.Entries) != 1 || result.Entries[0]["text"] != "hello from OpenCode" {
		t.Fatalf("entries = %#v", result.Entries)
	}
}
