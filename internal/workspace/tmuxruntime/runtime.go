package tmuxruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"ai-agent-manager/internal/state"
)

const literalSendChunkSize = 16000

var ansiPattern = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)

type Status struct {
	State    string `json:"state"`
	LastLine string `json:"lastLine"`
}

func NormalizeSessionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = "abolqasem"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.'
		if allowed {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	normalized := strings.Trim(builder.String(), "-")
	if normalized == "" {
		return "abolqasem"
	}
	if len(normalized) > 80 {
		normalized = strings.TrimRight(normalized[:80], "-")
	}
	if normalized == "" {
		return "abolqasem"
	}
	return normalized
}

func DefaultCommand() string {
	if command := strings.TrimSpace(os.Getenv("ABOLQASEM_TMUX_CHAT_COMMAND")); command != "" {
		return command
	}
	return "codex"
}

func EnsureSession(ctx context.Context, sessionName string, cwd string, command string) error {
	if err := RequireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if exec.CommandContext(ctx, "tmux", "has-session", "-t", sessionName).Run() == nil {
		return nil
	}
	return exec.CommandContext(ctx, "tmux", buildEnsureSessionArgs(sessionName, cwd, command)...).Run()
}

func KillSession(ctx context.Context, sessionName string) error {
	if err := RequireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if exec.CommandContext(ctx, "tmux", "has-session", "-t", sessionName).Run() != nil {
		return nil
	}
	return exec.CommandContext(ctx, "tmux", "kill-session", "-t", sessionName).Run()
}

func AttachCommand(ctx context.Context, sessionName string) (*exec.Cmd, error) {
	if err := RequireTmux(); err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, "tmux", "attach-session", "-t", NormalizeSessionName(sessionName)), nil
}

func Send(ctx context.Context, sessionName string, text string, enter bool) error {
	if err := RequireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if err := sendText(ctx, sessionName, text); err != nil {
		return err
	}
	if enter {
		if err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, "Enter").Run(); err != nil {
			return err
		}
		return retryPendingPromptSubmit(ctx, sessionName, text)
	}
	return nil
}

func sendText(ctx context.Context, sessionName string, text string) error {
	if text == "" {
		return nil
	}
	if strings.ContainsAny(text, "\r\n") {
		return pasteText(ctx, sessionName, text)
	}
	for _, chunk := range chunkLiteralText(text) {
		if err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, "-l", chunk).Run(); err != nil {
			return err
		}
	}
	return nil
}

func pasteText(ctx context.Context, sessionName string, text string) error {
	bufferName := fmt.Sprintf("abolqasem-paste-%d", time.Now().UnixNano())
	loadCommand := exec.CommandContext(ctx, "tmux", "load-buffer", "-b", bufferName, "-")
	loadCommand.Stdin = strings.NewReader(text)
	if err := loadCommand.Run(); err != nil {
		return err
	}
	return exec.CommandContext(ctx, "tmux", "paste-buffer", "-d", "-p", "-b", bufferName, "-t", sessionName).Run()
}

func Interrupt(ctx context.Context, sessionName string) error {
	if err := RequireTmux(); err != nil {
		return err
	}
	return exec.CommandContext(ctx, "tmux", "send-keys", "-t", NormalizeSessionName(sessionName), "C-c").Run()
}

func Capture(ctx context.Context, sessionName string, lines int) (string, error) {
	if err := RequireTmux(); err != nil {
		return "", err
	}
	if lines < 20 {
		lines = 20
	}
	if lines > 5000 {
		lines = 5000
	}
	output, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-p", "-t", NormalizeSessionName(sessionName), "-S", fmt.Sprintf("-%d", lines)).Output()
	return string(output), err
}

func Resize(ctx context.Context, sessionName string, cols int, rows int) error {
	if err := RequireTmux(); err != nil {
		return err
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return exec.CommandContext(ctx, "tmux", "resize-window", "-t", NormalizeSessionName(sessionName), "-x", fmt.Sprint(cols), "-y", fmt.Sprint(rows)).Run()
}

func ReadStatus(ctx context.Context, sessionName string) (Status, error) {
	output, err := Capture(ctx, sessionName, 120)
	if err != nil {
		return Status{}, err
	}
	lines := meaningfulLines(output)
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}
	return Status{State: statusStateFromLines(lines), LastLine: lastLine}, nil
}

func RequireTmux() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("tmux is not installed or is not available in PATH")
		}
		return err
	}
	return nil
}

func buildEnsureSessionArgs(sessionName string, cwd string, command string) []string {
	args := []string{"new-session", "-d", "-s", sessionName}
	for _, entry := range providerProxyTmuxEnv() {
		args = append(args, "-e", entry)
	}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	if command = strings.TrimSpace(command); command != "" {
		return append(args, command)
	}
	return append(args, DefaultCommand())
}

func providerProxyTmuxEnv() []string {
	keys := map[string]bool{
		"HTTP_PROXY":  true,
		"HTTPS_PROXY": true,
		"ALL_PROXY":   true,
		"NO_PROXY":    true,
		"http_proxy":  true,
		"https_proxy": true,
		"all_proxy":   true,
		"no_proxy":    true,
	}
	env := []string{}
	for _, entry := range state.CurrentProviderProxyEnv() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && keys[key] {
			env = append(env, entry)
		}
	}
	return env
}

func chunkLiteralText(text string) []string {
	if text == "" {
		return nil
	}
	chunks := []string{}
	start := 0
	for index := range text {
		if index > start && index-start >= literalSendChunkSize {
			chunks = append(chunks, text[start:index])
			start = index
		}
	}
	chunks = append(chunks, text[start:])
	return chunks
}

func meaningfulLines(output string) []string {
	lines := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(stripANSI(line))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func statusStateFromLines(lines []string) string {
	if len(lines) == 0 {
		return "idle"
	}
	if looksWorking(lines) {
		return "running"
	}
	if looksWaiting(lines) {
		return "waiting"
	}
	return "running"
}

func tailLines(lines []string, limit int) []string {
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func looksWorking(lines []string) bool {
	for _, line := range tailLines(lines, 12) {
		normalized := strings.ToLower(line)
		if strings.HasPrefix(normalized, "working") && strings.Contains(normalized, "esc to interrupt") {
			return true
		}
	}
	return false
}

func looksWaiting(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range tailLines(lines, 12) {
		if isPromptLine(line) || isAgentStatusLine(line) {
			return true
		}
	}
	return false
}

func isPromptLine(line string) bool {
	return line == "›" || line == ">" || line == "❯" || line == "$" || line == "#" ||
		strings.HasPrefix(line, "› ") || strings.HasPrefix(line, "> ") || strings.HasPrefix(line, "❯ ") ||
		strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "# ") ||
		strings.HasSuffix(line, " ›") || strings.HasSuffix(line, " ❯") ||
		strings.HasSuffix(line, " $") || strings.HasSuffix(line, " #")
}

func isAgentStatusLine(line string) bool {
	normalized := strings.ToLower(line)
	if strings.HasPrefix(normalized, "gpt-") && strings.Contains(normalized, "context ") {
		return true
	}
	if (strings.HasPrefix(normalized, "claude") || strings.HasPrefix(normalized, "gemini")) &&
		(strings.Contains(normalized, "context") ||
			strings.Contains(normalized, "token") ||
			strings.Contains(normalized, "model") ||
			strings.Contains(normalized, "cwd") ||
			strings.Contains(normalized, "directory")) {
		return true
	}
	return false
}

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func retryPendingPromptSubmit(ctx context.Context, sessionName string, text string) error {
	lastSubmittedLine := lastSubmittedPromptLine(text)
	if lastSubmittedLine == "" {
		return nil
	}

	for attempt := 0; attempt < 2; attempt += 1 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(140 * time.Millisecond):
		}

		output, err := Capture(ctx, sessionName, 120)
		if err != nil {
			return nil
		}
		lines := meaningfulLines(output)
		if statusStateFromLines(lines) != "waiting" || !tailPromptStillContainsText(lines, lastSubmittedLine) {
			return nil
		}
		if err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, "Enter").Run(); err != nil {
			return err
		}
	}

	return nil
}

func lastSubmittedPromptLine(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index -= 1 {
		line := strings.TrimSpace(lines[index])
		if line != "" {
			return line
		}
	}
	return ""
}

func tailPromptStillContainsText(lines []string, submitted string) bool {
	submitted = strings.TrimSpace(submitted)
	if submitted == "" {
		return false
	}
	tail := tailLines(lines, 16)
	hasPromptTail := false
	for _, line := range tail {
		if isPromptLine(line) {
			hasPromptTail = true
		}
		if promptStillContainsText(line, submitted) {
			return true
		}
	}
	if !hasPromptTail {
		return false
	}
	normalizedSubmitted := strings.ToLower(strings.Join(strings.Fields(submitted), " "))
	for _, line := range tail {
		normalizedLine := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(stripANSI(line))), " "))
		if normalizedLine != "" && strings.Contains(normalizedLine, normalizedSubmitted) {
			return true
		}
	}
	return false
}

func promptStillContainsText(line string, submitted string) bool {
	line = strings.TrimSpace(stripANSI(line))
	submitted = strings.TrimSpace(submitted)
	if line == "" || submitted == "" {
		return false
	}
	if !(strings.HasPrefix(line, "›") || strings.HasPrefix(line, ">") || strings.HasPrefix(line, "❯") || strings.HasPrefix(line, "$") || strings.HasPrefix(line, "#")) {
		return false
	}
	normalizedLine := strings.ToLower(strings.Join(strings.Fields(line), " "))
	normalizedSubmitted := strings.ToLower(strings.Join(strings.Fields(submitted), " "))
	return strings.Contains(normalizedLine, normalizedSubmitted)
}
