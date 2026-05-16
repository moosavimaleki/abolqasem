package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type SessionHandle interface {
	SendPrompt(ctx context.Context, content string) error
	SetModel(ctx context.Context, model string) error
	SetPermissionMode(ctx context.Context, planMode bool) error
	Interrupt(ctx context.Context) error
	Close() error
}

type SessionStarter interface {
	Start(ctx context.Context, args StartSessionArgs) (SessionHandle, error)
}

type SessionStarterFunc func(context.Context, StartSessionArgs) (SessionHandle, error)

func (fn SessionStarterFunc) Start(ctx context.Context, args StartSessionArgs) (SessionHandle, error) {
	return fn(ctx, args)
}

type SessionManager struct {
	starter  SessionStarter
	sessions map[string]*SessionState
}

type StartSessionArgs struct {
	ChatID       string
	LocalPath    string
	Model        string
	Effort       string
	PlanMode     bool
	SessionToken string
	ForkSession  bool
}

type SessionState struct {
	ID           string
	ChatID       string
	Session      SessionHandle
	LocalPath    string
	Model        string
	Effort       string
	PlanMode     bool
	SessionToken string
}

func NewSessionManager(starter SessionStarter) *SessionManager {
	return &SessionManager{
		starter:  starter,
		sessions: map[string]*SessionState{},
	}
}

func (m *SessionManager) StartTurn(ctx context.Context, args StartSessionArgs, prompt string) (SessionHandle, error) {
	session, err := m.ensureSession(ctx, args)
	if err != nil {
		return nil, err
	}
	if err := session.Session.SendPrompt(ctx, prompt); err != nil {
		return nil, err
	}
	return session.Session, nil
}

func (m *SessionManager) CloseChat(chatID string) error {
	session := m.sessions[chatID]
	if session == nil {
		return nil
	}
	delete(m.sessions, chatID)
	return session.Session.Close()
}

func (m *SessionManager) Session(chatID string) (*SessionState, bool) {
	session := m.sessions[chatID]
	if session == nil {
		return nil, false
	}
	cloned := *session
	return &cloned, true
}

func (m *SessionManager) ensureSession(ctx context.Context, args StartSessionArgs) (*SessionState, error) {
	session := m.sessions[args.ChatID]
	if session == nil || session.LocalPath != args.LocalPath || session.Effort != args.Effort || args.ForkSession {
		if session != nil {
			_ = session.Session.Close()
			delete(m.sessions, args.ChatID)
		}
		started, err := m.starter.Start(ctx, args)
		if err != nil {
			return nil, err
		}
		session = &SessionState{
			ID:           randomID(),
			ChatID:       args.ChatID,
			Session:      started,
			LocalPath:    args.LocalPath,
			Model:        args.Model,
			Effort:       args.Effort,
			PlanMode:     args.PlanMode,
			SessionToken: args.SessionToken,
		}
		m.sessions[args.ChatID] = session
		return session, nil
	}

	if session.Model != args.Model {
		if err := session.Session.SetModel(ctx, args.Model); err != nil {
			return nil, err
		}
		session.Model = args.Model
	}
	if session.PlanMode != args.PlanMode {
		if err := session.Session.SetPermissionMode(ctx, args.PlanMode); err != nil {
			return nil, err
		}
		session.PlanMode = args.PlanMode
	}
	return session, nil
}

func randomID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "claude-session"
	}
	return hex.EncodeToString(data[:])
}
