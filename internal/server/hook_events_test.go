package server

import (
	"abolqasem/internal/state"
	"testing"
)

func TestIsResponseCompleteHookEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      bool
	}{
		{name: "codex stop", eventName: "Stop", want: true},
		{name: "claude stop normalized", eventName: "stop", want: true},
		{name: "gemini after agent", eventName: "AfterAgent", want: true},
		{name: "generic completion", eventName: "turn.completed", want: true},
		{name: "prompt submission", eventName: "UserPromptSubmit", want: false},
		{name: "gemini session end", eventName: "SessionEnd", want: false},
		{name: "unknown event", eventName: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := state.HookEvent{HookEventName: test.eventName}
			if got := isResponseCompleteHookEvent(event); got != test.want {
				t.Fatalf("isResponseCompleteHookEvent(%q) = %v, want %v", test.eventName, got, test.want)
			}
		})
	}
}
