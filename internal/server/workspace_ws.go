package server

import (
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/protocol"
	"ai-agent-manager/internal/workspace/terminal"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var workspaceWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "http://localhost:")
	},
}

var workspaceTerminals = newWorkspaceTerminalHub()

const keybindingsSubscription = "__keybindings__"

type workspaceConnection struct {
	conn *websocket.Conn
	hub  *workspaceTerminalHub

	writeMu       sync.Mutex
	subscriptions map[string]string
}

func handleWorkspaceWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	conn, err := workspaceWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	workspaceConn := &workspaceConnection{
		conn:          conn,
		hub:           workspaceTerminals,
		subscriptions: map[string]string{},
	}
	workspaceConnections.add(workspaceConn)
	defer workspaceConnections.remove(workspaceConn)
	defer workspaceConn.close()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		envelope, err := protocol.DecodeClientEnvelope(data)
		if err != nil {
			_ = workspaceConn.write(protocol.ErrorEnvelope("", err.Error()))
			continue
		}
		response := workspaceConn.handle(envelope)
		if response == nil {
			continue
		}
		if err := workspaceConn.write(*response); err != nil {
			return
		}
	}
}

func (c *workspaceConnection) handle(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	switch envelope.Type {
	case protocol.EnvelopeSubscribe:
		return c.handleSubscribe(envelope)
	case protocol.EnvelopeUnsubscribe:
		c.unsubscribe(envelope.ID)
		return nil
	case protocol.EnvelopeCommand:
		return c.handleCommand(envelope)
	default:
		response := protocol.ErrorEnvelope(envelope.ID, "unsupported envelope type")
		return &response
	}
}

func (c *workspaceConnection) handleSubscribe(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	if envelope.Topic == nil {
		response := protocol.ErrorEnvelope(envelope.ID, "missing topic")
		return &response
	}
	switch {
	case envelope.Topic.Type == protocol.TopicTerminal && envelope.Topic.TerminalID != "":
		c.subscribe(envelope.ID, terminalSubscription+envelope.Topic.TerminalID)
	case envelope.Topic.Type == protocol.TopicKeybindings:
		c.subscribe(envelope.ID, keybindingsSubscription)
	case envelope.Topic.Type == protocol.TopicSidebar:
		c.subscribe(envelope.ID, sidebarSubscription)
	case envelope.Topic.Type == protocol.TopicLocalProjects:
		c.subscribe(envelope.ID, localProjectsSubscription)
	case envelope.Topic.Type == protocol.TopicChat && envelope.Topic.ChatID != "":
		c.subscribe(envelope.ID, chatSubscription+envelope.Topic.ChatID)
	}
	snapshotType, data := workspaceSnapshotForTopic(*envelope.Topic)
	response := protocol.SnapshotEnvelope(envelope.ID, snapshotType, data)
	return &response
}

func (c *workspaceConnection) handleCommand(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	commandType, err := protocol.CommandType(envelope.Command)
	if err != nil {
		response := protocol.ErrorEnvelope(envelope.ID, err.Error())
		return &response
	}

	switch commandType {
	case protocol.CommandSystemPing:
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandBrowserListLocalHTTPServers:
		result, err := listWorkspaceLocalHTTPServers(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandBrowserKillLocalHTTPServer:
		if err := killWorkspaceLocalHTTPServer(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandProjectReadQuickActions:
		result, err := workspaceReadProjectQuickActions(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandProjectWriteQuickActions:
		result, err := workspaceWriteProjectQuickActions(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandSettingsReadAppSettings:
		response := protocol.AckEnvelope(envelope.ID, workspaceAppSettingsSnapshot())
		return &response
	case protocol.CommandSettingsReadKeybindings:
		snapshot, err := state.LoadKeybindingsSnapshot()
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandSettingsWriteKeybindings:
		snapshot, err := writeWorkspaceKeybindings(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitKeybindingsSnapshot(snapshot)
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandSettingsReadLLMProvider:
		snapshot, err := state.LoadLlmProviderSnapshot()
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandSettingsWriteLLMProvider:
		snapshot, err := writeWorkspaceLlmProvider(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandSettingsValidateLLMProvider:
		result, err := validateWorkspaceLlmProvider(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandSkillsSearch:
		result, err := workspaceSearchSkills(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandSkillsInstall:
		result, err := workspaceInstallSkill(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandSkillsUninstall:
		result, err := workspaceUninstallSkill(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandSkillsListInstalled:
		response := protocol.AckEnvelope(envelope.ID, listInstalledSkills(""))
		return &response
	case protocol.CommandSettingsWriteAppSettingsPatch:
		snapshot, err := applyWorkspaceAppSettingsPatch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandProjectOpen:
		var payload struct {
			LocalPath string `json:"localPath"`
			Title     string `json:"title"`
		}
		if err := json.Unmarshal(envelope.Command, &payload); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		project, err := workspaceOpenProject(payload.LocalPath, payload.Title)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast("")
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"projectId": project.ID})
		return &response
	case protocol.CommandChatCreate:
		var payload struct {
			ProjectID string `json:"projectId"`
		}
		if err := json.Unmarshal(envelope.Command, &payload); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		chat, err := workspaceCreateChat(payload.ProjectID)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chat.ID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"chatId": chat.ID})
		return &response
	case protocol.CommandChatSend:
		command, err := decodeSendCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		result, err := workspaceAgentCoordinator().Send(context.Background(), command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitWorkspaceSnapshots(result.ChatID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandMessageEnqueue:
		command, err := decodeQueueCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		queuedID, err := workspaceAgentCoordinator().Enqueue(command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitWorkspaceSnapshots(command.ChatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"queuedMessageId": queuedID})
		return &response
	case protocol.CommandMessageDequeue:
		chatID, queuedID, err := decodeQueuedMessageCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceAgentCoordinator().Dequeue(chatID, queuedID); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitWorkspaceSnapshots(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandMessageSteer:
		chatID, queuedID, err := decodeQueuedMessageCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceAgentCoordinator().Dequeue(chatID, queuedID); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitWorkspaceSnapshots(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatCancel:
		chatID, err := decodeChatID(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceAgentCoordinator().Cancel(chatID); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitWorkspaceSnapshots(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatRespondTool:
		command, err := decodeToolResponseCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceAgentCoordinator().RespondTool(context.Background(), command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		c.emitWorkspaceSnapshots(command.ChatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatMarkRead:
		chatID, err := decodeChatID(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceMarkChatRead(chatID); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandAppReadManagement:
		response := protocol.AckEnvelope(envelope.ID, workspaceManagementSnapshot())
		return &response
	case protocol.CommandAppWriteManagementSettings:
		snapshot, err := applyWorkspaceManagementPatch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandAppReloadSessions:
		report, err := runDiscovery()
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, "failed to reload sessions")
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"status": "ok", "report": report})
		return &response
	case protocol.CommandAppRestart:
		if err := scheduleServerRestart(); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"status": "restarting"})
		return &response
	case protocol.CommandAppReadHooksStatus:
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"items": workspaceHookStatuses()})
		return &response
	case protocol.CommandUpdateCheck:
		response := protocol.AckEnvelope(envelope.ID, workspaceCheckUpdate())
		return &response
	case protocol.CommandUpdateInstall:
		response := protocol.AckEnvelope(envelope.ID, workspaceInstallUpdate())
		return &response
	case protocol.CommandTerminalCreate:
		result, err := workspaceTerminals.create(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandTerminalInput:
		if err := workspaceTerminals.input(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandTerminalResize:
		if err := workspaceTerminals.resize(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandTerminalClose:
		if err := workspaceTerminals.close(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	default:
		response := protocol.ErrorEnvelope(envelope.ID, commandType+" is not implemented in the Go workspace backend yet")
		return &response
	}
}

func (c *workspaceConnection) write(envelope protocol.ServerEnvelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(envelope)
}

func (c *workspaceConnection) subscribe(subscriptionID string, terminalID string) {
	c.subscriptions[subscriptionID] = terminalID
	if terminalID == keybindingsSubscription {
		return
	}
	if strings.HasPrefix(terminalID, terminalSubscription) {
		c.hub.subscribe(strings.TrimPrefix(terminalID, terminalSubscription), subscriptionID, c)
	}
}

func (c *workspaceConnection) unsubscribe(subscriptionID string) {
	terminalID, ok := c.subscriptions[subscriptionID]
	if !ok {
		return
	}
	delete(c.subscriptions, subscriptionID)
	if terminalID == keybindingsSubscription {
		return
	}
	if strings.HasPrefix(terminalID, terminalSubscription) {
		c.hub.unsubscribe(strings.TrimPrefix(terminalID, terminalSubscription), subscriptionID, c)
	}
}

func (c *workspaceConnection) close() {
	for subscriptionID := range c.subscriptions {
		c.unsubscribe(subscriptionID)
	}
}

func (c *workspaceConnection) emitKeybindingsSnapshot(snapshot state.KeybindingsSnapshot) {
	for subscriptionID, topic := range c.subscriptions {
		if topic != keybindingsSubscription {
			continue
		}
		_ = c.write(protocol.SnapshotEnvelope(subscriptionID, protocol.SnapshotKeybindings, snapshot))
	}
}

func workspaceSnapshotForTopic(topic protocol.SubscriptionTopic) (string, any) {
	switch topic.Type {
	case protocol.TopicSidebar:
		return protocol.SnapshotSidebar, workspaceSidebarSnapshot()
	case protocol.TopicLocalProjects:
		return protocol.SnapshotLocalProjects, workspaceLocalProjectsSnapshot()
	case protocol.TopicUpdate:
		return protocol.SnapshotUpdate, workspaceUpdateSnapshot()
	case protocol.TopicKeybindings:
		snapshot, err := state.LoadKeybindingsSnapshot()
		if err != nil {
			return protocol.SnapshotKeybindings, map[string]any{
				"bindings":        state.DefaultKeybindings(),
				"warning":         err.Error(),
				"filePathDisplay": state.GetKeybindingsFilePath(),
			}
		}
		return protocol.SnapshotKeybindings, snapshot
	case protocol.TopicAppSettings:
		return protocol.SnapshotAppSettings, workspaceAppSettingsSnapshot()
	case protocol.TopicChat:
		return protocol.SnapshotChat, workspaceChatSnapshot(topic.ChatID, subscriptionRecentLimit(topic))
	case protocol.TopicProjectGit:
		return protocol.SnapshotProjectGit, nil
	case protocol.TopicTerminal:
		return protocol.SnapshotTerminal, workspaceTerminals.snapshot(topic.TerminalID)
	default:
		return topic.Type, nil
	}
}

type workspaceTerminalHub struct {
	manager *terminal.Manager

	mu          sync.Mutex
	subscribers map[string]map[string]*workspaceConnection
}

func newWorkspaceTerminalHub() *workspaceTerminalHub {
	hub := &workspaceTerminalHub{
		subscribers: map[string]map[string]*workspaceConnection{},
	}
	hub.manager = terminal.NewManager(hub.broadcast)
	return hub
}

func (h *workspaceTerminalHub) subscribe(terminalID string, subscriptionID string, conn *workspaceConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[terminalID] == nil {
		h.subscribers[terminalID] = map[string]*workspaceConnection{}
	}
	h.subscribers[terminalID][subscriptionID] = conn
}

func (h *workspaceTerminalHub) unsubscribe(terminalID string, subscriptionID string, conn *workspaceConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subscribers := h.subscribers[terminalID]
	if subscribers == nil {
		return
	}
	if subscribers[subscriptionID] == conn {
		delete(subscribers, subscriptionID)
	}
	if len(subscribers) == 0 {
		delete(h.subscribers, terminalID)
	}
}

func (h *workspaceTerminalHub) broadcast(event terminal.Event) {
	h.mu.Lock()
	subscribers := make(map[string]*workspaceConnection, len(h.subscribers[event.TerminalID]))
	for subscriptionID, conn := range h.subscribers[event.TerminalID] {
		subscribers[subscriptionID] = conn
	}
	h.mu.Unlock()
	for subscriptionID, conn := range subscribers {
		_ = conn.write(protocol.EventEnvelope(subscriptionID, event))
	}
}

func (h *workspaceTerminalHub) snapshot(terminalID string) *terminal.Snapshot {
	return h.manager.Snapshot(terminalID)
}

func (h *workspaceTerminalHub) create(raw json.RawMessage) (terminal.Snapshot, error) {
	var payload struct {
		ProjectID  string `json:"projectId"`
		TerminalID string `json:"terminalId"`
		Cols       int    `json:"cols"`
		Rows       int    `json:"rows"`
		Scrollback int    `json:"scrollback"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return terminal.Snapshot{}, err
	}
	return h.manager.Create(context.Background(), terminal.CreateRequest{
		ProjectID:  payload.ProjectID,
		TerminalID: payload.TerminalID,
		Cols:       payload.Cols,
		Rows:       payload.Rows,
		Scrollback: payload.Scrollback,
	})
}

func (h *workspaceTerminalHub) input(raw json.RawMessage) error {
	var payload struct {
		TerminalID string `json:"terminalId"`
		Data       string `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	return h.manager.Input(payload.TerminalID, payload.Data)
}

func (h *workspaceTerminalHub) resize(raw json.RawMessage) error {
	var payload struct {
		TerminalID string `json:"terminalId"`
		Cols       int    `json:"cols"`
		Rows       int    `json:"rows"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	return h.manager.Resize(payload.TerminalID, payload.Cols, payload.Rows)
}

func (h *workspaceTerminalHub) close(raw json.RawMessage) error {
	var payload struct {
		TerminalID string `json:"terminalId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	return h.manager.Close(payload.TerminalID)
}

func workspaceMachineName() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "Local Machine"
	}
	return name
}

func workspacePlatform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

func workspaceAppSettingsSnapshot() map[string]any {
	settings, _ := state.LoadSettings()
	settings = state.NormalizeSettings(settings)
	return map[string]any{
		"analyticsEnabled":        settings.AnalyticsEnabled,
		"browserSettingsMigrated": settings.BrowserSettingsMigrated,
		"locale":                  settings.Locale,
		"theme":                   settings.Theme,
		"chatSoundPreference":     settings.ChatSoundPreference,
		"chatSoundId":             settings.ChatSoundID,
		"terminal": map[string]any{
			"scrollbackLines": settings.Terminal.ScrollbackLines,
			"minColumnWidth":  settings.Terminal.MinColumnWidth,
		},
		"editor": map[string]any{
			"preset":          settings.Editor.Preset,
			"commandTemplate": settings.Editor.CommandTemplate,
		},
		"defaultProvider":  settings.DefaultProvider,
		"providerDefaults": providerDefaultsSnapshot(settings.ProviderDefaults),
		"management":       workspaceManagementSnapshot(),
		"warning":          nil,
		"filePathDisplay":  state.GetSettingsFilePath(),
	}
}

func providerDefaultsSnapshot(defaults map[string]state.ProviderPreference) map[string]any {
	out := map[string]any{}
	for provider, preference := range defaults {
		out[provider] = map[string]any{
			"model":        preference.Model,
			"modelOptions": preference.ModelOptions,
			"planMode":     preference.PlanMode,
		}
	}
	return out
}

func applyWorkspaceAppSettingsPatch(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		Patch state.AppSettingsPatch `json:"patch"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	settings, err := state.LoadSettings()
	if err != nil {
		return nil, err
	}
	settings = state.ApplySettingsPatch(settings, payload.Patch)
	if err := state.SaveSettings(settings); err != nil {
		return nil, err
	}
	return workspaceAppSettingsSnapshot(), nil
}

func writeWorkspaceKeybindings(raw json.RawMessage) (state.KeybindingsSnapshot, error) {
	var payload struct {
		Bindings map[string][]string `json:"bindings"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return state.KeybindingsSnapshot{}, err
	}
	return state.SaveKeybindings(payload.Bindings)
}

func writeWorkspaceLlmProvider(raw json.RawMessage) (state.LlmProviderSnapshot, error) {
	payload, err := decodeLlmProviderCommand(raw)
	if err != nil {
		return state.LlmProviderSnapshot{}, err
	}
	return state.SaveLlmProviderSnapshot(payload)
}

func validateWorkspaceLlmProvider(raw json.RawMessage) (state.LlmProviderValidationResult, error) {
	payload, err := decodeLlmProviderCommand(raw)
	if err != nil {
		return state.LlmProviderValidationResult{}, err
	}
	return state.ValidateLlmProviderCredentials(payload), nil
}

func decodeLlmProviderCommand(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		Provider string `json:"provider"`
		APIKey   string `json:"apiKey"`
		Model    string `json:"model"`
		BaseURL  string `json:"baseUrl"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return map[string]any{
		"provider": payload.Provider,
		"apiKey":   payload.APIKey,
		"model":    payload.Model,
		"baseUrl":  payload.BaseURL,
	}, nil
}

func handleWorkspaceAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"enabled":       false,
		"authenticated": true,
	})
}
