package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"abolqasem/internal/providers/catalog"
	"abolqasem/internal/workspace/readmodels"
)

var (
	ErrChatAlreadyRunning       = errors.New("chat is already running")
	ErrQueuedNotFound           = errors.New("queued message not found")
	ErrPendingToolNotFound      = errors.New("pending tool not found")
	ErrToolResponseUnsupported  = errors.New("active turn does not support tool responses")
	ErrTurnStarterNotConfigured = errors.New("turn starter is not configured")
)

type Store interface {
	CreateChat(projectID string) (readmodels.ChatRecord, error)
	RequireChat(chatID string) (readmodels.ChatRecord, error)
	SetChatProvider(chatID string, provider string) error
	SetPlanMode(chatID string, planMode bool) error
	SetSessionToken(chatID string, sessionToken string) error
	AppendUserPrompt(chatID string, content string, attachments []readmodels.ChatAttachment, steered bool) error
	AppendTranscriptEntry(chatID string, entry readmodels.TranscriptEntry) error
	RecordTurnStarted(chatID string) error
	RecordTurnFinished(chatID string) error
	RecordTurnFailed(chatID string, message string) error
	RecordTurnCancelled(chatID string) error
	EnqueueMessage(chatID string, message QueueMessageInput) (readmodels.QueuedChatMessage, error)
	GetQueuedMessages(chatID string) []readmodels.QueuedChatMessage
	GetQueuedMessage(chatID string, queuedMessageID string) (readmodels.QueuedChatMessage, bool)
	RemoveQueuedMessage(chatID string, queuedMessageID string) error
}

type TurnStarter interface {
	StartTurn(ctx context.Context, request TurnRequest) (Turn, error)
}

type Turn interface {
	Cancel() error
}

type TurnEventSource interface {
	Events() <-chan TurnEvent
}

type ToolResponder interface {
	RespondTool(ctx context.Context, response ToolResponse) error
}

type ToolEventRecorder interface {
	RecordToolCall(chatID string, request PendingToolRequest) error
	RecordToolResult(chatID string, toolUseID string, result any) error
}

type SystemInitRecorder interface {
	EnsureSystemInit(chatID string, provider string, model string) error
}

type CheckpointRecorder interface {
	RecordCheckpointBeforeUserPrompt(chatID string, content string, attachments []readmodels.ChatAttachment, steered bool) error
}

type CheckpointTurnBinder interface {
	RecordCheckpointTurnBoundary(chatID string, threadID string, turnID string) error
}

type TurnStarterFunc func(ctx context.Context, request TurnRequest) (Turn, error)

func (fn TurnStarterFunc) StartTurn(ctx context.Context, request TurnRequest) (Turn, error) {
	return fn(ctx, request)
}

type Coordinator struct {
	store         Store
	starter       TurnStarter
	onStateChange func(chatID string)

	mu     sync.Mutex
	active map[string]*ActiveTurn
}

type ActiveTurn struct {
	ChatID      string
	ProjectID   string
	Provider    string
	Model       string
	Effort      string
	ServiceTier string
	PlanMode    bool
	Status      readmodels.AbolqasemStatus
	Turn        Turn
	StartedAt   time.Time
	Cancel      context.CancelFunc
	PendingTool *PendingToolRequest
	Draining    bool
}

type PendingToolRequest struct {
	ToolUseID string
	ToolKind  string
	ToolName  string
	Input     any
}

type PendingToolSnapshot struct {
	ToolUseID string `json:"toolUseId"`
	ToolKind  string `json:"toolKind"`
}

type SendCommand struct {
	ChatID       string
	ProjectID    string
	Content      string
	Attachments  []readmodels.ChatAttachment
	Provider     string
	Model        string
	ModelOptions *catalog.ModelOptions
	Effort       string
	PlanMode     bool
}

type QueueMessageInput struct {
	Content      string
	Attachments  []readmodels.ChatAttachment
	Provider     string
	Model        string
	ModelOptions *catalog.ModelOptions
	PlanMode     bool
}

type TurnRequest struct {
	ChatID                  string
	ProjectID               string
	LocalPath               string
	Provider                string
	Content                 string
	Attachments             []readmodels.ChatAttachment
	Model                   string
	Effort                  string
	ServiceTier             string
	PlanMode                bool
	SessionToken            string
	PendingForkSessionToken string
	Env                     []string
}

type SendResult struct {
	ChatID          string `json:"chatId"`
	Queued          bool   `json:"queued,omitempty"`
	QueuedMessageID string `json:"queuedMessageId,omitempty"`
}

type ToolResponseCommand struct {
	ChatID    string
	ToolUseID string
	Result    any
}

type ToolResponse struct {
	ToolUseID string
	Result    any
}

type TurnEventKind string

const (
	TurnEventTranscript   TurnEventKind = "transcript"
	TurnEventSessionToken TurnEventKind = "session_token"
	TurnEventStarted      TurnEventKind = "turn_started"
	TurnEventPendingTool  TurnEventKind = "pending_tool"
	TurnEventDraining     TurnEventKind = "draining"
	TurnEventFinished     TurnEventKind = "finished"
	TurnEventFailed       TurnEventKind = "failed"
	TurnEventCancelled    TurnEventKind = "cancelled"
)

type TurnEvent struct {
	Type         TurnEventKind
	Entry        readmodels.TranscriptEntry
	SessionToken string
	TurnID       string
	PendingTool  *PendingToolRequest
	Draining     bool
	Error        error
	Message      string
}

func NewCoordinator(store Store, starter TurnStarter, onStateChange func(chatID string)) *Coordinator {
	if starter == nil {
		starter = TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
			return nil, ErrTurnStarterNotConfigured
		})
	}
	if onStateChange == nil {
		onStateChange = func(string) {}
	}
	return &Coordinator{
		store:         store,
		starter:       starter,
		onStateChange: onStateChange,
		active:        map[string]*ActiveTurn{},
	}
}

func (c *Coordinator) ActiveStatuses() map[string]readmodels.AbolqasemStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	statuses := map[string]readmodels.AbolqasemStatus{}
	for chatID, turn := range c.active {
		statuses[chatID] = turn.Status
	}
	return statuses
}

func (c *Coordinator) DrainingChatIDs() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	draining := map[string]bool{}
	for chatID, turn := range c.active {
		if turn.Draining {
			draining[chatID] = true
		}
	}
	return draining
}

func (c *Coordinator) PendingTool(chatID string) *PendingToolSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	active := c.active[chatID]
	if active == nil || active.PendingTool == nil {
		return nil
	}
	return &PendingToolSnapshot{
		ToolUseID: active.PendingTool.ToolUseID,
		ToolKind:  active.PendingTool.ToolKind,
	}
}

func (c *Coordinator) SetPendingTool(chatID string, request PendingToolRequest) error {
	c.mu.Lock()
	active := c.active[chatID]
	if active == nil {
		c.mu.Unlock()
		return errors.New("chat turn is not active")
	}
	active.Status = readmodels.StatusWaitingForUser
	active.PendingTool = &request
	c.mu.Unlock()

	if recorder, ok := c.store.(ToolEventRecorder); ok {
		if err := recorder.RecordToolCall(chatID, request); err != nil {
			return err
		}
	}
	c.emitStateChange(chatID)
	return nil
}

func (c *Coordinator) RespondTool(ctx context.Context, command ToolResponseCommand) error {
	c.mu.Lock()
	active := c.active[command.ChatID]
	if active == nil || active.PendingTool == nil || active.PendingTool.ToolUseID != command.ToolUseID {
		c.mu.Unlock()
		return ErrPendingToolNotFound
	}
	responder, ok := active.Turn.(ToolResponder)
	if !ok {
		c.mu.Unlock()
		return ErrToolResponseUnsupported
	}
	c.mu.Unlock()

	if err := responder.RespondTool(ctx, ToolResponse{
		ToolUseID: command.ToolUseID,
		Result:    command.Result,
	}); err != nil {
		return err
	}
	if recorder, ok := c.store.(ToolEventRecorder); ok {
		if err := recorder.RecordToolResult(command.ChatID, command.ToolUseID, command.Result); err != nil {
			return err
		}
	}

	c.mu.Lock()
	if active = c.active[command.ChatID]; active != nil && active.PendingTool != nil && active.PendingTool.ToolUseID == command.ToolUseID {
		active.PendingTool = nil
		active.Status = readmodels.StatusRunning
	}
	c.mu.Unlock()
	c.emitStateChange(command.ChatID)
	return nil
}

func (c *Coordinator) Send(ctx context.Context, command SendCommand) (SendResult, error) {
	chatID := command.ChatID
	if chatID == "" {
		if command.ProjectID == "" {
			return SendResult{}, errors.New("missing projectId for new chat")
		}
		chat, err := c.store.CreateChat(command.ProjectID)
		if err != nil {
			return SendResult{}, err
		}
		chatID = chat.ID
	}

	if c.isActive(chatID) {
		queued, err := c.store.EnqueueMessage(chatID, QueueMessageInput{
			Content:      command.Content,
			Attachments:  command.Attachments,
			Provider:     command.Provider,
			Model:        command.Model,
			ModelOptions: command.ModelOptions,
			PlanMode:     command.PlanMode,
		})
		if err != nil {
			return SendResult{}, err
		}
		c.emitStateChange(chatID)
		return SendResult{ChatID: chatID, Queued: true, QueuedMessageID: queued.ID}, nil
	}

	if err := c.startTurn(ctx, chatID, command.Content, command.Attachments, command.Provider, command.Model, command.ModelOptions, command.Effort, command.PlanMode, false); err != nil {
		return SendResult{}, err
	}
	return SendResult{ChatID: chatID}, nil
}

func (c *Coordinator) Enqueue(command SendCommand) (string, error) {
	queued, err := c.store.EnqueueMessage(command.ChatID, QueueMessageInput{
		Content:      command.Content,
		Attachments:  command.Attachments,
		Provider:     command.Provider,
		Model:        command.Model,
		ModelOptions: command.ModelOptions,
		PlanMode:     command.PlanMode,
	})
	if err != nil {
		return "", err
	}
	c.emitStateChange(command.ChatID)
	return queued.ID, nil
}

func (c *Coordinator) Dequeue(chatID string, queuedMessageID string) error {
	if _, ok := c.store.GetQueuedMessage(chatID, queuedMessageID); !ok {
		return ErrQueuedNotFound
	}
	if err := c.store.RemoveQueuedMessage(chatID, queuedMessageID); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return nil
}

func (c *Coordinator) Finish(chatID string) error {
	c.clearActive(chatID)
	if err := c.store.RecordTurnFinished(chatID); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return c.maybeStartNextQueuedMessage(context.Background(), chatID)
}

func (c *Coordinator) Cancel(chatID string) error {
	c.mu.Lock()
	active := c.active[chatID]
	if active != nil {
		delete(c.active, chatID)
	}
	c.mu.Unlock()

	if active == nil {
		return nil
	}
	if active.Cancel != nil {
		active.Cancel()
	}
	if active.Turn != nil {
		_ = active.Turn.Cancel()
	}
	if err := c.store.RecordTurnCancelled(chatID); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return nil
}

func (c *Coordinator) StopDraining(chatID string) {
	c.mu.Lock()
	active := c.active[chatID]
	if active != nil {
		active.Draining = false
	}
	c.mu.Unlock()
	if active != nil {
		c.emitStateChange(chatID)
	}
}

func (c *Coordinator) maybeStartNextQueuedMessage(ctx context.Context, chatID string) error {
	if c.isActive(chatID) {
		return nil
	}
	queuedMessages := c.store.GetQueuedMessages(chatID)
	if len(queuedMessages) == 0 {
		return nil
	}
	next := queuedMessages[0]
	if err := c.store.RemoveQueuedMessage(chatID, next.ID); err != nil {
		return err
	}
	return c.startTurn(ctx, chatID, next.Content, next.Attachments, derefString(next.Provider), next.Model, next.ModelOptions, "", derefBool(next.PlanMode), false)
}

func (c *Coordinator) startTurn(
	ctx context.Context,
	chatID string,
	content string,
	attachments []readmodels.ChatAttachment,
	provider string,
	model string,
	modelOptions *catalog.ModelOptions,
	legacyEffort string,
	planMode bool,
	steered bool,
) error {
	chat, err := c.store.RequireChat(chatID)
	if err != nil {
		return err
	}

	resolvedProvider := resolveProvider(provider, chat.Provider)
	settings := providerSettings(resolvedProvider, model, modelOptions, legacyEffort, planMode)
	turnCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	if c.active[chatID] != nil {
		cancel()
		c.mu.Unlock()
		return ErrChatAlreadyRunning
	}
	active := &ActiveTurn{
		ChatID:      chatID,
		ProjectID:   chat.ProjectID,
		Provider:    resolvedProvider,
		Model:       settings.model,
		Effort:      settings.effort,
		ServiceTier: settings.serviceTier,
		PlanMode:    settings.planMode,
		Status:      initialStatus(resolvedProvider),
		StartedAt:   time.Now(),
		Cancel:      cancel,
	}
	c.active[chatID] = active
	c.mu.Unlock()

	if chat.Provider == nil {
		if err := c.store.SetChatProvider(chatID, resolvedProvider); err != nil {
			cancel()
			c.clearActive(chatID)
			return err
		}
	}
	if err := c.store.SetPlanMode(chatID, settings.planMode); err != nil {
		cancel()
		c.clearActive(chatID)
		return err
	}
	if recorder, ok := c.store.(SystemInitRecorder); ok {
		if err := recorder.EnsureSystemInit(chatID, resolvedProvider, settings.model); err != nil {
			cancel()
			c.clearActive(chatID)
			return err
		}
	}
	if recorder, ok := c.store.(CheckpointRecorder); ok {
		_ = recorder.RecordCheckpointBeforeUserPrompt(chatID, content, attachments, steered)
	}
	if err := c.store.AppendUserPrompt(chatID, content, attachments, steered); err != nil {
		cancel()
		c.clearActive(chatID)
		return err
	}
	if err := c.store.RecordTurnStarted(chatID); err != nil {
		cancel()
		c.clearActive(chatID)
		return err
	}

	turn, err := c.starter.StartTurn(turnCtx, TurnRequest{
		ChatID:                  chatID,
		ProjectID:               chat.ProjectID,
		Provider:                resolvedProvider,
		Content:                 content,
		Attachments:             attachments,
		Model:                   settings.model,
		Effort:                  settings.effort,
		ServiceTier:             settings.serviceTier,
		PlanMode:                settings.planMode,
		SessionToken:            derefString(chat.SessionToken),
		PendingForkSessionToken: derefString(chat.PendingForkSessionToken),
	})
	if err != nil {
		cancel()
		c.clearActive(chatID)
		_ = c.store.RecordTurnFailed(chatID, err.Error())
		c.emitStateChange(chatID)
		return fmt.Errorf("start turn: %w", err)
	}

	c.mu.Lock()
	if c.active[chatID] == active {
		active.Turn = turn
	}
	c.mu.Unlock()
	if source, ok := turn.(TurnEventSource); ok {
		if events := source.Events(); events != nil {
			go c.consumeTurnEvents(turnCtx, chatID, active, events)
		}
	}
	c.emitStateChange(chatID)
	return nil
}

func (c *Coordinator) consumeTurnEvents(ctx context.Context, chatID string, active *ActiveTurn, events <-chan TurnEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				_ = c.finishFromProvider(chatID, active)
				return
			}
			if !c.activeMatches(chatID, active) {
				return
			}
			if c.handleTurnEvent(chatID, active, event) {
				return
			}
		}
	}
}

func (c *Coordinator) handleTurnEvent(chatID string, active *ActiveTurn, event TurnEvent) bool {
	switch event.Type {
	case TurnEventTranscript:
		if event.Entry != nil {
			if err := c.store.AppendTranscriptEntry(chatID, event.Entry); err != nil {
				_ = c.failFromProvider(chatID, active, err)
				return true
			}
			c.emitStateChange(chatID)
		}
	case TurnEventSessionToken:
		if event.SessionToken != "" {
			if err := c.store.SetSessionToken(chatID, event.SessionToken); err != nil {
				_ = c.failFromProvider(chatID, active, err)
				return true
			}
			c.emitStateChange(chatID)
		}
	case TurnEventStarted:
		if binder, ok := c.store.(CheckpointTurnBinder); ok && event.SessionToken != "" && event.TurnID != "" {
			if err := binder.RecordCheckpointTurnBoundary(chatID, event.SessionToken, event.TurnID); err != nil {
				_ = c.failFromProvider(chatID, active, err)
				return true
			}
		}
	case TurnEventPendingTool:
		if event.PendingTool != nil {
			if err := c.SetPendingTool(chatID, *event.PendingTool); err != nil {
				_ = c.failFromProvider(chatID, active, err)
				return true
			}
		}
	case TurnEventDraining:
		c.setDraining(chatID, active, event.Draining)
	case TurnEventFinished:
		_ = c.finishFromProvider(chatID, active)
		return true
	case TurnEventFailed:
		_ = c.failFromProvider(chatID, active, eventError(event))
		return true
	case TurnEventCancelled:
		_ = c.cancelFromProvider(chatID, active)
		return true
	}
	return false
}

func (c *Coordinator) finishFromProvider(chatID string, active *ActiveTurn) error {
	if !c.removeActive(chatID, active) {
		return nil
	}
	if err := c.store.RecordTurnFinished(chatID); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return c.maybeStartNextQueuedMessage(context.Background(), chatID)
}

func (c *Coordinator) failFromProvider(chatID string, active *ActiveTurn, err error) error {
	if !c.removeActive(chatID, active) {
		return nil
	}
	message := "provider failed"
	if err != nil {
		message = err.Error()
	}
	if err := c.store.RecordTurnFailed(chatID, message); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return nil
}

func (c *Coordinator) cancelFromProvider(chatID string, active *ActiveTurn) error {
	if !c.removeActive(chatID, active) {
		return nil
	}
	if err := c.store.RecordTurnCancelled(chatID); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return nil
}

func (c *Coordinator) setDraining(chatID string, active *ActiveTurn, draining bool) {
	c.mu.Lock()
	if c.active[chatID] == active {
		active.Draining = draining
		c.mu.Unlock()
		c.emitStateChange(chatID)
		return
	}
	c.mu.Unlock()
}

func (c *Coordinator) activeMatches(chatID string, active *ActiveTurn) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active[chatID] == active
}

func (c *Coordinator) removeActive(chatID string, active *ActiveTurn) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[chatID] != active {
		return false
	}
	delete(c.active, chatID)
	if active.Cancel != nil {
		active.Cancel()
	}
	return true
}

func (c *Coordinator) isActive(chatID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active[chatID] != nil
}

func (c *Coordinator) clearActive(chatID string) {
	c.mu.Lock()
	delete(c.active, chatID)
	c.mu.Unlock()
}

func (c *Coordinator) emitStateChange(chatID string) {
	c.onStateChange(chatID)
}

type resolvedProviderSettings struct {
	model       string
	effort      string
	serviceTier string
	planMode    bool
}

func resolveProvider(requested string, current *string) string {
	if current != nil && *current != "" {
		return *current
	}
	if requested != "" {
		return requested
	}
	return "claude"
}

func providerSettings(provider string, model string, modelOptions *catalog.ModelOptions, legacyEffort string, planMode bool) resolvedProviderSettings {
	entry := catalog.GetOrDefault(provider)
	if entry.ID == "claude" {
		normalizedModel := catalog.NormalizeServerModel(entry.ID, model)
		options := catalog.NormalizeClaudeModelOptions(normalizedModel, modelOptions, legacyEffort)
		return resolvedProviderSettings{
			model:    catalog.ResolveClaudeAPIModelID(normalizedModel, options.ContextWindow),
			effort:   options.ReasoningEffort,
			planMode: entry.SupportsPlanMode && planMode,
		}
	}

	if entry.ID == "codex" {
		options := catalog.NormalizeCodexModelOptions(modelOptions, legacyEffort)
		return resolvedProviderSettings{
			model:       catalog.NormalizeServerModel(entry.ID, model),
			effort:      options.ReasoningEffort,
			serviceTier: catalog.CodexServiceTierFromModelOptions(options),
			planMode:    entry.SupportsPlanMode && planMode,
		}
	}

	return resolvedProviderSettings{
		model:    catalog.NormalizeServerModel(entry.ID, model),
		planMode: entry.SupportsPlanMode && planMode,
	}
}

func initialStatus(provider string) readmodels.AbolqasemStatus {
	if provider == "claude" {
		return readmodels.StatusRunning
	}
	return readmodels.StatusStarting
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefBool(value *bool) bool {
	return value != nil && *value
}

func eventError(event TurnEvent) error {
	if event.Error != nil {
		return event.Error
	}
	if event.Message != "" {
		return errors.New(event.Message)
	}
	return errors.New("provider failed")
}

type noopTurn struct{}

func (noopTurn) Cancel() error {
	return nil
}

func (noopTurn) RespondTool(context.Context, ToolResponse) error {
	return nil
}
