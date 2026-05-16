package server

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ai-agent-manager/internal/providers/catalog"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/agent"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/eventstore"
	"ai-agent-manager/internal/workspace/protocol"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

const (
	sidebarSubscription       = "__sidebar__"
	localProjectsSubscription = "__local_projects__"
	updateSubscription        = "__update__"
	appSettingsSubscription   = "__app_settings__"
	terminalSubscription      = "terminal:"
	chatSubscription          = "chat:"
	projectGitSubscription    = "project_git:"
)

var (
	workspaceCoordinatorMu  sync.Mutex
	workspaceCoordinator    *agent.Coordinator
	workspaceCoordinatorDir string
	workspaceConnections    = newWorkspaceConnectionRegistry()
)

type workspaceEventStore struct {
	store *eventstore.Store
}

type workspaceConnectionRegistry struct {
	mu          sync.Mutex
	connections map[*workspaceConnection]struct{}
	subscribers map[string]map[*workspaceConnection]map[string]struct{}
}

func newWorkspaceConnectionRegistry() *workspaceConnectionRegistry {
	return &workspaceConnectionRegistry{
		connections: map[*workspaceConnection]struct{}{},
		subscribers: map[string]map[*workspaceConnection]map[string]struct{}{},
	}
}

func (r *workspaceConnectionRegistry) add(conn *workspaceConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[conn] = struct{}{}
}

func (r *workspaceConnectionRegistry) remove(conn *workspaceConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, conn)
	for topicKey, topicSubscribers := range r.subscribers {
		delete(topicSubscribers, conn)
		if len(topicSubscribers) == 0 {
			delete(r.subscribers, topicKey)
		}
	}
}

func (r *workspaceConnectionRegistry) subscribe(topicKey string, subscriptionID string, conn *workspaceConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subscribers[topicKey] == nil {
		r.subscribers[topicKey] = map[*workspaceConnection]map[string]struct{}{}
	}
	if r.subscribers[topicKey][conn] == nil {
		r.subscribers[topicKey][conn] = map[string]struct{}{}
	}
	r.subscribers[topicKey][conn][subscriptionID] = struct{}{}
}

func (r *workspaceConnectionRegistry) unsubscribe(topicKey string, subscriptionID string, conn *workspaceConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	topicSubscribers := r.subscribers[topicKey]
	if topicSubscribers == nil {
		return
	}
	subscriptionIDs := topicSubscribers[conn]
	if subscriptionIDs == nil {
		return
	}
	delete(subscriptionIDs, subscriptionID)
	if len(subscriptionIDs) == 0 {
		delete(topicSubscribers, conn)
	}
	if len(topicSubscribers) == 0 {
		delete(r.subscribers, topicKey)
	}
}

func (r *workspaceConnectionRegistry) topicSubscribers(topicKey string) map[*workspaceConnection][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	topicSubscribers := r.subscribers[topicKey]
	out := make(map[*workspaceConnection][]string, len(topicSubscribers))
	for conn, subscriptionIDs := range topicSubscribers {
		for subscriptionID := range subscriptionIDs {
			out[conn] = append(out[conn], subscriptionID)
		}
	}
	return out
}

func (r *workspaceConnectionRegistry) broadcast(chatID string) {
	r.broadcastTopic(sidebarSubscription, protocol.SnapshotSidebar, workspaceSidebarSnapshot())
	r.broadcastTopic(localProjectsSubscription, protocol.SnapshotLocalProjects, workspaceLocalProjectsSnapshot())
	if chatID != "" {
		r.broadcastChat(chatID)
	}
}

func (r *workspaceConnectionRegistry) broadcastTopic(topicKey string, snapshotType string, data any) {
	for conn, subscriptionIDs := range r.topicSubscribers(topicKey) {
		for _, subscriptionID := range subscriptionIDs {
			_ = conn.write(protocol.SnapshotEnvelope(subscriptionID, snapshotType, data))
		}
	}
}

func (r *workspaceConnectionRegistry) broadcastChat(chatID string) {
	for conn, subscriptionIDs := range r.topicSubscribers(chatSubscription + chatID) {
		for _, subscriptionID := range subscriptionIDs {
			subscription, ok := conn.subscription(subscriptionID)
			if !ok {
				continue
			}
			_ = conn.write(protocol.SnapshotEnvelope(
				subscriptionID,
				protocol.SnapshotChat,
				workspaceChatSnapshot(chatID, subscriptionRecentLimit(subscription.topic)),
			))
		}
	}
}

func (r *workspaceConnectionRegistry) broadcastKeybindings(snapshot state.KeybindingsSnapshot) {
	r.broadcastTopic(keybindingsSubscription, protocol.SnapshotKeybindings, snapshot)
}

func (r *workspaceConnectionRegistry) broadcastUpdate(snapshot map[string]any) {
	r.broadcastTopic(updateSubscription, protocol.SnapshotUpdate, snapshot)
}

func (r *workspaceConnectionRegistry) broadcastAppSettings(snapshot map[string]any) {
	r.broadcastTopic(appSettingsSubscription, protocol.SnapshotAppSettings, snapshot)
}

func workspaceAgentCoordinator() *agent.Coordinator {
	dir := workspaceDataDir()
	workspaceCoordinatorMu.Lock()
	defer workspaceCoordinatorMu.Unlock()
	if workspaceCoordinator != nil && workspaceCoordinatorDir == dir {
		return workspaceCoordinator
	}
	workspaceCoordinatorDir = dir
	workspaceCoordinator = agent.NewCoordinator(&workspaceEventStore{store: eventstore.New(dir)}, nil, func(chatID string) {
		workspaceConnections.broadcast(chatID)
	})
	return workspaceCoordinator
}

func (s *workspaceEventStore) CreateChat(projectID string) (readmodels.ChatRecord, error) {
	project, err := s.requireProject(projectID)
	if err != nil {
		return readmodels.ChatRecord{}, err
	}
	now := time.Now().UnixMilli()
	chatID := "chat-" + randomID()
	title := "New Chat"
	event, err := events.NewAt(events.TypeChatCreated, now, map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     title,
	})
	if err != nil {
		return readmodels.ChatRecord{}, err
	}
	if err := s.store.Append(events.StreamChats, event); err != nil {
		return readmodels.ChatRecord{}, err
	}
	return readmodels.ChatRecord{
		ID:        chatID,
		ProjectID: project.ID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *workspaceEventStore) RequireChat(chatID string) (readmodels.ChatRecord, error) {
	state, err := s.store.LoadState()
	if err != nil {
		return readmodels.ChatRecord{}, err
	}
	chat, ok := state.ChatsByID[chatID]
	if !ok || chat.DeletedAt != 0 {
		return readmodels.ChatRecord{}, errors.New("chat not found")
	}
	return chat, nil
}

func (s *workspaceEventStore) SetChatProvider(chatID string, provider string) error {
	event, err := events.New(events.TypeChatProviderSet, map[string]any{"chatId": chatID, "provider": provider})
	if err != nil {
		return err
	}
	return s.store.Append(events.StreamChats, event)
}

func (s *workspaceEventStore) SetPlanMode(chatID string, planMode bool) error {
	event, err := events.New(events.TypeChatPlanModeSet, map[string]any{"chatId": chatID, "planMode": planMode})
	if err != nil {
		return err
	}
	return s.store.Append(events.StreamChats, event)
}

func (s *workspaceEventStore) AppendUserPrompt(chatID string, content string, attachments []readmodels.ChatAttachment, steered bool) error {
	entry := transcript.New(transcript.KindUserPrompt, map[string]any{
		"content":     content,
		"attachments": attachments,
		"steered":     steered,
	})
	return s.AppendTranscriptEntry(chatID, entry)
}

func (s *workspaceEventStore) AppendTranscriptEntry(chatID string, entry readmodels.TranscriptEntry) error {
	event, err := events.New(events.TypeMessageAppended, map[string]any{"chatId": chatID, "entry": entry})
	if err != nil {
		return err
	}
	return s.store.Append(events.StreamMessages, event)
}

func (s *workspaceEventStore) RecordToolCall(chatID string, request agent.PendingToolRequest) error {
	toolName := request.ToolName
	if toolName == "" {
		toolName = workspaceToolName(request.ToolKind)
	}
	input := request.Input
	if input == nil {
		input = workspaceToolDefaultInput(request.ToolKind)
	}
	return s.AppendTranscriptEntry(chatID, transcript.New(transcript.KindToolCall, map[string]any{
		"tool": map[string]any{
			"kind":     "tool",
			"toolKind": request.ToolKind,
			"toolName": toolName,
			"toolId":   request.ToolUseID,
			"input":    input,
		},
	}))
}

func (s *workspaceEventStore) RecordToolResult(chatID string, toolUseID string, result any) error {
	return s.AppendTranscriptEntry(chatID, transcript.New(transcript.KindToolResult, map[string]any{
		"toolId":   toolUseID,
		"content":  result,
		"debugRaw": mustJSONString(map[string]any{"tool_use_result": result}),
	}))
}

func (s *workspaceEventStore) RecordTurnStarted(chatID string) error {
	return s.appendTurn(events.TypeTurnStarted, chatID, nil)
}

func (s *workspaceEventStore) RecordTurnFinished(chatID string) error {
	return s.appendTurn(events.TypeTurnFinished, chatID, nil)
}

func (s *workspaceEventStore) RecordTurnFailed(chatID string, message string) error {
	return s.appendTurn(events.TypeTurnFailed, chatID, map[string]any{"message": message})
}

func (s *workspaceEventStore) RecordTurnCancelled(chatID string) error {
	return s.appendTurn(events.TypeTurnCancelled, chatID, nil)
}

func (s *workspaceEventStore) EnqueueMessage(chatID string, message agent.QueueMessageInput) (readmodels.QueuedChatMessage, error) {
	now := time.Now().UnixMilli()
	queued := readmodels.QueuedChatMessage{
		ID:           "queued-" + randomID(),
		Content:      message.Content,
		Attachments:  append([]readmodels.ChatAttachment(nil), message.Attachments...),
		CreatedAt:    now,
		Model:        message.Model,
		ModelOptions: message.ModelOptions,
	}
	if message.Provider != "" {
		queued.Provider = &message.Provider
	}
	queued.PlanMode = &message.PlanMode
	event, err := events.NewAt(events.TypeQueuedMessageEnqueued, now, map[string]any{"chatId": chatID, "message": queued})
	if err != nil {
		return readmodels.QueuedChatMessage{}, err
	}
	if err := s.store.Append(events.StreamQueuedMessages, event); err != nil {
		return readmodels.QueuedChatMessage{}, err
	}
	return queued, nil
}

func (s *workspaceEventStore) GetQueuedMessages(chatID string) []readmodels.QueuedChatMessage {
	state, err := s.store.LoadState()
	if err != nil {
		return nil
	}
	return append([]readmodels.QueuedChatMessage(nil), state.QueuedMessagesByChatID[chatID]...)
}

func (s *workspaceEventStore) GetQueuedMessage(chatID string, queuedMessageID string) (readmodels.QueuedChatMessage, bool) {
	for _, message := range s.GetQueuedMessages(chatID) {
		if message.ID == queuedMessageID {
			return message, true
		}
	}
	return readmodels.QueuedChatMessage{}, false
}

func (s *workspaceEventStore) RemoveQueuedMessage(chatID string, queuedMessageID string) error {
	event, err := events.New(events.TypeQueuedMessageRemoved, map[string]any{"chatId": chatID, "queuedMessageId": queuedMessageID})
	if err != nil {
		return err
	}
	return s.store.Append(events.StreamQueuedMessages, event)
}

func (s *workspaceEventStore) appendTurn(eventType string, chatID string, extra map[string]any) error {
	data := map[string]any{"chatId": chatID}
	for key, value := range extra {
		data[key] = value
	}
	event, err := events.New(eventType, data)
	if err != nil {
		return err
	}
	return s.store.Append(events.StreamTurns, event)
}

func (s *workspaceEventStore) requireProject(projectID string) (readmodels.ProjectRecord, error) {
	state, err := s.store.LoadState()
	if err != nil {
		return readmodels.ProjectRecord{}, err
	}
	project, ok := state.ProjectsByID[projectID]
	if !ok || project.DeletedAt != 0 {
		return readmodels.ProjectRecord{}, errors.New("project not found")
	}
	return project, nil
}

func workspaceOpenProject(localPath string, title string) (readmodels.ProjectRecord, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return readmodels.ProjectRecord{}, errors.New("localPath is required")
	}
	if title = strings.TrimSpace(title); title == "" {
		title = filepath.Base(localPath)
	}
	store := workspaceStore()
	state, err := store.LoadState()
	if err != nil {
		return readmodels.ProjectRecord{}, err
	}
	if projectID := state.ProjectIDsByPath[localPath]; projectID != "" {
		if project, ok := state.ProjectsByID[projectID]; ok && project.DeletedAt == 0 {
			return project, nil
		}
	}
	now := time.Now().UnixMilli()
	project := readmodels.ProjectRecord{
		ID:        "project-" + randomID(),
		LocalPath: localPath,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	event, err := events.NewAt(events.TypeProjectOpened, now, map[string]any{
		"projectId": project.ID,
		"localPath": project.LocalPath,
		"title":     project.Title,
	})
	if err != nil {
		return readmodels.ProjectRecord{}, err
	}
	if err := store.Append(events.StreamProjects, event); err != nil {
		return readmodels.ProjectRecord{}, err
	}
	return project, nil
}

func workspaceCreateChat(projectID string) (readmodels.ChatRecord, error) {
	return (&workspaceEventStore{store: workspaceStore()}).CreateChat(projectID)
}

func workspaceMarkChatRead(chatID string) error {
	event, err := events.New(events.TypeChatReadStateSet, map[string]any{"chatId": chatID, "unread": false})
	if err != nil {
		return err
	}
	return workspaceStore().Append(events.StreamChats, event)
}

func workspaceAppendAssistantText(chatID string, text string) error {
	if strings.TrimSpace(chatID) == "" {
		return errors.New("chatId is required")
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := (&workspaceEventStore{store: workspaceStore()}).AppendTranscriptEntry(chatID, transcript.New(transcript.KindAssistantText, map[string]any{
		"text": text,
	})); err != nil {
		return err
	}
	workspaceConnections.broadcast(chatID)
	return nil
}

func (c *workspaceConnection) emitWorkspaceSnapshots(chatID string) {
	c.subscriptionsMu.Lock()
	subscriptions := make(map[string]workspaceSubscription, len(c.subscriptions))
	for subscriptionID, subscription := range c.subscriptions {
		subscriptions[subscriptionID] = subscription
	}
	c.subscriptionsMu.Unlock()

	for subscriptionID, subscription := range subscriptions {
		topic := subscription.key
		switch {
		case topic == sidebarSubscription:
			_ = c.write(protocol.SnapshotEnvelope(subscriptionID, protocol.SnapshotSidebar, workspaceSidebarSnapshot()))
		case topic == localProjectsSubscription:
			_ = c.write(protocol.SnapshotEnvelope(subscriptionID, protocol.SnapshotLocalProjects, workspaceLocalProjectsSnapshot()))
		case chatID != "" && topic == chatSubscription+chatID:
			_ = c.write(protocol.SnapshotEnvelope(subscriptionID, protocol.SnapshotChat, workspaceChatSnapshot(chatID, subscriptionRecentLimit(subscription.topic))))
		}
	}
}

func decodeSendCommand(raw json.RawMessage) (agent.SendCommand, error) {
	var payload struct {
		ChatID       string                      `json:"chatId"`
		ProjectID    string                      `json:"projectId"`
		Content      string                      `json:"content"`
		Attachments  []readmodels.ChatAttachment `json:"attachments"`
		Provider     string                      `json:"provider"`
		Model        string                      `json:"model"`
		ModelOptions *catalog.ModelOptions       `json:"modelOptions"`
		Effort       string                      `json:"effort"`
		PlanMode     bool                        `json:"planMode"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return agent.SendCommand{}, err
	}
	return agent.SendCommand{
		ChatID:       payload.ChatID,
		ProjectID:    payload.ProjectID,
		Content:      payload.Content,
		Attachments:  payload.Attachments,
		Provider:     payload.Provider,
		Model:        payload.Model,
		ModelOptions: payload.ModelOptions,
		Effort:       payload.Effort,
		PlanMode:     payload.PlanMode,
	}, nil
}

func decodeQueueCommand(raw json.RawMessage) (agent.SendCommand, error) {
	return decodeSendCommand(raw)
}

func decodeChatID(raw json.RawMessage) (string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.ChatID) == "" {
		return "", errors.New("chatId is required")
	}
	return payload.ChatID, nil
}

func decodeQueuedMessageCommand(raw json.RawMessage) (string, string, error) {
	var payload struct {
		ChatID          string `json:"chatId"`
		QueuedMessageID string `json:"queuedMessageId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(payload.ChatID) == "" {
		return "", "", errors.New("chatId is required")
	}
	if strings.TrimSpace(payload.QueuedMessageID) == "" {
		return "", "", errors.New("queuedMessageId is required")
	}
	return payload.ChatID, payload.QueuedMessageID, nil
}

func decodeToolResponseCommand(raw json.RawMessage) (agent.ToolResponseCommand, error) {
	var payload struct {
		ChatID    string `json:"chatId"`
		ToolUseID string `json:"toolUseId"`
		Result    any    `json:"result"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return agent.ToolResponseCommand{}, err
	}
	if strings.TrimSpace(payload.ChatID) == "" {
		return agent.ToolResponseCommand{}, errors.New("chatId is required")
	}
	if strings.TrimSpace(payload.ToolUseID) == "" {
		return agent.ToolResponseCommand{}, errors.New("toolUseId is required")
	}
	return agent.ToolResponseCommand{
		ChatID:    payload.ChatID,
		ToolUseID: payload.ToolUseID,
		Result:    payload.Result,
	}, nil
}

func workspaceToolName(toolKind string) string {
	switch toolKind {
	case "ask_user_question":
		return "AskUserQuestion"
	case "exit_plan_mode":
		return "ExitPlanMode"
	default:
		return toolKind
	}
}

func workspaceToolDefaultInput(toolKind string) any {
	switch toolKind {
	case "ask_user_question":
		return map[string]any{"questions": []any{}}
	default:
		return map[string]any{}
	}
}

func mustJSONString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
