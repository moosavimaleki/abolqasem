package server

import (
	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
	"abolqasem/internal/workspace/protocol"
	"abolqasem/internal/workspace/terminal"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var workspaceWSUpgrader = websocket.Upgrader{
	CheckOrigin: workspaceWSOriginAllowed,
}

var workspaceTerminals = newWorkspaceTerminalHub()

const keybindingsSubscription = "__keybindings__"

func workspaceWSOriginAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	return isAllowedWorkspaceWSOrigin(r.Header.Get("Origin"))
}

func isAllowedWorkspaceWSOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil {
		return false
	}
	if parsed.Scheme != "http" {
		return false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "127.0.0.1", "localhost":
		return true
	default:
		return false
	}
}

type workspaceConnection struct {
	conn *websocket.Conn
	hub  *workspaceTerminalHub

	writeMu sync.Mutex
	writeFn func(protocol.ServerEnvelope) error

	subscriptionsMu sync.Mutex
	subscriptions   map[string]workspaceSubscription
}

type workspaceSubscription struct {
	key   string
	topic protocol.SubscriptionTopic
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
		subscriptions: map[string]workspaceSubscription{},
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
		if c.shouldHandleCommandAsync(envelope.Command) {
			c.handleCommandAsync(envelope)
			return nil
		}
		return c.handleCommand(envelope)
	default:
		response := protocol.ErrorEnvelope(envelope.ID, "unsupported envelope type")
		return &response
	}
}

func (c *workspaceConnection) shouldHandleCommandAsync(raw json.RawMessage) bool {
	commandType, err := protocol.CommandType(raw)
	if err != nil {
		return false
	}
	switch commandType {
	case protocol.CommandSkillsInstall, protocol.CommandSkillsUninstall, protocol.CommandMCPRegistryInstall, protocol.CommandChatRefreshDiffs:
		return true
	default:
		return false
	}
}

func (c *workspaceConnection) handleCommandAsync(envelope protocol.ClientEnvelope) {
	go func() {
		response := c.handleCommand(envelope)
		if response == nil {
			return
		}
		_ = c.write(*response)
	}()
}

func (c *workspaceConnection) handleSubscribe(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	if envelope.Topic == nil {
		response := protocol.ErrorEnvelope(envelope.ID, "missing topic")
		return &response
	}
	if key := workspaceSubscriptionKey(*envelope.Topic); key != "" {
		c.subscribe(envelope.ID, key, *envelope.Topic)
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
		workspaceConnections.broadcastKeybindings(snapshot)
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
	case protocol.CommandSkillsListOperations:
		response := protocol.AckEnvelope(envelope.ID, workspaceListSkillOperations())
		return &response
	case protocol.CommandMCPList:
		result, err := workspaceMCPList(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandMCPSave:
		result, err := workspaceMCPSave(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandMCPRemove:
		result, err := workspaceMCPRemove(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandMCPRegistrySearch:
		result, err := workspaceMCPRegistrySearch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandMCPRegistryInstall:
		result, err := workspaceMCPRegistryInstall(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandSystemOpenExternal:
		if err := workspaceOpenExternal(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandSettingsWriteAppSettingsPatch:
		snapshot, err := applyWorkspaceAppSettingsPatch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandSettingsRefreshProviderModels:
		if _, err := workspaceRefreshProviderModels(true); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		snapshot := workspaceAppSettingsSnapshot()
		workspaceConnections.broadcastAppSettings(snapshot)
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
	case protocol.CommandProjectCreate:
		project, err := workspaceCreateProject(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast("")
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"projectId": project.ID})
		return &response
	case protocol.CommandProjectRename:
		if err := workspaceRenameProject(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast("")
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandProjectRemove:
		if err := workspaceRemoveProject(envelope.Command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast("")
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandSidebarReorderProjectGroups:
		response := protocol.ErrorEnvelope(envelope.ID, protocol.CommandSidebarReorderProjectGroups+" is not implemented in the Go workspace backend yet")
		return &response
	case protocol.CommandProjectReadDiffPatch:
		result, err := workspaceReadDiffPatch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatCreate:
		var payload struct {
			ProjectID   string `json:"projectId"`
			Provider    string `json:"provider"`
			TmuxCommand string `json:"tmuxCommand"`
		}
		if err := json.Unmarshal(envelope.Command, &payload); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		chat, err := workspaceCreateChatWithOptions(payload.ProjectID, payload.Provider, payload.TmuxCommand)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chat.ID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"chatId": chat.ID})
		return &response
	case protocol.CommandChatFork:
		result, chatID, err := workspaceForkChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatConvertPreview:
		result, err := workspacePreviewConvertChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatConvert:
		result, chatID, err := workspaceConvertChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatExportTranscript:
		result, err := workspaceExportChatTranscript(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatMigrateToTmux:
		result, err := workspaceMigrateChatsToTmux(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		for _, chat := range result.Chats {
			workspaceConnections.broadcast(chat.ChatID)
		}
		if len(result.Chats) == 0 {
			workspaceConnections.broadcast("")
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatRename:
		chatID, err := workspaceRenameChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandChatArchive:
		chatID, err := workspaceArchiveChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandChatUnarchive:
		chatID, err := workspaceUnarchiveChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandChatDelete:
		chatID, err := workspaceDeleteChat(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandChatSetDraftProtection:
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
		return &response
	case protocol.CommandChatSend:
		command, err := decodeSendCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if _, ok := workspaceLegacySessionByChatID(command.ChatID); ok && !workspaceStoredChatExists(command.ChatID) {
			chatID, err := workspaceMaterializeLegacyChat(command.ChatID)
			if err != nil {
				response := protocol.ErrorEnvelope(envelope.ID, err.Error())
				return &response
			}
			command.ChatID = chatID
		}
		if result, handled, err := workspaceSendTmuxChat(command); handled {
			if err != nil {
				response := protocol.ErrorEnvelope(envelope.ID, err.Error())
				return &response
			}
			workspaceConnections.broadcast(result.ChatID)
			response := protocol.AckEnvelope(envelope.ID, result)
			return &response
		}
		result, err := workspaceAgentCoordinator().Send(context.Background(), command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(result.ChatID)
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
		workspaceConnections.broadcast(command.ChatID)
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
		workspaceConnections.broadcast(chatID)
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
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatCancel:
		chatID, err := decodeChatID(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if handled, err := workspaceCancelTmuxChat(chatID); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		} else if !handled {
			if err := workspaceAgentCoordinator().Cancel(chatID); err != nil {
				response := protocol.ErrorEnvelope(envelope.ID, err.Error())
				return &response
			}
		}
		workspaceConnections.broadcast(chatID)
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
		workspaceConnections.broadcast(command.ChatID)
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
	case protocol.CommandChatRefresh:
		chatID, err := decodeChatID(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatRestartTmux:
		chatID, err := decodeChatID(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceRestartTmuxChat(chatID); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatApplyRuntimePreferences:
		command, err := decodeRuntimePreferenceCommand(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		if err := workspaceApplyRuntimePreferences(command); err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(command.ChatID)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandChatRefreshDiffs:
		snapshot, projectID, changed, err := workspaceRefreshDiffs(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGitSnapshot(projectID, snapshot)
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"changed": changed})
		return &response
	case protocol.CommandChatInitGit:
		result, projectID, err := workspaceInitGit(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatGetGitHubPublishInfo:
		result, err := workspaceGetGitHubPublishInfo(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatCheckGitHubRepoAvailability:
		result, err := workspaceCheckGitHubRepoAvailability(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatPublishToGitHub:
		result, projectID, err := workspacePublishToGitHub(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatListBranches:
		result, err := workspaceListBranches(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatPreviewMergeBranch:
		result, err := workspacePreviewMergeBranch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatMergeBranch:
		result, projectID, err := workspaceMergeBranch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatSyncBranch:
		result, projectID, err := workspaceSyncBranch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatCheckoutBranch:
		result, projectID, err := workspaceCheckoutBranch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatCreateBranch:
		result, projectID, err := workspaceCreateBranch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatGenerateCommitMessage:
		result, err := workspaceGenerateCommitMessage(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatCommitDiffs:
		result, projectID, err := workspaceCommitDiffs(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatDiscardDiffFile:
		result, projectID, err := workspaceDiscardDiffFile(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatIgnoreDiffFile:
		result, projectID, err := workspaceIgnoreDiffFile(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatListCheckpoints:
		result, err := workspaceListCheckpoints(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatRestoreCheckpoint:
		result, projectID, err := workspaceRestoreCheckpoint(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceConnections.broadcast(result.Checkpoint.ChatID)
		workspaceConnections.broadcastProjectGit(projectID)
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatStopDraining:
		chatID, err := decodeChatID(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		workspaceAgentCoordinator().StopDraining(chatID)
		workspaceConnections.broadcast(chatID)
		response := protocol.AckEnvelope(envelope.ID, workspaceAck())
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
		workspaceConnections.broadcastAppSettings(workspaceAppSettingsSnapshot())
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandAppReloadSessions:
		report, err := runDiscovery()
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, "failed to reload sessions")
			return &response
		}
		workspaceConnections.broadcast("")
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
		snapshot := workspaceCheckUpdate()
		workspaceConnections.broadcastUpdate(snapshot)
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	case protocol.CommandUpdateInstall:
		result := workspaceInstallUpdate()
		workspaceConnections.broadcastUpdate(workspaceUpdateSnapshot())
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatReadTranscriptIndex:
		result, err := workspaceReadChatTranscriptIndex(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatLoadHistory:
		result, err := workspaceLoadChatHistory(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
		return &response
	case protocol.CommandChatLoadHistoryAround:
		result, err := workspaceLoadChatHistoryAround(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, result)
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
	if c.writeFn != nil {
		return c.writeFn(envelope)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(envelope)
}

func (c *workspaceConnection) subscribe(subscriptionID string, key string, topic protocol.SubscriptionTopic) {
	c.subscriptionsMu.Lock()
	previous, replacing := c.subscriptions[subscriptionID]
	c.subscriptions[subscriptionID] = workspaceSubscription{key: key, topic: topic}
	c.subscriptionsMu.Unlock()
	if replacing {
		c.unregisterSubscription(subscriptionID, previous.key)
	}
	if key == "" {
		return
	}
	c.registerSubscription(subscriptionID, key)
}

func (c *workspaceConnection) registerSubscription(subscriptionID string, key string) {
	if strings.HasPrefix(key, terminalSubscription) {
		c.hub.subscribe(strings.TrimPrefix(key, terminalSubscription), subscriptionID, c)
		return
	}
	workspaceConnections.subscribe(key, subscriptionID, c)
}

func (c *workspaceConnection) unsubscribe(subscriptionID string) {
	c.subscriptionsMu.Lock()
	subscription, ok := c.subscriptions[subscriptionID]
	if !ok {
		c.subscriptionsMu.Unlock()
		return
	}
	delete(c.subscriptions, subscriptionID)
	c.subscriptionsMu.Unlock()
	if subscription.key == "" {
		return
	}
	c.unregisterSubscription(subscriptionID, subscription.key)
}

func (c *workspaceConnection) unregisterSubscription(subscriptionID string, key string) {
	if strings.HasPrefix(key, terminalSubscription) {
		c.hub.unsubscribe(strings.TrimPrefix(key, terminalSubscription), subscriptionID, c)
		return
	}
	workspaceConnections.unsubscribe(key, subscriptionID, c)
}

func (c *workspaceConnection) close() {
	subscriptionIDs := c.subscriptionIDs()
	for _, subscriptionID := range subscriptionIDs {
		c.unsubscribe(subscriptionID)
	}
}

func (c *workspaceConnection) subscriptionIDs() []string {
	c.subscriptionsMu.Lock()
	defer c.subscriptionsMu.Unlock()
	ids := make([]string, 0, len(c.subscriptions))
	for subscriptionID := range c.subscriptions {
		ids = append(ids, subscriptionID)
	}
	return ids
}

func (c *workspaceConnection) subscription(subscriptionID string) (workspaceSubscription, bool) {
	c.subscriptionsMu.Lock()
	defer c.subscriptionsMu.Unlock()
	subscription, ok := c.subscriptions[subscriptionID]
	return subscription, ok
}

func workspaceSubscriptionKey(topic protocol.SubscriptionTopic) string {
	switch topic.Type {
	case protocol.TopicTerminal:
		if topic.TerminalID == "" {
			return ""
		}
		return terminalSubscription + topic.TerminalID
	case protocol.TopicKeybindings:
		return keybindingsSubscription
	case protocol.TopicSidebar:
		return sidebarSubscription
	case protocol.TopicLocalProjects:
		return localProjectsSubscription
	case protocol.TopicUpdate:
		return updateSubscription
	case protocol.TopicAppSettings:
		return appSettingsSubscription
	case protocol.TopicChat:
		if topic.ChatID == "" {
			return ""
		}
		return chatSubscription + topic.ChatID
	case protocol.TopicProjectGit:
		if topic.ProjectID == "" {
			return ""
		}
		return projectGitSubscription + topic.ProjectID
	default:
		return ""
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
		return protocol.SnapshotProjectGit, workspaceProjectGitSubscriptionSnapshot(topic.ProjectID)
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

func (h *workspaceTerminalHub) rootPIDsByCWD(cwd string) []int {
	return h.manager.RootPIDsByCWD(cwd)
}

func (h *workspaceTerminalHub) create(raw json.RawMessage) (terminal.Snapshot, error) {
	request, err := workspaceTerminalCreateRequest(raw)
	if err != nil {
		return terminal.Snapshot{}, err
	}
	return h.manager.Create(context.Background(), request)
}

func workspaceTerminalCreateRequest(raw json.RawMessage) (terminal.CreateRequest, error) {
	var payload struct {
		ProjectID   string `json:"projectId"`
		TerminalID  string `json:"terminalId"`
		Mode        string `json:"mode"`
		ChatID      string `json:"chatId"`
		TmuxSession string `json:"tmuxSession"`
		Command     string `json:"command"`
		Cols        int    `json:"cols"`
		Rows        int    `json:"rows"`
		Scrollback  int    `json:"scrollback"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return terminal.CreateRequest{}, err
	}
	projectPath, err := workspaceProjectLocalPathRequired(payload.ProjectID)
	if err != nil {
		return terminal.CreateRequest{}, err
	}
	mode := strings.TrimSpace(payload.Mode)
	tmuxSession := strings.TrimSpace(payload.TmuxSession)
	command := strings.TrimSpace(payload.Command)
	if mode == "tmux" && tmuxSession == "" {
		tmuxSession = workspaceChatTmuxSession(payload.ChatID)
	}
	if mode == "tmux" && strings.TrimSpace(payload.ChatID) != "" {
		if chat, _, err := workspaceChatProjectRequired(payload.ChatID); err == nil {
			if strings.TrimSpace(chat.TmuxSession) != "" {
				tmuxSession = chat.TmuxSession
			}
			if command == "" {
				command = workspaceTmuxCommandForChat(chat, "")
			}
		}
	}
	return terminal.CreateRequest{
		ProjectID:   payload.ProjectID,
		TerminalID:  payload.TerminalID,
		CWD:         projectPath,
		Mode:        mode,
		ChatID:      payload.ChatID,
		TmuxSession: tmuxSession,
		Command:     command,
		Cols:        payload.Cols,
		Rows:        payload.Rows,
		Scrollback:  payload.Scrollback,
	}, nil
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

func workspaceAppSettingsSnapshot() map[string]any {
	settings, err := workspaceRefreshProviderModels(false)
	if err != nil {
		settings, _ = state.LoadSettings()
	}
	settings = state.NormalizeSettings(settings)
	providerexec.SetConfiguredExecutables(settings.ProviderExecutables)
	return map[string]any{
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
		"providerProxy": map[string]any{
			"mode":      settings.ProviderProxy.Mode,
			"httpProxy": settings.ProviderProxy.HTTPProxy,
			"noProxy":   settings.ProviderProxy.NoProxy,
		},
		"providerExecutables":  providerExecutableSnapshot(settings.ProviderExecutables),
		"tmuxCommands":         settings.TmuxCommands,
		"defaultProvider":      settings.DefaultProvider,
		"providerDefaults":     providerDefaultsSnapshot(settings.ProviderDefaults),
		"providerModelCatalog": providerModelCatalogSnapshot(settings.ProviderModelCatalog),
		"commitMessageGenerator": map[string]any{
			"provider": settings.CommitMessageGenerator.Provider,
			"model":    settings.CommitMessageGenerator.Model,
		},
		"availableProviders": workspaceAvailableProvidersForSettings(settings),
		"management":         workspaceManagementSnapshot(),
		"warning":            nil,
		"filePathDisplay":    state.GetSettingsFilePath(),
	}
}

func providerExecutableSnapshot(configured map[string]string) map[string]string {
	out := map[string]string{}
	for _, provider := range []string{"claude", "codex", "gemini"} {
		if executable := strings.TrimSpace(configured[provider]); executable != "" {
			out[provider] = executable
			continue
		}
		if executable := providerexec.DetectExecutable(provider); executable != "" {
			out[provider] = executable
		}
	}
	return out
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

func handleWorkspaceAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
