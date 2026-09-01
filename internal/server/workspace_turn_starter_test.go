package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	codexprovider "abolqasem/internal/providers/codex"
	codexrpc "abolqasem/internal/providers/codex/rpc"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/readmodels"
)

type workspaceCodexTestTransport struct {
	sent chan []byte
}

func TestCredentialSwitchResetsOnlyIdleCodexSessions(t *testing.T) {
	manager := newWorkspaceCodexSessionManager()
	idle := &workspaceCodexSession{chatID: "idle"}
	running := &workspaceCodexSession{chatID: "running"}
	running.turnMu.Lock()
	defer running.turnMu.Unlock()
	manager.sessions[idle.chatID] = idle
	manager.sessions[running.chatID] = running

	if reset := manager.resetForCredentialSwitch(); reset != 1 {
		t.Fatalf("idle sessions reset = %d, want 1", reset)
	}
	if _, found := manager.sessions[idle.chatID]; found {
		t.Fatal("idle session must be removed")
	}
	if manager.sessions[running.chatID] != running {
		t.Fatal("running session must remain available to finish with its old credential")
	}
}

func TestResetIdleThreadDoesNotInterruptRunningTurn(t *testing.T) {
	manager := newWorkspaceCodexSessionManager()
	session := &workspaceCodexSession{chatID: "chat", threadID: "thread"}
	manager.sessions[session.chatID] = session
	session.turnMu.Lock()
	if manager.resetIdleThread("chat", "thread") {
		t.Fatal("active turn must not be reset")
	}
	session.turnMu.Unlock()
	if !manager.resetIdleThread("chat", "thread") {
		t.Fatal("idle session must be reset")
	}
	if len(manager.sessions) != 0 {
		t.Fatalf("session remains after reset: %#v", manager.sessions)
	}
}

func (transport *workspaceCodexTestTransport) Send(message []byte) error {
	transport.sent <- append([]byte(nil), message...)
	return nil
}

func TestWorkspaceCodexStdoutAcceptsResumeResponsesLargerThanScannerLimit(t *testing.T) {
	transport := &workspaceCodexTestTransport{sent: make(chan []byte, 1)}
	client := codexrpc.NewClient(transport)
	process := &workspaceCodexProcess{client: client}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	done := make(chan error, 1)
	go func() {
		done <- client.Call(ctx, "thread/resume", map[string]any{"threadId": "session-large"}, &result)
	}()

	request := <-transport.sent
	var envelope struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		t.Fatal(err)
	}
	response := `{"id":"` + envelope.ID + `","result":{"thread":{"id":"session-large"},"history":"` + strings.Repeat("x", 9*1024*1024) + `"}}` + "\n"
	process.scanStdout(strings.NewReader(response))

	if err := <-done; err != nil {
		t.Fatalf("large resume response was not delivered: %v", err)
	}
	if result.Thread.ID != "session-large" {
		t.Fatalf("thread id = %q", result.Thread.ID)
	}
}

func TestWorkspaceCodexInputsReferenceAttachedTextWithoutInliningIt(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "pasted-text.txt")
	if err := os.WriteFile(textPath, []byte("long pasted body"), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := workspaceCodexInputs("inspect these", []readmodels.ChatAttachment{
		{Kind: "image", AbsolutePath: filepath.Join(dir, "shot.png"), DisplayName: "shot.png", MimeType: "image/png"},
		{Kind: "file", AbsolutePath: textPath, DisplayName: "pasted-text.txt", MimeType: "text/plain"},
	})
	if len(inputs) != 2 {
		t.Fatalf("expected reference text and native image inputs, got %#v", inputs)
	}
	if inputs[1].Type != "localImage" || inputs[1].Path == "" {
		t.Fatalf("expected native localImage input, got %#v", inputs[1])
	}
	if inputs[0].Type != "text" || !strings.Contains(inputs[0].Text, "# Files mentioned by the user:") || !strings.Contains(inputs[0].Text, "## pasted-text.txt: "+textPath) || strings.Contains(inputs[0].Text, "long pasted body") {
		t.Fatalf("expected Codex Mobile style file reference without pasted body, got %#v", inputs[0])
	}
}

func TestWorkspaceCodexTurnStartIncludesSelectedReasoningEffort(t *testing.T) {
	transport := &workspaceCodexTestTransport{sent: make(chan []byte, 1)}
	process := &workspaceCodexProcess{client: codexrpc.NewClient(transport)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resultCh := make(chan struct {
		id  string
		err error
	}, 1)
	go func() {
		id, err := process.StartTurn(ctx, "thread-1", agent.TurnRequest{
			Content: "use the selected effort",
			Model:   "gpt-5.6",
			Effort:  "xhigh",
		})
		resultCh <- struct {
			id  string
			err error
		}{id: id, err: err}
	}()

	request := <-transport.sent
	var envelope struct {
		ID     string         `json:"id"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Params["effort"] != "xhigh" {
		t.Fatalf("top-level effort = %#v", envelope.Params["effort"])
	}
	mode, ok := envelope.Params["collaborationMode"].(map[string]any)
	if !ok {
		t.Fatalf("collaboration mode missing from params: %#v", envelope.Params)
	}
	settings, ok := mode["settings"].(map[string]any)
	if !ok || settings["reasoning_effort"] != "xhigh" {
		t.Fatalf("collaboration reasoning effort = %#v", settings["reasoning_effort"])
	}
	response := []byte(`{"id":"` + envelope.ID + `","result":{"turn":{"id":"turn-1"}}}` + "\n")
	process.scanStdout(strings.NewReader(string(response)))
	result := <-resultCh
	if result.err != nil || result.id != "turn-1" {
		t.Fatalf("StartTurn result = %q, %v", result.id, result.err)
	}
}

func TestWorkspaceCodexOpenThreadIncludesSelectedModelProvider(t *testing.T) {
	transport := &workspaceCodexTestTransport{sent: make(chan []byte, 1)}
	process := &workspaceCodexProcess{client: codexrpc.NewClient(transport)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		_, err := process.OpenThread(ctx, agent.TurnRequest{
			Model:              "gpt-5.6",
			CodexModelProvider: "codex_manager",
		})
		resultCh <- err
	}()

	request := <-transport.sent
	var envelope struct {
		ID     string         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(request, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Method != "thread/start" || envelope.Params["modelProvider"] != "codex_manager" {
		t.Fatalf("model provider was not sent in thread/start: %#v", envelope)
	}
	process.scanStdout(strings.NewReader(`{"id":"` + envelope.ID + `","result":{"thread":{"id":"thread-1"}}}` + "\n"))
	if err := <-resultCh; err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceCodexExecutionPolicy(t *testing.T) {
	standard := workspaceCodexExecutionPolicyFor("standard")
	if standard.approvalPolicy != "on-request" || standard.sandbox != "read-only" {
		t.Fatalf("unexpected standard execution policy: %#v", standard)
	}

	dangerous := workspaceCodexExecutionPolicyFor("dangerous")
	if dangerous.approvalPolicy != "never" || dangerous.sandbox != "danger-full-access" {
		t.Fatalf("unexpected dangerous execution policy: %#v", dangerous)
	}

	if fallback := workspaceCodexExecutionPolicyFor("unknown"); fallback != dangerous {
		t.Fatalf("unknown execution mode must preserve the existing unrestricted default: %#v", fallback)
	}
}

func TestWorkspaceCodexSessionReuseRequiresMatchingExecutionMode(t *testing.T) {
	session := &workspaceCodexSession{cwd: "/workspace", threadID: "thread", executionMode: "dangerous", process: &workspaceCodexProcess{done: make(chan struct{})}}
	if session.reusableFor(agent.TurnRequest{LocalPath: "/workspace", ExecutionMode: "standard"}) {
		t.Fatal("expected execution-mode change to replace the app-server session")
	}
}

func TestWorkspaceCodexApprovalWaitsForExplicitToolResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	turn := &workspaceAsyncTurn{
		events:        make(chan agent.TurnEvent, 1),
		toolResponses: map[string]chan workspaceToolResponse{},
	}
	resultCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := turn.waitForToolResponse(ctx, codexprovider.ToolRequest{Tool: map[string]any{
			"toolId":   "approval-1",
			"toolKind": "approval_request",
			"toolName": "ApprovalRequest",
			"input": map[string]any{
				"approvalKind": "file_change",
				"itemId":       "patch-1",
			},
		}})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	pending := <-turn.events
	if pending.Type != agent.TurnEventPendingTool || pending.PendingTool == nil || pending.PendingTool.ToolUseID != "approval-1" {
		t.Fatalf("expected a pending approval event, got %#v", pending)
	}
	select {
	case result := <-resultCh:
		t.Fatalf("approval returned before the user responded: %#v", result)
	default:
	}

	if err := turn.RespondTool(context.Background(), agent.ToolResponse{
		ToolUseID: "approval-1",
		Result:    map[string]any{"decision": "accept"},
	}); err != nil {
		t.Fatalf("RespondTool returned error: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatal(err)
	case result := <-resultCh:
		encoded, _ := json.Marshal(result)
		if string(encoded) != `{"decision":"accept"}` {
			t.Fatalf("unexpected approval result: %s", encoded)
		}
	}
}

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
