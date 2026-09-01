package login

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	codexrpc "abolqasem/internal/providers/codex/rpc"
)

// AppServerClient runs each device authorization in a temporary CODEX_HOME.
// The temporary auth file is imported by Service and is removed after the flow.
type AppServerClient struct {
	Executable string
	BaseEnv    []string
	TempRoot   string

	mu       sync.Mutex
	sessions map[string]*appServerLoginSession
}

type appServerLoginSession struct {
	home   string
	cmd    *exec.Cmd
	client *codexrpc.Client
	stdin  *appServerLoginTransport
}

type appServerLoginTransport struct {
	stdin io.WriteCloser
	mu    sync.Mutex
}

func (t *appServerLoginTransport) Send(message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.stdin.Write(append(append([]byte(nil), message...), '\n'))
	return err
}

func (c *AppServerClient) StartDeviceLogin(ctx context.Context) (Code, error) {
	if err := c.CleanupStaleTempHomes(); err != nil {
		return Code{}, err
	}
	home, err := os.MkdirTemp(c.TempRoot, "codex-manager-login-")
	if err != nil {
		return Code{}, fmt.Errorf("create temporary CODEX_HOME: %w", err)
	}
	session, err := c.start(ctx, home)
	if err != nil {
		_ = os.RemoveAll(home)
		return Code{}, err
	}
	var result struct {
		Type            string `json:"type"`
		LoginID         string `json:"loginId"`
		VerificationURL string `json:"verificationUrl"`
		UserCode        string `json:"userCode"`
	}
	err = session.client.Call(ctx, "account/login/start", map[string]string{"type": "chatgptDeviceCode"}, &result)
	if err != nil {
		c.closeSession(session)
		return Code{}, err
	}
	code := Code{LoginID: result.LoginID, VerificationURL: result.VerificationURL, UserCode: result.UserCode}
	if result.Type != "chatgptDeviceCode" || code.LoginID == "" || code.VerificationURL == "" || code.UserCode == "" {
		c.closeSession(session)
		return Code{}, errors.New("app-server returned an invalid device-login response")
	}
	c.mu.Lock()
	if c.sessions == nil {
		c.sessions = map[string]*appServerLoginSession{}
	}
	c.sessions[code.LoginID] = session
	c.mu.Unlock()
	return code, nil
}

// CleanupStaleTempHomes resolves an interrupted app restart deterministically:
// device-code attempts are not resumable without their app-server process, so
// their temporary auth homes are removed before a new attempt starts.
func (c *AppServerClient) CleanupStaleTempHomes() error {
	if c.TempRoot == "" {
		return nil
	}
	entries, err := os.ReadDir(c.TempRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read temporary login directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "codex-manager-login-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.TempRoot, entry.Name())); err != nil {
			return fmt.Errorf("clean interrupted device login: %w", err)
		}
	}
	return nil
}

func (c *AppServerClient) WaitDeviceLogin(ctx context.Context, loginID string) (map[string]any, error) {
	session := c.session(loginID)
	if session == nil {
		return nil, errors.New("device login was not found")
	}
	defer c.removeAndClose(loginID, session)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case notification := <-session.client.Notifications():
			if notification.Method != "account/login/completed" {
				continue
			}
			var completed struct {
				LoginID string  `json:"loginId"`
				Success bool    `json:"success"`
				Error   *string `json:"error"`
			}
			if err := json.Unmarshal(notification.Params, &completed); err != nil {
				return nil, fmt.Errorf("decode device login completion: %w", err)
			}
			if completed.LoginID != loginID {
				continue
			}
			if !completed.Success {
				if completed.Error != nil && *completed.Error != "" {
					return nil, errors.New(*completed.Error)
				}
				return nil, errors.New("device login did not complete")
			}
			return readTemporaryAuth(session.home)
		}
	}
}

func (c *AppServerClient) CancelDeviceLogin(ctx context.Context, loginID string) error {
	session := c.session(loginID)
	if session == nil {
		return nil
	}
	defer c.removeAndClose(loginID, session)
	var result any
	err := session.client.Call(ctx, "account/login/cancel", map[string]string{"loginId": loginID}, &result)
	return err
}

func (c *AppServerClient) start(ctx context.Context, home string) (*appServerLoginSession, error) {
	executable := c.Executable
	if executable == "" {
		executable = "codex"
	}
	command := exec.CommandContext(ctx, executable, "app-server")
	command.Env = withEnv(c.BaseEnv, "CODEX_HOME", home)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	transport := &appServerLoginTransport{stdin: stdin}
	session := &appServerLoginSession{home: home, cmd: command, stdin: transport, client: codexrpc.NewClient(transport)}
	if err := command.Start(); err != nil {
		return nil, err
	}
	go scanAppServerOutput(stdout, session.client.HandleMessage)
	go scanAppServerOutput(stderr, func(line []byte) error {
		session.client.RecordStderr(string(line))
		return nil
	})
	var initialized any
	if err := session.client.Call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "abolqasem", "title": "Abolqasem", "version": "dev"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}, &initialized); err != nil {
		c.closeSession(session)
		return nil, err
	}
	return session, nil
}

func scanAppServerOutput(reader io.Reader, handle func([]byte) error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		_ = handle(scanner.Bytes())
	}
}

func (c *AppServerClient) session(loginID string) *appServerLoginSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[loginID]
}

func (c *AppServerClient) removeAndClose(loginID string, session *appServerLoginSession) {
	c.mu.Lock()
	if c.sessions[loginID] == session {
		delete(c.sessions, loginID)
	}
	c.mu.Unlock()
	c.closeSession(session)
}

func (c *AppServerClient) closeSession(session *appServerLoginSession) {
	if session == nil {
		return
	}
	if session.cmd != nil && session.cmd.Process != nil {
		_ = session.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = session.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = session.cmd.Process.Kill()
			<-done
		}
	}
	_ = os.RemoveAll(session.home)
}

func readTemporaryAuth(home string) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return nil, fmt.Errorf("read temporary Codex auth: %w", err)
	}
	var authData map[string]any
	if err := json.Unmarshal(data, &authData); err != nil {
		return nil, fmt.Errorf("decode temporary Codex auth: %w", err)
	}
	return authData, nil
}

func withEnv(base []string, key string, value string) []string {
	env := append([]string(nil), base...)
	if len(env) == 0 {
		env = os.Environ()
	}
	prefix := key + "="
	for index, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
