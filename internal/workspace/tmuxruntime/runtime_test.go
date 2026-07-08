package tmuxruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSessionName(t *testing.T) {
	if got := NormalizeSessionName(" Chat:9550 / Demo "); got != "chat-9550-demo" {
		t.Fatalf("unexpected normalized tmux session %q", got)
	}
	if got := NormalizeSessionName(""); got != "abolqasem" {
		t.Fatalf("unexpected empty tmux session %q", got)
	}
}

func TestBuildEnsureSessionArgs(t *testing.T) {
	args := buildEnsureSessionArgs("abolqasem-chat-1", "/tmp/project", "codex --yolo")
	expected := []string{"new-session", "-d", "-s", "abolqasem-chat-1", "-c", "/tmp/project", "codex --yolo"}
	if strings.Join(args, "\x00") != strings.Join(expected, "\x00") {
		t.Fatalf("unexpected tmux args: %#v", args)
	}
}

func TestChunkLiteralTextKeepsUtf8Runes(t *testing.T) {
	text := strings.Repeat("الف", 6000)
	chunks := chunkLiteralText(text)
	if len(chunks) < 2 {
		t.Fatalf("expected chunked text, got %d chunks", len(chunks))
	}
	if strings.Join(chunks, "") != text {
		t.Fatal("chunked text did not round-trip")
	}
	for _, chunk := range chunks {
		if !strings.Contains(chunk, "ا") {
			t.Fatalf("unexpected chunk %q", chunk)
		}
	}
}

func TestSendSubmitsWithNamedEnterKeyAfterLiteralText(t *testing.T) {
	restoreTmuxRuntimeCommands(t)

	requireTmux = func() error { return nil }
	commands := [][]string{}
	runTmuxCommand = func(_ context.Context, args ...string) error {
		commands = append(commands, append([]string(nil), args...))
		return nil
	}

	if err := Send(context.Background(), "Chat:1", "hello", true); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	expected := [][]string{
		{"send-keys", "-t", "chat-1", "-l", "hello"},
		{"send-keys", "-t", "chat-1", "Enter"},
	}
	if strings.Join(flattenCommands(commands), "\x00") != strings.Join(flattenCommands(expected), "\x00") {
		t.Fatalf("unexpected tmux command sequence: %#v", commands)
	}
	for _, command := range commands {
		for _, arg := range command {
			if arg == "paste-buffer" || arg == "load-buffer" || arg == "\r" || arg == "\n" {
				t.Fatalf("submit must use a named Enter key, got command %#v", command)
			}
		}
	}
}

func TestTmuxSubmitDelayProtectsTuiComposerBeforeEnter(t *testing.T) {
	if got := tmuxSubmitDelay(""); got != 0 {
		t.Fatalf("empty submit delay = %s, want 0", got)
	}
	if got := tmuxSubmitDelay("hello"); got < 350*time.Millisecond {
		t.Fatalf("text submit delay = %s, want at least 350ms", got)
	}
}

func TestEnsureSessionSkipsWhenTmuxMissing(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	if err := RequireTmux(); err != nil {
		t.Fatalf("RequireTmux returned error: %v", err)
	}
}

func TestEnsureSessionCreatesDetachedTmuxSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	session := fmt.Sprintf("abolqasem-runtime-test-%d", time.Now().UnixNano())
	if err := EnsureSession(context.Background(), session, t.TempDir(), "sh"); err != nil {
		t.Fatalf("EnsureSession returned error: %v", err)
	}
	defer exec.Command("tmux", "kill-session", "-t", session).Run()
	if err := exec.Command("tmux", "has-session", "-t", session).Run(); err != nil {
		t.Fatalf("expected tmux session to exist: %v", err)
	}
}

func TestRestartSessionKillsThenCreatesDetachedSession(t *testing.T) {
	restoreTmuxRuntimeCommands(t)

	requireTmux = func() error { return nil }
	sessionExists := true
	commands := [][]string{}
	runTmuxCommand = func(_ context.Context, args ...string) error {
		command := append([]string(nil), args...)
		commands = append(commands, command)
		if len(args) >= 1 && args[0] == "has-session" {
			if sessionExists {
				return nil
			}
			return errors.New("missing")
		}
		if len(args) >= 1 && args[0] == "kill-session" {
			sessionExists = false
			return nil
		}
		return nil
	}

	if err := RestartSession(context.Background(), "Chat:1", "/tmp/project", "codex --sandbox workspace-write"); err != nil {
		t.Fatalf("RestartSession returned error: %v", err)
	}

	expected := [][]string{
		{"has-session", "-t", "chat-1"},
		{"kill-session", "-t", "chat-1"},
		{"has-session", "-t", "chat-1"},
		{"new-session", "-d", "-s", "chat-1", "-c", "/tmp/project", "codex --sandbox workspace-write"},
	}
	if strings.Join(flattenCommands(commands), "\x00") != strings.Join(flattenCommands(expected), "\x00") {
		t.Fatalf("unexpected tmux command sequence: %#v", commands)
	}
}

func TestSendMultilinePastesIntoTmuxPane(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	session := fmt.Sprintf("abolqasem-runtime-paste-test-%d", time.Now().UnixNano())
	outputPath := filepath.Join(t.TempDir(), "input.txt")
	if err := EnsureSession(context.Background(), session, t.TempDir(), "cat > "+shellQuote(outputPath)); err != nil {
		t.Fatalf("EnsureSession returned error: %v", err)
	}
	defer exec.Command("tmux", "kill-session", "-t", session).Run()

	if err := Send(context.Background(), session, "first line\nsecond line", true); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if err := exec.Command("tmux", "send-keys", "-t", session, "C-d").Run(); err != nil {
		t.Fatalf("send C-d: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(outputPath)
		if err == nil && string(data) == "first line\nsecond line\n" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(outputPath)
	t.Fatalf("unexpected pasted content %q", string(data))
}

func TestSendSingleLineSubmitsIntoTmuxPane(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	session := fmt.Sprintf("abolqasem-runtime-send-test-%d", time.Now().UnixNano())
	outputPath := filepath.Join(t.TempDir(), "input.txt")
	if err := EnsureSession(context.Background(), session, t.TempDir(), "cat > "+shellQuote(outputPath)); err != nil {
		t.Fatalf("EnsureSession returned error: %v", err)
	}
	defer exec.Command("tmux", "kill-session", "-t", session).Run()

	if err := Send(context.Background(), session, "hello", true); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if err := exec.Command("tmux", "send-keys", "-t", session, "C-d").Run(); err != nil {
		t.Fatalf("send C-d: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(outputPath)
		if err == nil && string(data) == "hello\n" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(outputPath)
	t.Fatalf("unexpected submitted content %q", string(data))
}

func TestStatusClassifiesPromptTail(t *testing.T) {
	status, err := statusFromOutput("hello\n›\n")
	if err != nil {
		t.Fatalf("statusFromOutput returned error: %v", err)
	}
	if status.State != "waiting" {
		t.Fatalf("expected waiting status, got %#v", status)
	}
}

func TestStatusClassifiesPromptWithTextTail(t *testing.T) {
	status, err := statusFromOutput("• done\n› سلام\n")
	if err != nil {
		t.Fatalf("statusFromOutput returned error: %v", err)
	}
	if status.State != "waiting" {
		t.Fatalf("expected waiting status, got %#v", status)
	}
}

func TestStatusClassifiesCodexFooterTail(t *testing.T) {
	status, err := statusFromOutput("• سلام.\ngpt-5.4 medium - ~/project - Context 90% used - 5h left\n")
	if err != nil {
		t.Fatalf("statusFromOutput returned error: %v", err)
	}
	if status.State != "waiting" {
		t.Fatalf("expected waiting status, got %#v", status)
	}
}

func TestStatusClassifiesClaudeAndGeminiFooterTail(t *testing.T) {
	cases := []string{
		"Claude Sonnet - /tmp/project - tokens 120k\n",
		"Gemini 3 Pro - C:\\Users\\me\\project - model ready\n",
	}

	for _, output := range cases {
		status, err := statusFromOutput(output)
		if err != nil {
			t.Fatalf("statusFromOutput returned error: %v", err)
		}
		if status.State != "waiting" {
			t.Fatalf("expected waiting status for %q, got %#v", output, status)
		}
	}
}

func TestStatusClassifiesCrossAgentPromptTail(t *testing.T) {
	status, err := statusFromOutput("done\n❯ summarize\n")
	if err != nil {
		t.Fatalf("statusFromOutput returned error: %v", err)
	}
	if status.State != "waiting" {
		t.Fatalf("expected waiting status, got %#v", status)
	}
}

func TestStatusClassifiesWorkingTail(t *testing.T) {
	status, err := statusFromOutput("Working (15s • esc to interrupt)\ngpt-5.4 medium - ~/project - Context 90% used\n")
	if err != nil {
		t.Fatalf("statusFromOutput returned error: %v", err)
	}
	if status.State != "running" {
		t.Fatalf("expected running status, got %#v", status)
	}
}

func statusFromOutput(output string) (Status, error) {
	lines := meaningfulLines(output)
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}
	return Status{State: statusStateFromLines(lines), LastLine: lastLine}, nil
}

func TestAttachCommandBuildsTmuxAttach(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	cmd, err := AttachCommand(context.Background(), "Chat:1")
	if err != nil {
		t.Fatalf("AttachCommand returned error: %v", err)
	}
	if strings.Join(cmd.Args, " ") != "tmux attach-session -t chat-1" {
		t.Fatalf("unexpected attach command args: %#v", cmd.Args)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func restoreTmuxRuntimeCommands(t *testing.T) {
	t.Helper()
	originalRequireTmux := requireTmux
	originalRunTmuxCommand := runTmuxCommand
	t.Cleanup(func() {
		requireTmux = originalRequireTmux
		runTmuxCommand = originalRunTmuxCommand
	})
}

func flattenCommands(commands [][]string) []string {
	flat := []string{}
	for _, command := range commands {
		flat = append(flat, command...)
		flat = append(flat, "|")
	}
	return flat
}
