package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ai-agent-manager/internal/providers/catalog"
	"ai-agent-manager/internal/workspace/readmodels"
)

var (
	ErrChatAlreadyRunning = errors.New("chat is already running")
	ErrQueuedNotFound     = errors.New("queued message not found")
)

type Store interface {
	CreateChat(projectID string) (readmodels.ChatRecord, error)
	RequireChat(chatID string) (readmodels.ChatRecord, error)
	SetChatProvider(chatID string, provider string) error
	SetPlanMode(chatID string, planMode bool) error
	AppendUserPrompt(chatID string, content string, attachments []readmodels.ChatAttachment, steered bool) error
	RecordTurnStarted(chatID string) error
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
	Provider    string
	Model       string
	Effort      string
	ServiceTier string
	PlanMode    bool
	Status      readmodels.KannaStatus
	Turn        Turn
	StartedAt   int64
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
	ChatID      string
	Provider    string
	Content     string
	Attachments []readmodels.ChatAttachment
	Model       string
	Effort      string
	ServiceTier string
	PlanMode    bool
}

type SendResult struct {
	ChatID          string `json:"chatId"`
	Queued          bool   `json:"queued,omitempty"`
	QueuedMessageID string `json:"queuedMessageId,omitempty"`
}

func NewCoordinator(store Store, starter TurnStarter, onStateChange func(chatID string)) *Coordinator {
	if starter == nil {
		starter = TurnStarterFunc(func(context.Context, TurnRequest) (Turn, error) {
			return noopTurn{}, nil
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

func (c *Coordinator) ActiveStatuses() map[string]readmodels.KannaStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	statuses := map[string]readmodels.KannaStatus{}
	for chatID, turn := range c.active {
		statuses[chatID] = turn.Status
	}
	return statuses
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
	if active.Turn != nil {
		_ = active.Turn.Cancel()
	}
	if err := c.store.RecordTurnCancelled(chatID); err != nil {
		return err
	}
	c.emitStateChange(chatID)
	return nil
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

	c.mu.Lock()
	if c.active[chatID] != nil {
		c.mu.Unlock()
		return ErrChatAlreadyRunning
	}
	active := &ActiveTurn{
		ChatID:      chatID,
		Provider:    resolvedProvider,
		Model:       settings.model,
		Effort:      settings.effort,
		ServiceTier: settings.serviceTier,
		PlanMode:    settings.planMode,
		Status:      initialStatus(resolvedProvider),
		StartedAt:   time.Now().UnixMilli(),
	}
	c.active[chatID] = active
	c.mu.Unlock()

	if chat.Provider == nil {
		if err := c.store.SetChatProvider(chatID, resolvedProvider); err != nil {
			c.clearActive(chatID)
			return err
		}
	}
	if err := c.store.SetPlanMode(chatID, settings.planMode); err != nil {
		c.clearActive(chatID)
		return err
	}
	if err := c.store.AppendUserPrompt(chatID, content, attachments, steered); err != nil {
		c.clearActive(chatID)
		return err
	}
	if err := c.store.RecordTurnStarted(chatID); err != nil {
		c.clearActive(chatID)
		return err
	}

	turn, err := c.starter.StartTurn(ctx, TurnRequest{
		ChatID:      chatID,
		Provider:    resolvedProvider,
		Content:     content,
		Attachments: attachments,
		Model:       settings.model,
		Effort:      settings.effort,
		ServiceTier: settings.serviceTier,
		PlanMode:    settings.planMode,
	})
	if err != nil {
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
	c.emitStateChange(chatID)
	return nil
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

	options := catalog.NormalizeCodexModelOptions(modelOptions, legacyEffort)
	return resolvedProviderSettings{
		model:       catalog.NormalizeServerModel(entry.ID, model),
		effort:      options.ReasoningEffort,
		serviceTier: catalog.CodexServiceTierFromModelOptions(options),
		planMode:    entry.SupportsPlanMode && planMode,
	}
}

func initialStatus(provider string) readmodels.KannaStatus {
	if provider == "claude" {
		return readmodels.StatusRunning
	}
	return readmodels.StatusStarting
}

type noopTurn struct{}

func (noopTurn) Cancel() error {
	return nil
}
