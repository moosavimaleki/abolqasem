package agent

import (
	"ai-agent-manager/internal/appinfo"
	"ai-agent-manager/internal/state"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"ai-agent-manager/internal/providers/providerexec"
)

const (
	CodexAgentName = "codex"
)

var (
	ErrThreadActive = errors.New("codex thread is already active")
	requestCounter  atomic.Int64
)

var codexSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)[^\s"']+`),
	regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s"']+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{8,}`),
}

type CodexRunError struct {
	Err     error
	LogPath string
}

func (e *CodexRunError) Error() string {
	if e == nil {
		return ""
	}
	message := "codex request failed"
	if e.Err != nil {
		message = e.Err.Error()
	}
	if e.LogPath != "" {
		return fmt.Sprintf("%s (log: %s)", message, e.LogPath)
	}
	return message
}

func (e *CodexRunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type CodexRequest struct {
	ThreadID string
	Message  string
	Cwd      string
	New      bool
	Model    string
}

type CodexResult struct {
	ThreadID       string
	TurnID         string
	TranscriptPath string
	Cwd            string
	Preview        string
	Status         string
	Model          string
}

type ModelInfo struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default"`
	Upgrade     string `json:"upgrade,omitempty"`
}

func CodexAvailable() bool {
	return providerexec.Executable("codex") != ""
}

func ListCodexModels(ctx context.Context) ([]ModelInfo, error) {
	client, err := startCodexClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if err := client.initialize(); err != nil {
		return nil, client.wrapErr(err)
	}

	result, err := client.call("model/list", map[string]any{"limit": 100})
	if err != nil {
		return nil, client.wrapErr(err)
	}

	rawItems, ok := result["data"].([]any)
	if !ok {
		return []ModelInfo{}, nil
	}
	models := make([]ModelInfo, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		model := ModelInfo{
			ID:          stringField(item, "id"),
			Model:       stringField(item, "model"),
			DisplayName: stringField(item, "displayName"),
			Description: stringField(item, "description"),
			IsDefault:   boolField(item, "isDefault"),
			Upgrade:     stringField(item, "upgrade"),
		}
		if model.Upgrade != "" {
			continue
		}
		if model.Model == "" {
			model.Model = model.ID
		}
		if model.ID == "" {
			model.ID = model.Model
		}
		if model.DisplayName == "" {
			model.DisplayName = model.Model
		}
		if model.Model != "" {
			models = append(models, model)
		}
	}
	return models, nil
}

func RunCodexTurn(ctx context.Context, req CodexRequest) (CodexResult, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return CodexResult{}, errors.New("message is empty")
	}
	model := strings.TrimSpace(req.Model)
	cwd := strings.TrimSpace(req.Cwd)
	if cwd == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cwd = home
		}
	}
	if cwd != "" {
		if stat, err := os.Stat(cwd); err != nil || !stat.IsDir() {
			return CodexResult{}, fmt.Errorf("cwd is not a readable directory: %s", cwd)
		}
	}

	client, err := startCodexClient(ctx)
	if err != nil {
		return CodexResult{}, err
	}
	defer client.Close()

	if err := client.initialize(); err != nil {
		return CodexResult{}, client.wrapErr(err)
	}

	thread, err := client.openThread(req.ThreadID, cwd, req.New, model)
	if err != nil {
		return CodexResult{}, client.wrapErr(err)
	}
	threadID := stringField(thread, "id")
	if threadID == "" {
		return CodexResult{}, client.wrapErr(errors.New("codex did not return a thread id"))
	}

	turn, err := client.turnStart(threadID, message, model)
	if err != nil {
		return CodexResult{}, client.wrapErr(err)
	}
	turnID := stringField(turn, "id")
	if turnID == "" {
		return CodexResult{}, client.wrapErr(errors.New("codex did not return a turn id"))
	}
	if err := client.waitForTurn(threadID, turnID); err != nil {
		return CodexResult{}, client.wrapErr(err)
	}

	return CodexResult{
		ThreadID:       threadID,
		TurnID:         turnID,
		TranscriptPath: stringField(thread, "path"),
		Cwd:            firstNonEmpty(stringField(thread, "cwd"), cwd),
		Preview:        messagePreview(message),
		Status:         "completed",
		Model:          model,
	}, nil
}

type codexClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lines   *bufio.Scanner
	stderr  *bytes.Buffer
	logFile *os.File
	logPath string
}

func startCodexClient(ctx context.Context) (*codexClient, error) {
	cmd := exec.CommandContext(ctx, providerexec.ExecutableOrName("codex"), "app-server")
	cmd.Env = state.CurrentProviderProxyEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stderr := &bytes.Buffer{}
	logFile, logPath := createCodexLogFile()
	if logFile != nil {
		cmd.Stderr = io.MultiWriter(stderr, codexRedactingWriter{writer: logFile})
	} else {
		cmd.Stderr = stderr
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	client := &codexClient{
		cmd:     cmd,
		stdin:   stdin,
		lines:   scanner,
		stderr:  stderr,
		logFile: logFile,
		logPath: logPath,
	}
	client.logf("started codex app-server pid=%d", cmd.Process.Pid)
	return client, nil
}

func (c *codexClient) Close() {
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	c.logf("closed codex app-server")
	if c.logFile != nil {
		_ = c.logFile.Close()
	}
}

func (c *codexClient) initialize() error {
	if _, err := c.call("initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    appinfo.CodexClientName,
			"title":   appinfo.DisplayName,
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"optOutNotificationMethods": []string{
				"command/exec/outputDelta",
				"item/agentMessage/delta",
				"item/plan/delta",
				"item/fileChange/outputDelta",
				"item/reasoning/summaryTextDelta",
				"item/reasoning/textDelta",
			},
		},
	}); err != nil {
		return err
	}
	return c.notify("initialized", nil)
}

func (c *codexClient) openThread(threadID, cwd string, forceNew bool, model string) (map[string]any, error) {
	if !forceNew && strings.TrimSpace(threadID) != "" {
		params := map[string]any{
			"threadId":       threadID,
			"cwd":            cwd,
			"approvalPolicy": "never",
			"sandbox":        "workspace-write",
		}
		if model != "" {
			params["model"] = model
		}
		result, err := c.call("thread/resume", params)
		if err != nil {
			return nil, err
		}
		thread := objectField(result, "thread")
		if statusType(thread["status"]) == "active" {
			return nil, ErrThreadActive
		}
		return thread, nil
	}

	params := map[string]any{
		"cwd":                cwd,
		"approvalPolicy":     "never",
		"sandbox":            "workspace-write",
		"serviceName":        appinfo.CodexClientName,
		"sessionStartSource": "startup",
	}
	if model != "" {
		params["model"] = model
	}
	result, err := c.call("thread/start", params)
	if err != nil {
		return nil, err
	}
	return objectField(result, "thread"), nil
}

func (c *codexClient) turnStart(threadID, message, model string) (map[string]any, error) {
	params := map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{
				"type":          "text",
				"text":          message,
				"text_elements": []any{},
			},
		},
	}
	if model != "" {
		params["model"] = model
	}
	result, err := c.call("turn/start", params)
	if err != nil {
		return nil, err
	}
	return objectField(result, "turn"), nil
}

func (c *codexClient) waitForTurn(threadID, turnID string) error {
	deadline := time.Now().Add(45 * time.Minute)
	for time.Now().Before(deadline) {
		msg, err := c.readMessage()
		if err != nil {
			return err
		}
		if c.isServerRequest(msg) {
			if err := c.rejectServerRequest(msg); err != nil {
				return err
			}
			continue
		}
		if stringField(msg, "method") != "turn/completed" {
			continue
		}
		params := objectField(msg, "params")
		if stringField(params, "threadId") != "" && stringField(params, "threadId") != threadID {
			continue
		}
		turn := objectField(params, "turn")
		if stringField(turn, "id") != turnID {
			continue
		}
		status := statusType(turn["status"])
		if status == "" || status == "completed" {
			return nil
		}
		if errText := codexTurnErrorMessage(turn); errText != "" {
			return fmt.Errorf("codex turn finished with status %s: %s", status, errText)
		}
		return fmt.Errorf("codex turn finished with status %s", status)
	}
	return errors.New("timed out waiting for codex turn to complete")
}

func (c *codexClient) call(method string, params any) (map[string]any, error) {
	id := requestCounter.Add(1)
	if err := c.write(map[string]any{
		"id":     id,
		"method": method,
		"params": params,
	}); err != nil {
		return nil, err
	}

	for {
		msg, err := c.readMessage()
		if err != nil {
			return nil, err
		}
		if c.isServerRequest(msg) {
			if err := c.rejectServerRequest(msg); err != nil {
				return nil, err
			}
			continue
		}
		if numericID(msg["id"]) != id {
			continue
		}
		if rpcErr, ok := msg["error"].(map[string]any); ok {
			return nil, fmt.Errorf("codex %s failed: %s", method, stringField(rpcErr, "message"))
		}
		return objectField(msg, "result"), nil
	}
}

func (c *codexClient) notify(method string, params any) error {
	msg := map[string]any{"method": method}
	if params != nil {
		msg["params"] = params
	}
	return c.write(msg)
}

func (c *codexClient) write(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.logRPCMessage(">>", msg)
	_, err = c.stdin.Write(data)
	return err
}

func (c *codexClient) readMessage() (map[string]any, error) {
	if !c.lines.Scan() {
		if err := c.lines.Err(); err != nil {
			return nil, err
		}
		errText := strings.TrimSpace(c.stderr.String())
		if errText == "" {
			errText = "codex app-server stopped"
		}
		errText = redactCodexLogText(errText)
		return nil, errors.New(errText)
	}
	line := string(c.lines.Bytes())
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		c.logf("<< %s", redactCodexLogText(line))
		return nil, err
	}
	c.logRPCMessage("<<", msg)
	return msg, nil
}

func (c *codexClient) wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return &CodexRunError{Err: err, LogPath: c.logPath}
}

func (c *codexClient) logf(format string, args ...any) {
	if c == nil || c.logFile == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(c.logFile, "%s %s\n", time.Now().Format(time.RFC3339Nano), line)
}

func (c *codexClient) logRPCMessage(prefix string, msg map[string]any) {
	if c == nil || c.logFile == nil {
		return
	}
	data, err := json.Marshal(redactCodexLogValue(msg))
	if err != nil {
		c.logf("%s %s", prefix, "[unserializable redacted payload]")
		return
	}
	c.logf("%s %s", prefix, string(data))
}

type codexRedactingWriter struct {
	writer io.Writer
}

func (w codexRedactingWriter) Write(p []byte) (int, error) {
	if w.writer == nil {
		return len(p), nil
	}
	_, err := w.writer.Write([]byte(redactCodexLogText(string(p))))
	return len(p), err
}

func redactCodexLogValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			if isCodexSensitiveLogKey(key) || isCodexContentLogKey(key) {
				redacted[key] = "[redacted]"
				continue
			}
			redacted[key] = redactCodexLogValue(item)
		}
		return redacted
	case []any:
		redacted := make([]any, 0, len(typed))
		for _, item := range typed {
			redacted = append(redacted, redactCodexLogValue(item))
		}
		return redacted
	case []map[string]any:
		redacted := make([]any, 0, len(typed))
		for _, item := range typed {
			redacted = append(redacted, redactCodexLogValue(item))
		}
		return redacted
	case string:
		return redactCodexLogText(typed)
	default:
		return typed
	}
}

func isCodexSensitiveLogKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "apikey", "api_key", "api-key", "authorization", "access_token", "accesstoken", "refresh_token", "refreshtoken", "secret", "password", "token":
		return true
	default:
		return false
	}
}

func isCodexContentLogKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "input", "text", "text_elements":
		return true
	default:
		return false
	}
}

func redactCodexLogText(text string) string {
	redacted := text
	for _, pattern := range codexSecretPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			if strings.Contains(match, " ") || strings.Contains(match, ":") || strings.Contains(match, "=") {
				for _, separator := range []string{"Bearer ", "bearer ", "=", ":"} {
					if index := strings.LastIndex(match, separator); index >= 0 {
						return match[:index+len(separator)] + "[redacted]"
					}
				}
			}
			return "[redacted]"
		})
	}
	return redacted
}

func createCodexLogFile() (*os.File, string) {
	logDir := filepath.Join(state.GetStateDir(), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, ""
	}
	now := time.Now()
	logPath := filepath.Join(logDir, fmt.Sprintf("codex-app-server-%s-%d-%d.log", now.Format("20060102-150405"), os.Getpid(), now.UnixNano()))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, ""
	}
	return file, logPath
}

func (c *codexClient) isServerRequest(msg map[string]any) bool {
	if msg["id"] == nil || msg["method"] == nil {
		return false
	}
	_, hasParams := msg["params"]
	_, hasResult := msg["result"]
	_, hasError := msg["error"]
	return hasParams && !hasResult && !hasError
}

func (c *codexClient) rejectServerRequest(msg map[string]any) error {
	method := stringField(msg, "method")
	decision := "cancel"
	switch method {
	case "applyPatchApproval", "execCommandApproval":
		decision = "denied"
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decision = "cancel"
	}
	return c.write(map[string]any{
		"id": msg["id"],
		"result": map[string]any{
			"decision": decision,
		},
	})
}

func objectField(value map[string]any, key string) map[string]any {
	if nested, ok := value[key].(map[string]any); ok {
		return nested
	}
	return map[string]any{}
}

func stringField(value map[string]any, key string) string {
	if raw, ok := value[key].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func numericID(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func boolField(value map[string]any, key string) bool {
	if raw, ok := value[key].(bool); ok {
		return raw
	}
	return false
}

func statusType(value any) string {
	if raw, ok := value.(string); ok {
		return raw
	}
	if obj, ok := value.(map[string]any); ok {
		return stringField(obj, "type")
	}
	return ""
}

func codexTurnErrorMessage(turn map[string]any) string {
	errObj := objectField(turn, "error")
	message := stringField(errObj, "message")
	if message == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(message), &parsed); err == nil {
		if detail := stringField(parsed, "detail"); detail != "" {
			return detail
		}
	}
	return message
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func messagePreview(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) <= 180 {
		return message
	}
	return string(runes[:180]) + "..."
}

func ProjectNameFromCwd(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "unknown"
	}
	base := filepath.Base(cwd)
	if base == "." || base == string(filepath.Separator) {
		return "unknown"
	}
	return base
}
