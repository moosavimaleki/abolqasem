package codex

import (
	"ai-agent-manager/internal/appinfo"
	"context"
	"errors"
	"strings"
	"sync"

	codexprotocol "ai-agent-manager/internal/providers/codex/protocol"
)

type RPCClient interface {
	Call(ctx context.Context, method string, params any, result any) error
}

type Manager struct {
	client RPCClient

	mu          sync.Mutex
	initialized bool
	sessions    map[string]*Session
}

type Session struct {
	ChatID   string
	ThreadID string
	CWD      string
	Model    string
}

type StartSessionArgs struct {
	ChatID                  string
	CWD                     string
	Model                   string
	ServiceTier             string
	SessionToken            string
	PendingForkSessionToken string
}

func NewManager(client RPCClient) *Manager {
	return &Manager{
		client:   client,
		sessions: map[string]*Session{},
	}
}

func (m *Manager) StartSession(ctx context.Context, args StartSessionArgs) (string, error) {
	if args.ChatID == "" {
		return "", errors.New("chatId is required")
	}
	if err := m.initialize(ctx); err != nil {
		return "", err
	}

	if args.PendingForkSessionToken != "" {
		return m.forkThread(ctx, args)
	}
	if args.SessionToken != "" {
		threadID, err := m.resumeThread(ctx, args)
		if err == nil {
			return threadID, nil
		}
		if !isRecoverableResumeError(err) {
			return "", err
		}
	}
	return m.startThread(ctx, args)
}

func (m *Manager) Session(chatID string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[chatID]
	if !ok {
		return nil, false
	}
	cloned := *session
	return &cloned, true
}

func (m *Manager) initialize(ctx context.Context) error {
	m.mu.Lock()
	if m.initialized {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	var result any
	err := m.client.Call(ctx, "initialize", codexprotocol.InitializeParams{
		ClientInfo: codexprotocol.ClientInfo{
			Name:    appinfo.Name,
			Title:   appinfo.DisplayName,
			Version: "dev",
		},
		Capabilities: codexprotocol.Capabilities{
			ExperimentalAPI: true,
		},
	}, &result)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.initialized = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) startThread(ctx context.Context, args StartSessionArgs) (string, error) {
	var response codexprotocol.ThreadStartResponse
	if err := m.client.Call(ctx, "thread/start", codexprotocol.ThreadStartParams{
		Model:                  optionalString(args.Model),
		CWD:                    optionalString(args.CWD),
		ServiceTier:            optionalString(args.ServiceTier),
		ExperimentalRawEvents:  true,
		PersistExtendedHistory: true,
	}, &response); err != nil {
		return "", err
	}
	m.saveSession(args.ChatID, response.Thread.ID, args.CWD, response.Model)
	return response.Thread.ID, nil
}

func (m *Manager) resumeThread(ctx context.Context, args StartSessionArgs) (string, error) {
	var response codexprotocol.ThreadResumeResponse
	if err := m.client.Call(ctx, "thread/resume", codexprotocol.ThreadResumeParams{
		ThreadID:               args.SessionToken,
		Model:                  optionalString(args.Model),
		CWD:                    optionalString(args.CWD),
		ServiceTier:            optionalString(args.ServiceTier),
		PersistExtendedHistory: true,
	}, &response); err != nil {
		return "", err
	}
	m.saveSession(args.ChatID, response.Thread.ID, args.CWD, response.Model)
	return response.Thread.ID, nil
}

func (m *Manager) forkThread(ctx context.Context, args StartSessionArgs) (string, error) {
	var response codexprotocol.ThreadForkResponse
	if err := m.client.Call(ctx, "thread/fork", codexprotocol.ThreadForkParams{
		ThreadID:               args.PendingForkSessionToken,
		Model:                  optionalString(args.Model),
		CWD:                    optionalString(args.CWD),
		ServiceTier:            optionalString(args.ServiceTier),
		PersistExtendedHistory: true,
	}, &response); err != nil {
		return "", err
	}
	m.saveSession(args.ChatID, response.Thread.ID, args.CWD, response.Model)
	return response.Thread.ID, nil
}

func (m *Manager) saveSession(chatID string, threadID string, cwd string, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[chatID] = &Session{
		ChatID:   chatID,
		ThreadID: threadID,
		CWD:      cwd,
		Model:    model,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func isRecoverableResumeError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "thread not found") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "unknown thread") ||
		strings.Contains(message, "method not found")
}
