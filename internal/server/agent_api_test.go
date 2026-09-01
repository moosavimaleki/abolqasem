package server

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"abolqasem/internal/providers/providerexec"
)

func TestRunLegacyAgentTurnSupportsClaudeAndOpenCode(t *testing.T) {
	binDir := t.TempDir()
	claudePath := filepath.Join(binDir, "claude")
	openCodePath := filepath.Join(binDir, "opencode")
	writeFakeExecutable(t, claudePath, "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"result\",\"result\":\"done\",\"session_id\":\"claude-session-test\"}'\n", "@echo off\necho {\"type\":\"result\",\"result\":\"done\",\"session_id\":\"claude-session-test\"}\n")
	writeFakeExecutable(t, openCodePath, "#!/bin/sh\nprintf '%s\\n' '{\"sessionID\":\"ses_opencode_test\",\"type\":\"text\",\"text\":\"done\"}'\n", "@echo off\necho {\"sessionID\":\"ses_opencode_test\",\"type\":\"text\",\"text\":\"done\"}\n")
	if runtime.GOOS == "windows" {
		claudePath += ".cmd"
		openCodePath += ".cmd"
	}
	providerexec.SetConfiguredExecutables(map[string]string{"claude": claudePath, "opencode": openCodePath})
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })

	for _, fixture := range []struct {
		agent string
		want  string
	}{
		{agent: "claude", want: "claude-session-test"},
		{agent: "opencode", want: "ses_opencode_test"},
	} {
		t.Run(fixture.agent, func(t *testing.T) {
			result, err := runLegacyAgentTurn(context.Background(), agentTurnRequest{Agent: fixture.agent, Message: "hello", Model: "test-model"}, "", t.TempDir())
			if err != nil {
				t.Fatalf("run legacy agent: %v", err)
			}
			if result.SessionID != fixture.want || result.Model != "test-model" {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
}

func TestCLIRuntimeStatusIsControllableWhenConfigured(t *testing.T) {
	binDir := t.TempDir()
	path := filepath.Join(binDir, "opencode")
	writeFakeExecutable(t, path, "#!/bin/sh\nexit 0\n", "@echo off\nexit /b 0\n")
	if runtime.GOOS == "windows" {
		path += ".cmd"
	}
	providerexec.SetConfiguredExecutables(map[string]string{"opencode": path})
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })

	status := buildCLIRuntimeStatus("opencode", "OpenCode")
	if !status.Available || !status.Controllable || !status.Capabilities["can_send"] {
		t.Fatalf("expected controllable OpenCode runtime, got %#v", status)
	}
}
