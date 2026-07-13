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

	"abolqasem/internal/state"
)

const literalSendChunkSize = 16000
const textSubmitDelay = 350 * time.Millisecond

var ansiPattern = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)
var runTmuxCommand = func(ctx context.Context, args ...string) error {
	return exec.CommandContext(ctx, "tmux", args...).Run()
}
var runTmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "tmux", args...).Output()
}
var requireTmux = RequireTmux

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
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if hasSession(ctx, sessionName) {
		return nil
	}
	return runTmuxCommand(ctx, buildEnsureSessionArgs(sessionName, cwd, command)...)
}

func SessionExists(ctx context.Context, sessionName string) bool {
	if requireTmux() != nil {
		return false
	}
	return hasSession(ctx, NormalizeSessionName(sessionName))
}

func KillSession(ctx context.Context, sessionName string) error {
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if !hasSession(ctx, sessionName) {
		return nil
	}
	return runTmuxCommand(ctx, "kill-session", "-t", sessionName)
}

func RestartSession(ctx context.Context, sessionName string, cwd string, command string) error {
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if hasSession(ctx, sessionName) {
		if err := runTmuxCommand(ctx, "kill-session", "-t", sessionName); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for hasSession(ctx, sessionName) {
		if time.Now().After(deadline) {
			return errors.New("tmux session did not stop")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return runTmuxCommand(ctx, buildEnsureSessionArgs(sessionName, cwd, command)...)
}

func AttachCommand(ctx context.Context, sessionName string) (*exec.Cmd, error) {
	if err := RequireTmux(); err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, "tmux", "attach-session", "-t", NormalizeSessionName(sessionName)), nil
}

func hasSession(ctx context.Context, sessionName string) bool {
	return runTmuxCommand(ctx, "has-session", "-t", sessionName) == nil
}

func Send(ctx context.Context, sessionName string, text string, enter bool) error {
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if err := sendText(ctx, sessionName, text); err != nil {
		return err
	}
	if enter {
		if delay := tmuxSubmitDelay(text); delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		return runTmuxCommand(ctx, "send-keys", "-t", sessionName, "Enter")
	}
	return nil
}

func sendText(ctx context.Context, sessionName string, text string) error {
	if text == "" {
		return nil
	}
	for _, chunk := range chunkLiteralText(text) {
		if err := runTmuxCommand(ctx, "send-keys", "-t", sessionName, "-l", chunk); err != nil {
			return err
		}
	}
	return nil
}

func tmuxSubmitDelay(text string) time.Duration {
	if text == "" {
		return 0
	}
	return textSubmitDelay
}

func Interrupt(ctx context.Context, sessionName string) error {
	if err := RequireTmux(); err != nil {
		return err
	}
	return exec.CommandContext(ctx, "tmux", "send-keys", "-t", NormalizeSessionName(sessionName), "C-c").Run()
}

func Capture(ctx context.Context, sessionName string, lines int) (string, error) {
	if err := requireTmux(); err != nil {
		return "", err
	}
	if lines < 20 {
		lines = 20
	}
	if lines > 5000 {
		lines = 5000
	}
	output, err := runTmuxOutput(ctx, "capture-pane", "-p", "-t", NormalizeSessionName(sessionName), "-S", fmt.Sprintf("-%d", lines))
	return string(output), err
}

func ApplyCodexRuntimePreferences(ctx context.Context, sessionName string, model string, effort string) error {
	model = strings.TrimSpace(model)
	effort = strings.TrimSpace(effort)
	if model == "" {
		return errors.New("model is required")
	}
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	status, err := ReadStatus(ctx, sessionName)
	if err == nil && status.State == "running" {
		return errors.New("agent is running; wait until it is ready before changing model")
	}
	if err := Send(ctx, sessionName, "/model", true); err != nil {
		return err
	}
	modelSelectedWithEffort, err := selectRuntimeMenuTarget(ctx, sessionName, "codex model menu", func(option string) bool {
		return codexOptionMatchesModel(option, model) && (effort == "" || codexOptionMatchesEffort(option, effort))
	})
	if err != nil {
		modelSelectedWithEffort = false
		if _, fallbackErr := selectRuntimeMenuTarget(ctx, sessionName, "codex model menu", func(option string) bool {
			return codexOptionMatchesModel(option, model)
		}); fallbackErr != nil {
			_ = runTmuxCommand(ctx, "send-keys", "-t", sessionName, "Escape")
			return err
		}
	}
	if effort == "" || modelSelectedWithEffort {
		return nil
	}
	if _, err := selectRuntimeMenuTarget(ctx, sessionName, "codex effort menu", func(option string) bool {
		return codexOptionMatchesEffort(option, effort)
	}); err != nil {
		_ = runTmuxCommand(ctx, "send-keys", "-t", sessionName, "Escape")
		return err
	}
	return nil
}

func ApplyClaudeRuntimePreferences(ctx context.Context, sessionName string, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if err := ensureAgentReady(ctx, sessionName); err != nil {
		return err
	}
	return sendSlashCommand(ctx, sessionName, "/model "+model)
}

func ApplyGeminiRuntimePreferences(ctx context.Context, sessionName string, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	if err := requireTmux(); err != nil {
		return err
	}
	sessionName = NormalizeSessionName(sessionName)
	if err := ensureAgentReady(ctx, sessionName); err != nil {
		return err
	}
	if err := sendSlashCommand(ctx, sessionName, "/model"); err != nil {
		return err
	}
	if _, err := selectRuntimeMenuTarget(ctx, sessionName, "gemini model menu", func(option string) bool {
		return runtimeOptionMatchesModel(option, model)
	}); err != nil {
		_ = runTmuxCommand(ctx, "send-keys", "-t", sessionName, "Escape")
		return err
	}
	return nil
}

func ensureAgentReady(ctx context.Context, sessionName string) error {
	status, err := ReadStatus(ctx, sessionName)
	if err == nil && status.State == "running" {
		return errors.New("agent is running; wait until it is ready before changing model")
	}
	return nil
}

func sendSlashCommand(ctx context.Context, sessionName string, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("command is required")
	}
	if err := sendText(ctx, sessionName, command); err != nil {
		return err
	}
	return runTmuxCommand(ctx, "send-keys", "-t", sessionName, "Enter")
}

func selectRuntimeMenuTarget(ctx context.Context, sessionName string, menuName string, matches func(string) bool) (bool, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		output, err := Capture(ctx, sessionName, 80)
		if err != nil {
			return false, err
		}
		keys, selectedText, err := runtimeMenuSelectionKeys(output, menuName, matches)
		if err == nil {
			for _, key := range keys {
				if err := runTmuxCommand(ctx, "send-keys", "-t", sessionName, key); err != nil {
					return false, err
				}
			}
			return strings.TrimSpace(selectedText) != "", nil
		}
		if time.Now().After(deadline) {
			return false, err
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

type runtimeMenuOption struct {
	text        string
	highlighted bool
}

func codexMenuSelectionKeys(output string, matches func(string) bool) ([]string, string, error) {
	return runtimeMenuSelectionKeys(output, "codex menu", matches)
}

func runtimeMenuSelectionKeys(output string, menuName string, matches func(string) bool) ([]string, string, error) {
	options := parseRuntimeMenuOptions(output)
	if len(options) == 0 {
		return nil, "", fmt.Errorf("%s was not found", menuName)
	}
	current := -1
	target := -1
	for index, option := range options {
		if option.highlighted {
			current = index
		}
		if target < 0 && matches(option.text) {
			target = index
		}
	}
	if target < 0 {
		return nil, "", fmt.Errorf("target model option was not found in %s", menuName)
	}
	if current < 0 {
		return nil, "", fmt.Errorf("%s current selection was not found", menuName)
	}
	keys := []string{}
	key := "Down"
	count := target - current
	if count < 0 {
		key = "Up"
		count = -count
	}
	for i := 0; i < count; i++ {
		keys = append(keys, key)
	}
	keys = append(keys, "Enter")
	return keys, options[target].text, nil
}

func parseRuntimeMenuOptions(output string) []runtimeMenuOption {
	options := []runtimeMenuOption{}
	for _, line := range strings.Split(output, "\n") {
		text, highlighted, ok := parseRuntimeMenuLine(line)
		if ok {
			options = append(options, runtimeMenuOption{text: text, highlighted: highlighted})
		}
	}
	return options
}

func parseRuntimeMenuLine(line string) (string, bool, bool) {
	line = strings.TrimSpace(stripANSI(line))
	line = strings.Trim(line, "│ ")
	if line == "" {
		return "", false, false
	}
	highlighted := false
	for _, prefix := range []string{"❯", "➜", "→", "›", ">"} {
		if strings.HasPrefix(line, prefix) {
			highlighted = true
			line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			break
		}
	}
	line = strings.TrimSpace(regexp.MustCompile(`^\(?[0-9]+[.)]\s*`).ReplaceAllString(line, ""))
	line = strings.TrimSpace(strings.TrimPrefix(line, "•"))
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	if line == "" || strings.HasPrefix(line, "/") {
		return "", false, false
	}
	normalized := strings.ToLower(line)
	if strings.Contains(normalized, "working ") || strings.Contains(normalized, "context ") || strings.Contains(normalized, "esc to interrupt") {
		return "", false, false
	}
	optionLike := highlighted ||
		strings.Contains(normalized, "gpt-") ||
		strings.Contains(normalized, "gemini-") ||
		strings.Contains(normalized, "gemma-") ||
		strings.Contains(normalized, "claude-") ||
		strings.HasPrefix(normalized, "gemini ") ||
		strings.HasPrefix(normalized, "gemma ") ||
		strings.HasPrefix(normalized, "claude ") ||
		strings.Contains(normalized, " sonnet") ||
		strings.Contains(normalized, " opus") ||
		strings.Contains(normalized, " haiku") ||
		codexOptionMatchesEffort(normalized, "minimal") ||
		codexOptionMatchesEffort(normalized, "low") ||
		codexOptionMatchesEffort(normalized, "medium") ||
		codexOptionMatchesEffort(normalized, "high") ||
		codexOptionMatchesEffort(normalized, "xhigh")
	if !optionLike {
		return "", false, false
	}
	return line, highlighted, true
}

func codexOptionMatchesModel(option string, model string) bool {
	return runtimeOptionMatchesModel(option, model)
}

func runtimeOptionMatchesModel(option string, model string) bool {
	return strings.Contains(normalizedRuntimeMenuText(option), normalizedRuntimeMenuText(model))
}

func codexOptionMatchesEffort(option string, effort string) bool {
	option = normalizedRuntimeMenuText(option)
	effort = normalizedRuntimeMenuText(effort)
	switch effort {
	case "xhigh":
		return strings.Contains(option, "xhigh") || strings.Contains(option, "x high") || strings.Contains(option, "extra high")
	case "high":
		return regexp.MustCompile(`(^| )high($| )`).MatchString(option) && !strings.Contains(option, "xhigh") && !strings.Contains(option, "x high")
	case "medium", "low", "minimal":
		return regexp.MustCompile(`(^| )` + regexp.QuoteMeta(effort) + `($| )`).MatchString(option)
	default:
		return strings.Contains(option, effort)
	}
}

func normalizedRuntimeMenuText(value string) string {
	value = strings.ToLower(stripANSI(value))
	value = strings.ReplaceAll(value, "gpt-", "gpt ")
	value = strings.ReplaceAll(value, "gemini-", "gemini ")
	value = strings.ReplaceAll(value, "gemma-", "gemma ")
	value = strings.ReplaceAll(value, "claude-", "claude ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = regexp.MustCompile(`[^a-z0-9.]+`).ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
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
