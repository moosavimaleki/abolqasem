package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"abolqasem/internal/workspace/agent"
)

func TestStartWorkspaceClaudeTurnUsesPendingForkToken(t *testing.T) {
	binDir := t.TempDir()
	argsPath := filepath.Join(binDir, "claude.args")
	writeFakeExecutable(t, filepath.Join(binDir, "claude"), fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
printf '%%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"forked"}]}}'
printf '%%s\n' '{"type":"result","result":"done","duration_ms":1,"session_id":"claude-forked-session"}'
`, argsPath), fmt.Sprintf(`@echo off
type nul > "%s"
:args
if "%%~1"=="" goto done
>> "%s" echo %%~1
shift /1
goto args
:done
echo {"type":"assistant","message":{"content":[{"type":"text","text":"forked"}]}}
echo {"type":"result","result":"done","duration_ms":1,"session_id":"claude-forked-session"}
`, argsPath, argsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	turn := startWorkspaceClaudeTurn(context.Background(), agent.TurnRequest{
		Content:                 "continue",
		PendingForkSessionToken: "claude-source-session",
	})
	events := drainWorkspaceTurnEvents(turn)
	if !hasTurnEvent(events, agent.TurnEventSessionToken, "claude-forked-session") {
		t.Fatalf("expected session token event, got %#v", events)
	}
	if !hasTurnEvent(events, agent.TurnEventFinished, "") {
		t.Fatalf("expected finished event, got %#v", events)
	}
	args := readArgsFile(t, argsPath)
	if !containsArgSequence(args, "--resume", "claude-source-session") || !containsArg(args, "--fork-session") {
		t.Fatalf("expected claude resume/fork args, got %#v", args)
	}
}

func writeFakeExecutable(t *testing.T, path string, unixBody string, windowsBody string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path += ".cmd"
		if err := os.WriteFile(path, []byte(windowsBody), 0o755); err != nil {
			t.Fatalf("write fake executable: %v", err)
		}
		return
	}
	if err := os.WriteFile(path, []byte(unixBody), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
}

func drainWorkspaceTurnEvents(turn agent.Turn) []agent.TurnEvent {
	source := turn.(agent.TurnEventSource)
	var events []agent.TurnEvent
	for event := range source.Events() {
		events = append(events, event)
	}
	return events
}

func readArgsFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	args := make([]string, 0, len(lines))
	for _, line := range lines {
		arg := strings.TrimSpace(line)
		if arg != "" {
			args = append(args, arg)
		}
	}
	return args
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func containsArgSequence(args []string, first string, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}

func hasTurnEvent(events []agent.TurnEvent, kind agent.TurnEventKind, sessionToken string) bool {
	for _, event := range events {
		if event.Type != kind {
			continue
		}
		if sessionToken == "" || event.SessionToken == sessionToken {
			return true
		}
	}
	return false
}
