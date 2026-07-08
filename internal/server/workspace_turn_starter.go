package server

import (
	"abolqasem/internal/appinfo"
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
	"sync"
	"time"

	claudeprovider "abolqasem/internal/providers/claude"
	codexprovider "abolqasem/internal/providers/codex"
	codexprotocol "abolqasem/internal/providers/codex/protocol"
	codexrpc "abolqasem/internal/providers/codex/rpc"
	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/eventstore"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

type workspaceTurnStarter struct {
	store *eventstore.Store
}

var workspaceCodexSessions = newWorkspaceCodexSessionManager()
var workspaceLoadProviderSettings = state.LoadSettings

type workspaceCodexSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*workspaceCodexSession
}

type workspaceCodexSession struct {
	chatID          string
	cwd             string
	threadID        string
	process         *workspaceCodexProcess
	turnMu          sync.Mutex
	idleDrainCancel context.CancelFunc
	idleDrainDone   chan struct{}
}

type workspaceAsyncTurn struct {
	cancelMu      sync.Mutex
	cancel        func() error
	events        chan agent.TurnEvent
	toolMu        sync.Mutex
	toolResponses map[string]chan workspaceToolResponse
}

type workspaceToolResponse struct {
	result any
	err    error
}

func newWorkspaceTurnStarter(store *eventstore.Store) *workspaceTurnStarter {
	return &workspaceTurnStarter{store: store}
}

func newWorkspaceCodexSessionManager() *workspaceCodexSessionManager {
	return &workspaceCodexSessionManager{
		sessions: map[string]*workspaceCodexSession{},
	}
}

func (m *workspaceCodexSessionManager) session(ctx context.Context, request agent.TurnRequest) (*workspaceCodexSession, error) {
	m.mu.Lock()
	existing := m.sessions[request.ChatID]
	if existing != nil && existing.reusableFor(request) {
		m.mu.Unlock()
		return existing, nil
	}
	if existing != nil {
		delete(m.sessions, request.ChatID)
	}
	m.mu.Unlock()

	if existing != nil {
		existing.close()
	}

	process, err := startWorkspaceCodexProcess(ctx, request.LocalPath, request.Env)
	if err != nil {
		return nil, err
	}
	if err := process.Initialize(ctx); err != nil {
		process.Close()
		return nil, process.wrapErr(err)
	}
	threadID, err := process.OpenThread(ctx, request)
	if err != nil {
		process.Close()
		return nil, process.wrapErr(err)
	}
	session := &workspaceCodexSession{
		chatID:   request.ChatID,
		cwd:      request.LocalPath,
		threadID: threadID,
		process:  process,
	}

	m.mu.Lock()
	if replaced := m.sessions[request.ChatID]; replaced != nil && replaced != session {
		m.mu.Unlock()
		session.close()
		return nil, errors.New("codex session was replaced")
	}
	m.sessions[request.ChatID] = session
	m.mu.Unlock()
	return session, nil
}

func (m *workspaceCodexSessionManager) remove(chatID string, process *workspaceCodexProcess) {
	m.mu.Lock()
	session := m.sessions[chatID]
	if session != nil && session.process == process {
		delete(m.sessions, chatID)
	}
	m.mu.Unlock()
}

func (m *workspaceCodexSessionManager) close(chatID string) {
	m.mu.Lock()
	session := m.sessions[chatID]
	if session != nil {
		delete(m.sessions, chatID)
	}
	m.mu.Unlock()
	if session != nil {
		session.close()
	}
}

func (s *workspaceCodexSession) reusableFor(request agent.TurnRequest) bool {
	if s == nil || s.process == nil || s.process.Exited() {
		return false
	}
	if request.PendingForkSessionToken != "" {
		return false
	}
	return s.cwd == request.LocalPath && s.threadID != ""
}

func (s *workspaceCodexSession) close() {
	if s == nil {
		return
	}
	s.stopIdleDrain()
	if s.process != nil {
		s.process.Close()
	}
}

func (s *workspaceCodexSession) startIdleDrain() {
	if s == nil || s.process == nil || s.process.Exited() {
		return
	}
	s.stopIdleDrain()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.idleDrainCancel = cancel
	s.idleDrainDone = done
	go func() {
		defer close(done)
		s.process.DrainIdleNotifications(ctx)
	}()
}

func (s *workspaceCodexSession) stopIdleDrain() {
	if s == nil || s.idleDrainCancel == nil {
		return
	}
	cancel := s.idleDrainCancel
	done := s.idleDrainDone
	s.idleDrainCancel = nil
	s.idleDrainDone = nil
	cancel()
	if done != nil {
		<-done
	}
}

func (s *workspaceTurnStarter) StartTurn(ctx context.Context, request agent.TurnRequest) (agent.Turn, error) {
	project, chat, err := s.turnContext(request.ChatID, request.ProjectID)
	if err != nil {
		return nil, err
	}
	request.ProjectID = project.ID
	request.LocalPath = project.LocalPath
	if request.SessionToken == "" {
		request.SessionToken = derefWorkspaceString(chat.SessionToken)
	}
	if request.PendingForkSessionToken == "" {
		request.PendingForkSessionToken = derefWorkspaceString(chat.PendingForkSessionToken)
	}

	switch request.Provider {
	case "codex":
		return startWorkspaceCodexTurn(ctx, request), nil
	case "claude":
		return startWorkspaceClaudeTurn(ctx, request), nil
	case "gemini":
		return startWorkspaceGeminiTurn(ctx, request), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", request.Provider)
	}
}

func (s *workspaceTurnStarter) turnContext(chatID string, projectID string) (readmodels.ProjectRecord, readmodels.ChatRecord, error) {
	storeState, err := s.store.LoadState()
	if err != nil {
		return readmodels.ProjectRecord{}, readmodels.ChatRecord{}, err
	}
	chat, ok := storeState.ChatsByID[chatID]
	if !ok || chat.DeletedAt != 0 {
		return readmodels.ProjectRecord{}, readmodels.ChatRecord{}, errors.New("chat not found")
	}
	if projectID == "" {
		projectID = chat.ProjectID
	}
	project, ok := storeState.ProjectsByID[projectID]
	if !ok || project.DeletedAt != 0 {
		return readmodels.ProjectRecord{}, readmodels.ChatRecord{}, errors.New("project not found")
	}
	if strings.TrimSpace(project.LocalPath) == "" {
		return readmodels.ProjectRecord{}, readmodels.ChatRecord{}, errors.New("project local path is empty")
	}
	return project, chat, nil
}

func (t *workspaceAsyncTurn) Cancel() error {
	if t == nil {
		return nil
	}
	t.cancelMu.Lock()
	cancel := t.cancel
	t.cancelMu.Unlock()
	if cancel == nil {
		return nil
	}
	return cancel()
}

func (t *workspaceAsyncTurn) Events() <-chan agent.TurnEvent {
	if t == nil {
		return nil
	}
	return t.events
}

func (t *workspaceAsyncTurn) setCancel(cancel func() error) {
	t.cancelMu.Lock()
	defer t.cancelMu.Unlock()
	t.cancel = cancel
}

func (t *workspaceAsyncTurn) RespondTool(_ context.Context, response agent.ToolResponse) error {
	if t == nil {
		return agent.ErrToolResponseUnsupported
	}
	t.toolMu.Lock()
	responseCh := t.toolResponses[response.ToolUseID]
	if responseCh != nil {
		delete(t.toolResponses, response.ToolUseID)
	}
	t.toolMu.Unlock()
	if responseCh == nil {
		return agent.ErrPendingToolNotFound
	}
	responseCh <- workspaceToolResponse{result: response.Result}
	return nil
}

func startWorkspaceClaudeTurn(parent context.Context, request agent.TurnRequest) agent.Turn {
	ctx, cancel := context.WithCancel(parent)
	turn := &workspaceAsyncTurn{
		cancel: func() error {
			cancel()
			return nil
		},
		events:        make(chan agent.TurnEvent, 32),
		toolResponses: map[string]chan workspaceToolResponse{},
	}
	go func() {
		defer close(turn.events)
		adapter := claudeprovider.NewAdapter("")
		sessionToken := request.SessionToken
		forkSession := false
		if request.PendingForkSessionToken != "" {
			sessionToken = request.PendingForkSessionToken
			forkSession = true
		}
		result, err := adapter.RunPromptResult(ctx, claudeprovider.PromptRequest{
			CWD:          request.LocalPath,
			Model:        request.Model,
			Effort:       request.Effort,
			PlanMode:     request.PlanMode,
			SessionToken: sessionToken,
			ForkSession:  forkSession,
			Prompt:       workspacePromptText(request.Content, request.Attachments),
			Env:          request.Env,
		})
		if err != nil {
			if ctx.Err() != nil {
				turn.events <- agent.TurnEvent{Type: agent.TurnEventCancelled}
				return
			}
			turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: err}
			return
		}
		if result.SessionToken != "" {
			turn.events <- agent.TurnEvent{Type: agent.TurnEventSessionToken, SessionToken: result.SessionToken}
		}
		for _, entry := range result.Entries {
			turn.events <- agent.TurnEvent{Type: agent.TurnEventTranscript, Entry: entry}
		}
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFinished}
	}()
	return turn
}

func startWorkspaceGeminiTurn(parent context.Context, request agent.TurnRequest) agent.Turn {
	ctx, cancel := context.WithCancel(parent)
	turn := &workspaceAsyncTurn{
		cancel: func() error {
			cancel()
			return nil
		},
		events:        make(chan agent.TurnEvent, 32),
		toolResponses: map[string]chan workspaceToolResponse{},
	}
	go func() {
		defer close(turn.events)
		started := time.Now()
		approvalMode := "yolo"
		if request.PlanMode {
			approvalMode = "plan"
		}
		args := []string{
			"--prompt", workspacePromptText(request.Content, request.Attachments),
			"--output-format", "text",
			"--approval-mode", approvalMode,
			"--skip-trust",
		}
		if strings.TrimSpace(request.SessionToken) != "" {
			args = append(args, "--resume", request.SessionToken)
		}
		if strings.TrimSpace(request.Model) != "" {
			args = append(args, "--model", request.Model)
		}
		cmd := exec.CommandContext(ctx, "gemini", args...)
		cmd.Env = state.CurrentProviderProxyEnvWithOverrides(request.Env)
		if request.LocalPath != "" {
			cmd.Dir = request.LocalPath
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				turn.events <- agent.TurnEvent{Type: agent.TurnEventCancelled}
				return
			}
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Message: message}
			return
		}
		output := strings.TrimSpace(stdout.String())
		if output != "" {
			turn.events <- agent.TurnEvent{
				Type:  agent.TurnEventTranscript,
				Entry: transcript.New(transcript.KindAssistantText, map[string]any{"text": output}),
			}
		}
		turn.events <- agent.TurnEvent{
			Type: agent.TurnEventTranscript,
			Entry: transcript.New(transcript.KindResult, map[string]any{
				"subtype":    "success",
				"isError":    false,
				"durationMs": float64(time.Since(started).Milliseconds()),
				"result":     "gemini completed",
			}),
		}
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFinished}
	}()
	return turn
}

func startWorkspaceCodexTurn(parent context.Context, request agent.TurnRequest) agent.Turn {
	ctx, cancel := context.WithCancel(parent)
	turn := &workspaceAsyncTurn{
		cancel: func() error {
			cancel()
			return nil
		},
		events:        make(chan agent.TurnEvent, 128),
		toolResponses: map[string]chan workspaceToolResponse{},
	}
	go runWorkspaceCodexTurn(ctx, cancel, request, turn)
	return turn
}

func runWorkspaceCodexTurn(ctx context.Context, turnCancel context.CancelFunc, request agent.TurnRequest, turn *workspaceAsyncTurn) {
	defer close(turn.events)

	session, err := workspaceCodexSessions.session(ctx, request)
	if err != nil {
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: err}
		return
	}
	session.turnMu.Lock()
	defer session.turnMu.Unlock()
	session.stopIdleDrain()
	process := session.process
	threadID := session.threadID
	if threadID != "" {
		turn.events <- agent.TurnEvent{Type: agent.TurnEventSessionToken, SessionToken: threadID}
	}

	turnID, err := process.StartTurn(ctx, threadID, request)
	if err != nil {
		workspaceCodexSessions.remove(request.ChatID, process)
		process.Close()
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: process.wrapErr(err)}
		return
	}
	turn.setCancel(func() error {
		cancelCtx, timeoutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer timeoutCancel()
		err := process.InterruptTurn(cancelCtx, threadID, turnID)
		turnCancel()
		if err != nil {
			workspaceCodexSessions.remove(request.ChatID, process)
			process.Close()
			return err
		}
		return nil
	})

	process.ForwardEvents(ctx, turn, threadID, turnID)
	if process.Exited() {
		workspaceCodexSessions.remove(request.ChatID, process)
		return
	}
	session.startIdleDrain()
}

type workspaceCodexProcess struct {
	cmd       *exec.Cmd
	client    *codexrpc.Client
	transport *workspaceCodexTransport
	logFile   *os.File
	logPath   string
	done      chan struct{}
	doneMu    sync.Mutex
	doneErr   error
}

type workspaceCodexTransport struct {
	stdin   io.WriteCloser
	logFile *os.File
	mu      sync.Mutex
}

func startWorkspaceCodexProcess(ctx context.Context, cwd string, env []string) (*workspaceCodexProcess, error) {
	// Abolqasem keeps codex app-server alive across turns; turn cancellation is sent via turn/interrupt.
	_ = ctx
	if settings, err := workspaceLoadProviderSettings(); err == nil {
		providerexec.SetConfiguredExecutables(settings.ProviderExecutables)
	}
	cmd := exec.Command(providerexec.ExecutableOrName("codex"), "app-server")
	cmd.Env = state.CurrentProviderProxyEnvWithOverrides(env)
	if cwd != "" {
		cmd.Dir = cwd
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	logFile, logPath := createWorkspaceCodexLogFile()
	transport := &workspaceCodexTransport{stdin: stdin, logFile: logFile}
	client := codexrpc.NewClient(transport)
	process := &workspaceCodexProcess{
		cmd:       cmd,
		client:    client,
		transport: transport,
		logFile:   logFile,
		logPath:   logPath,
		done:      make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, err
	}
	process.logf("started codex app-server pid=%d cwd=%s", cmd.Process.Pid, cwd)
	go process.scanStdout(stdout)
	go process.scanStderr(stderr)
	go func() {
		err := cmd.Wait()
		process.doneMu.Lock()
		process.doneErr = err
		process.doneMu.Unlock()
		close(process.done)
	}()
	return process, nil
}

func (p *workspaceCodexProcess) Initialize(ctx context.Context) error {
	var result any
	if err := p.client.Call(ctx, "initialize", codexprotocol.InitializeParams{
		ClientInfo: codexprotocol.ClientInfo{
			Name:    appinfo.Name,
			Title:   appinfo.DisplayName,
			Version: "dev",
		},
		Capabilities: codexprotocol.Capabilities{
			ExperimentalAPI: true,
		},
	}, &result); err != nil {
		return err
	}
	return p.notify("initialized", nil)
}

func (p *workspaceCodexProcess) OpenThread(ctx context.Context, request agent.TurnRequest) (string, error) {
	approvalPolicy := "never"
	sandbox := "danger-full-access"
	persistExtendedHistory := false
	model := optionalWorkspaceString(request.Model)
	cwd := optionalWorkspaceString(request.LocalPath)
	serviceTier := optionalWorkspaceString(request.ServiceTier)

	if request.PendingForkSessionToken != "" {
		var response codexprotocol.ThreadForkResponse
		err := p.client.Call(ctx, "thread/fork", codexprotocol.ThreadForkParams{
			ThreadID:               request.PendingForkSessionToken,
			Model:                  model,
			CWD:                    cwd,
			ServiceTier:            serviceTier,
			ApprovalPolicy:         &approvalPolicy,
			Sandbox:                &sandbox,
			PersistExtendedHistory: persistExtendedHistory,
		}, &response)
		if err != nil {
			return "", err
		}
		return response.Thread.ID, nil
	}

	if request.SessionToken != "" {
		var response codexprotocol.ThreadResumeResponse
		err := p.client.Call(ctx, "thread/resume", codexprotocol.ThreadResumeParams{
			ThreadID:               request.SessionToken,
			Model:                  model,
			CWD:                    cwd,
			ServiceTier:            serviceTier,
			ApprovalPolicy:         &approvalPolicy,
			Sandbox:                &sandbox,
			PersistExtendedHistory: persistExtendedHistory,
		}, &response)
		if err == nil {
			return response.Thread.ID, nil
		}
		if !isWorkspaceRecoverableCodexResumeError(err) {
			return "", err
		}
	}

	var response codexprotocol.ThreadStartResponse
	err := p.client.Call(ctx, "thread/start", codexprotocol.ThreadStartParams{
		Model:                  model,
		CWD:                    cwd,
		ServiceTier:            serviceTier,
		ApprovalPolicy:         &approvalPolicy,
		Sandbox:                &sandbox,
		ExperimentalRawEvents:  false,
		PersistExtendedHistory: persistExtendedHistory,
	}, &response)
	if err != nil {
		return "", err
	}
	return response.Thread.ID, nil
}

func (p *workspaceCodexProcess) StartTurn(ctx context.Context, threadID string, request agent.TurnRequest) (string, error) {
	model := optionalWorkspaceString(request.Model)
	effort := optionalWorkspaceString(request.Effort)
	serviceTier := optionalWorkspaceString(request.ServiceTier)
	approvalPolicy := "never"
	mode := "default"
	if request.PlanMode {
		mode = "plan"
	}

	params := codexprotocol.TurnStartParams{
		ThreadID: threadID,
		Input: []codexprotocol.TextUserInput{
			{
				Type:         "text",
				Text:         workspacePromptText(request.Content, request.Attachments),
				TextElements: []string{},
			},
		},
		ApprovalPolicy: &approvalPolicy,
		Model:          model,
		Effort:         effort,
		ServiceTier:    serviceTier,
		CollaborationMode: &codexprotocol.CollaborationMode{
			Mode: mode,
			Settings: codexprotocol.CollaborationModeSettings{
				Model:                 model,
				ReasoningEffort:       nil,
				DeveloperInstructions: nil,
			},
		},
	}

	var response codexprotocol.TurnStartResponse
	if err := p.client.Call(ctx, "turn/start", params, &response); err != nil {
		return "", err
	}
	return response.Turn.ID, nil
}

func (p *workspaceCodexProcess) InterruptTurn(ctx context.Context, threadID string, turnID string) error {
	if p == nil || p.client == nil || threadID == "" || turnID == "" {
		return nil
	}
	var result any
	return p.client.Call(ctx, "turn/interrupt", codexprotocol.TurnInterruptParams{
		ThreadID: threadID,
		TurnID:   turnID,
	}, &result)
}

func (p *workspaceCodexProcess) Exited() bool {
	if p == nil || p.done == nil {
		return true
	}
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *workspaceCodexProcess) DoneErr() error {
	if p == nil {
		return nil
	}
	p.doneMu.Lock()
	defer p.doneMu.Unlock()
	return p.doneErr
}

func (p *workspaceCodexProcess) ForwardEvents(ctx context.Context, turn *workspaceAsyncTurn, threadID string, turnID string) {
	normalizer := codexprovider.NewStreamNormalizer()
	for {
		select {
		case <-ctx.Done():
			turn.events <- agent.TurnEvent{Type: agent.TurnEventCancelled}
			return
		case <-p.done:
			if ctx.Err() != nil {
				turn.events <- agent.TurnEvent{Type: agent.TurnEventCancelled}
				return
			}
			if err := p.DoneErr(); err != nil {
				turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: p.wrapErr(err)}
				return
			}
			turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: p.wrapErr(errors.New("codex app-server stopped before turn completed"))}
			return
		case notification := <-p.client.Notifications():
			if notification.ID != "" {
				p.handleServerRequest(ctx, turn, notification)
				continue
			}
			for _, item := range normalizer.HandleNotification(notification) {
				forwardWorkspaceHarnessEvent(item, turn.events)
			}
			if notification.Method == "turn/completed" {
				status, message, matched := workspaceCodexTurnCompletion(notification.Params, turnID)
				if !matched {
					continue
				}
				switch status {
				case "failed":
					turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Message: firstNonEmptyWorkspaceString(message, "codex turn failed")}
				case "interrupted", "cancelled", "canceled":
					turn.events <- agent.TurnEvent{Type: agent.TurnEventCancelled}
				default:
					turn.events <- agent.TurnEvent{Type: agent.TurnEventFinished}
				}
				return
			}
			if notification.Method == "error" {
				message := workspaceCodexErrorMessage(notification.Params)
				turn.events <- agent.TurnEvent{Type: agent.TurnEventTranscript, Entry: workspaceCodexResultEntry("error", true, message)}
				turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Message: message}
				return
			}
		}
	}
}

func (p *workspaceCodexProcess) DrainIdleNotifications(ctx context.Context) {
	if p == nil || p.client == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.done:
			return
		case notification := <-p.client.Notifications():
			if notification.ID != "" {
				p.respondNoActiveTurn(notification.ID)
			}
		}
	}
}

func (p *workspaceCodexProcess) respondNoActiveTurn(id string) {
	if p == nil || p.transport == nil || strings.TrimSpace(id) == "" {
		return
	}
	data, err := json.Marshal(map[string]any{
		"id": id,
		"error": map[string]any{
			"message": "No active turn",
		},
	})
	if err != nil {
		return
	}
	_ = p.transport.Send(data)
}

func (p *workspaceCodexProcess) handleServerRequest(ctx context.Context, turn *workspaceAsyncTurn, notification codexrpc.Notification) {
	harnessEvents, response, err := codexprovider.HandleServerRequest(codexprovider.ServerRequest{
		ID:     notification.ID,
		Method: notification.Method,
		Params: notification.Params,
	}, codexprovider.RequestHandlers{
		OnToolRequest: func(request codexprovider.ToolRequest) (map[string]any, error) {
			return turn.waitForToolResponse(ctx, request)
		},
		OnApprovalRequest: func(codexprovider.ApprovalRequest) (string, error) {
			return "decline", nil
		},
	})
	if err != nil {
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: err}
		return
	}
	if notification.Method != "item/tool/requestUserInput" {
		for _, item := range harnessEvents {
			forwardWorkspaceHarnessEvent(item, turn.events)
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: err}
		return
	}
	if err := p.transport.Send(data); err != nil {
		turn.events <- agent.TurnEvent{Type: agent.TurnEventFailed, Error: err}
	}
}

func (t *workspaceAsyncTurn) waitForToolResponse(ctx context.Context, request codexprovider.ToolRequest) (map[string]any, error) {
	toolID := stringValue(request.Tool["toolId"])
	if toolID == "" {
		toolID = stringValue(request.Tool["id"])
	}
	if toolID == "" {
		return map[string]any{}, nil
	}
	responseCh := make(chan workspaceToolResponse, 1)
	t.toolMu.Lock()
	t.toolResponses[toolID] = responseCh
	t.toolMu.Unlock()
	defer func() {
		t.toolMu.Lock()
		delete(t.toolResponses, toolID)
		t.toolMu.Unlock()
	}()

	toolKind := stringValue(request.Tool["toolKind"])
	if toolKind == "" {
		toolKind = "ask_user_question"
	}
	t.events <- agent.TurnEvent{
		Type: agent.TurnEventPendingTool,
		PendingTool: &agent.PendingToolRequest{
			ToolUseID: toolID,
			ToolKind:  toolKind,
			ToolName:  stringValue(request.Tool["toolName"]),
			Input:     request.Tool["input"],
		},
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-responseCh:
		if response.err != nil {
			return nil, response.err
		}
		if result, ok := response.result.(map[string]any); ok {
			return result, nil
		}
		return map[string]any{"answers": response.result}, nil
	}
}

func (p *workspaceCodexProcess) Close() {
	if p == nil {
		return
	}
	if p.transport != nil && p.transport.stdin != nil {
		_ = p.transport.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	select {
	case <-p.done:
	default:
	}
	p.logf("closed codex app-server")
	if p.logFile != nil {
		_ = p.logFile.Close()
	}
}

func (p *workspaceCodexProcess) notify(method string, params any) error {
	payload := map[string]any{"method": method}
	if params != nil {
		payload["params"] = params
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.transport.Send(data)
}

func (p *workspaceCodexProcess) scanStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		p.logf("<< %s", redactWorkspaceCodexJSON(line))
		if err := p.client.HandleMessage(line); err != nil {
			p.logf("invalid codex message: %s", err.Error())
		}
	}
	if err := scanner.Err(); err != nil {
		p.logf("stdout scan failed: %s", err.Error())
	}
}

func (p *workspaceCodexProcess) scanStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			p.client.RecordStderr(redactWorkspaceCodexText(line))
			p.logf("!! %s", redactWorkspaceCodexText(line))
		}
	}
	if err := scanner.Err(); err != nil {
		p.logf("stderr scan failed: %s", err.Error())
	}
}

func (p *workspaceCodexProcess) wrapErr(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if stderr := strings.TrimSpace(p.client.Stderr()); stderr != "" {
		message = message + ": " + stderr
	}
	if p.logPath != "" {
		return fmt.Errorf("%s (log: %s)", message, p.logPath)
	}
	return errors.New(message)
}

func (p *workspaceCodexProcess) logf(format string, args ...any) {
	if p == nil || p.logFile == nil {
		return
	}
	_, _ = fmt.Fprintf(p.logFile, "%s %s\n", time.Now().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}

func (t *workspaceCodexTransport) Send(message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.logFile != nil {
		_, _ = fmt.Fprintf(t.logFile, "%s >> %s\n", time.Now().Format(time.RFC3339Nano), redactWorkspaceCodexJSON(message))
	}
	payload := append(append([]byte(nil), message...), '\n')
	_, err := t.stdin.Write(payload)
	return err
}

func forwardWorkspaceHarnessEvent(event codexprovider.HarnessEvent, out chan<- agent.TurnEvent) {
	switch event.Type {
	case "session_token":
		if event.SessionToken != "" {
			out <- agent.TurnEvent{Type: agent.TurnEventSessionToken, SessionToken: event.SessionToken}
		}
	case "transcript":
		if event.Entry != nil {
			out <- agent.TurnEvent{Type: agent.TurnEventTranscript, Entry: event.Entry}
		}
	}
}

func workspaceCodexTurnCompletion(raw json.RawMessage, turnID string) (string, string, bool) {
	var params struct {
		Turn map[string]any `json:"turn"`
	}
	if json.Unmarshal(raw, &params) != nil || params.Turn == nil {
		return "", "", true
	}
	if turnID != "" && stringValue(params.Turn["id"]) != "" && stringValue(params.Turn["id"]) != turnID {
		return "", "", false
	}
	status := strings.ToLower(stringValue(params.Turn["status"]))
	errorMessage := ""
	if errMap, ok := params.Turn["error"].(map[string]any); ok {
		errorMessage = stringValue(errMap["message"])
	}
	return status, errorMessage, true
}

func workspaceCodexErrorMessage(raw json.RawMessage) string {
	var params map[string]any
	if json.Unmarshal(raw, &params) != nil {
		return "codex reported an error"
	}
	if errMap, ok := params["error"].(map[string]any); ok {
		if message := stringValue(errMap["message"]); message != "" {
			return message
		}
	}
	if message := stringValue(params["message"]); message != "" {
		return message
	}
	return "codex reported an error"
}

func workspaceCodexResultEntry(subtype string, isError bool, message string) readmodels.TranscriptEntry {
	return transcript.New(transcript.KindResult, map[string]any{
		"subtype":    subtype,
		"isError":    isError,
		"durationMs": float64(0),
		"result":     message,
	})
}

func workspacePromptText(content string, attachments []readmodels.ChatAttachment) string {
	content = strings.TrimSpace(content)
	if len(attachments) == 0 {
		return content
	}
	lines := []string{"<abolqasem-attachments>"}
	for _, attachment := range attachments {
		lines = append(lines, fmt.Sprintf(
			`<attachment kind="%s" mime_type="%s" path="%s" project_path="%s" size_bytes="%d" display_name="%s" />`,
			escapeWorkspaceXML(attachment.Kind),
			escapeWorkspaceXML(attachment.MimeType),
			escapeWorkspaceXML(attachment.AbsolutePath),
			escapeWorkspaceXML(attachment.RelativePath),
			attachment.Size,
			escapeWorkspaceXML(attachment.DisplayName),
		))
	}
	lines = append(lines, "</abolqasem-attachments>")
	if content == "" {
		content = "Please inspect the attached files."
	}
	return strings.TrimSpace(content + "\n\n" + strings.Join(lines, "\n"))
}

func escapeWorkspaceXML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

func createWorkspaceCodexLogFile() (*os.File, string) {
	logDir := filepath.Join(state.GetStateDir(), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, ""
	}
	now := time.Now()
	logPath := filepath.Join(logDir, fmt.Sprintf("codex-workspace-%s-%d-%d.log", now.Format("20060102-150405"), os.Getpid(), now.UnixNano()))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, ""
	}
	return file, logPath
}

func optionalWorkspaceString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func isWorkspaceRecoverableCodexResumeError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "thread not found") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "unknown thread") ||
		strings.Contains(message, "method not found")
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func derefWorkspaceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmptyWorkspaceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var workspaceCodexSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)[^\s"']+`),
	regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s"']+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{8,}`),
}

func redactWorkspaceCodexJSON(data []byte) string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return redactWorkspaceCodexText(string(data))
	}
	redacted, err := json.Marshal(redactWorkspaceCodexValue(value))
	if err != nil {
		return "[unserializable redacted payload]"
	}
	return string(redacted)
}

func redactWorkspaceCodexValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isWorkspaceCodexSensitiveLogKey(key) || isWorkspaceCodexContentLogKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactWorkspaceCodexValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactWorkspaceCodexValue(item))
		}
		return out
	case string:
		return redactWorkspaceCodexText(typed)
	default:
		return typed
	}
}

func isWorkspaceCodexSensitiveLogKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "apikey", "api_key", "api-key", "authorization", "access_token", "accesstoken", "refresh_token", "refreshtoken", "secret", "password", "token":
		return true
	default:
		return false
	}
}

func isWorkspaceCodexContentLogKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "input", "text", "text_elements":
		return true
	default:
		return false
	}
}

func redactWorkspaceCodexText(text string) string {
	redacted := text
	for _, pattern := range workspaceCodexSecretPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			for _, separator := range []string{"Bearer ", "bearer ", "=", ":"} {
				if index := strings.LastIndex(match, separator); index >= 0 {
					return match[:index+len(separator)] + "[redacted]"
				}
			}
			return "[redacted]"
		})
	}
	return redacted
}
