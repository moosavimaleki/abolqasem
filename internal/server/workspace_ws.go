package server

import (
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/protocol"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"

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

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		envelope, err := protocol.DecodeClientEnvelope(data)
		if err != nil {
			_ = writeWorkspaceEnvelope(conn, protocol.ErrorEnvelope("", err.Error()))
			continue
		}
		response := handleWorkspaceEnvelope(envelope)
		if response == nil {
			continue
		}
		if err := writeWorkspaceEnvelope(conn, *response); err != nil {
			return
		}
	}
}

func handleWorkspaceEnvelope(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	switch envelope.Type {
	case protocol.EnvelopeSubscribe:
		return handleWorkspaceSubscribe(envelope)
	case protocol.EnvelopeUnsubscribe:
		return nil
	case protocol.EnvelopeCommand:
		return handleWorkspaceCommand(envelope)
	default:
		response := protocol.ErrorEnvelope(envelope.ID, "unsupported envelope type")
		return &response
	}
}

func handleWorkspaceSubscribe(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	if envelope.Topic == nil {
		response := protocol.ErrorEnvelope(envelope.ID, "missing topic")
		return &response
	}
	snapshotType, data := workspaceSnapshotForTopic(*envelope.Topic)
	response := protocol.SnapshotEnvelope(envelope.ID, snapshotType, data)
	return &response
}

func handleWorkspaceCommand(envelope protocol.ClientEnvelope) *protocol.ServerEnvelope {
	commandType, err := protocol.CommandType(envelope.Command)
	if err != nil {
		response := protocol.ErrorEnvelope(envelope.ID, err.Error())
		return &response
	}

	switch commandType {
	case protocol.CommandSystemPing:
		response := protocol.AckEnvelope(envelope.ID, map[string]any{"ok": true})
		return &response
	case protocol.CommandSettingsReadAppSettings:
		response := protocol.AckEnvelope(envelope.ID, workspaceAppSettingsSnapshot())
		return &response
	case protocol.CommandSettingsWriteAppSettingsPatch:
		snapshot, err := applyWorkspaceAppSettingsPatch(envelope.Command)
		if err != nil {
			response := protocol.ErrorEnvelope(envelope.ID, err.Error())
			return &response
		}
		response := protocol.AckEnvelope(envelope.ID, snapshot)
		return &response
	default:
		response := protocol.ErrorEnvelope(envelope.ID, commandType+" is not implemented in the Go workspace backend yet")
		return &response
	}
}

func writeWorkspaceEnvelope(conn *websocket.Conn, envelope protocol.ServerEnvelope) error {
	return conn.WriteJSON(envelope)
}

func workspaceSnapshotForTopic(topic protocol.SubscriptionTopic) (string, any) {
	switch topic.Type {
	case protocol.TopicSidebar:
		return protocol.SnapshotSidebar, map[string]any{"projectGroups": []any{}}
	case protocol.TopicLocalProjects:
		return protocol.SnapshotLocalProjects, map[string]any{
			"machine": map[string]any{
				"id":          "local",
				"displayName": workspaceMachineName(),
				"platform":    workspacePlatform(),
			},
			"projects": []any{},
		}
	case protocol.TopicUpdate:
		return protocol.SnapshotUpdate, workspaceUpdateSnapshot()
	case protocol.TopicKeybindings:
		return protocol.SnapshotKeybindings, workspaceKeybindingsSnapshot()
	case protocol.TopicAppSettings:
		return protocol.SnapshotAppSettings, workspaceAppSettingsSnapshot()
	case protocol.TopicChat:
		return protocol.SnapshotChat, nil
	case protocol.TopicProjectGit:
		return protocol.SnapshotProjectGit, nil
	case protocol.TopicTerminal:
		return protocol.SnapshotTerminal, nil
	default:
		return topic.Type, nil
	}
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

func workspaceUpdateSnapshot() map[string]any {
	return map[string]any{
		"currentVersion":    "0.1.0",
		"latestVersion":     nil,
		"status":            "idle",
		"updateAvailable":   false,
		"lastCheckedAt":     nil,
		"error":             nil,
		"installAction":     "restart",
		"reloadRequestedAt": nil,
	}
}

func workspaceKeybindingsSnapshot() map[string]any {
	return map[string]any{
		"bindings": map[string][]string{
			"toggleEmbeddedTerminal":     {"cmd+j", "ctrl+`"},
			"toggleRightSidebar":         {"cmd+b", "ctrl+b"},
			"openInFinder":               {"cmd+alt+f", "ctrl+alt+f"},
			"openInEditor":               {"cmd+shift+o", "ctrl+shift+o"},
			"addSplitTerminal":           {"cmd+/", "ctrl+/"},
			"jumpToSidebarChat":          {"cmd+alt"},
			"createChatInCurrentProject": {"cmd+alt+n"},
			"openAddProject":             {"cmd+alt+o"},
		},
		"warning":         nil,
		"filePathDisplay": "~/.cache/ai-agent-manager/keybindings.json",
	}
}

func workspaceAppSettingsSnapshot() map[string]any {
	settings, _ := state.LoadSettings()
	settings = state.NormalizeSettings(settings)
	return map[string]any{
		"analyticsEnabled":        false,
		"browserSettingsMigrated": true,
		"locale":                  settings.Locale,
		"theme":                   "system",
		"chatSoundPreference":     "unfocused",
		"chatSoundId":             "pop",
		"terminal": map[string]any{
			"scrollbackLines": 5000,
			"minColumnWidth":  8,
		},
		"editor": map[string]any{
			"preset":          "custom",
			"commandTemplate": "",
		},
		"defaultProvider": "last_used",
		"providerDefaults": map[string]any{
			"claude": map[string]any{
				"model": "claude-sonnet-4-6",
				"modelOptions": map[string]any{
					"reasoningEffort": "none",
					"contextWindow":   "200k",
				},
				"planMode": false,
			},
			"codex": map[string]any{
				"model": "gpt-5.5",
				"modelOptions": map[string]any{
					"reasoningEffort": "medium",
					"fastMode":        false,
				},
				"planMode": false,
			},
		},
		"warning":         nil,
		"filePathDisplay": state.GetSettingsFilePath(),
	}
}

func applyWorkspaceAppSettingsPatch(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		Patch struct {
			Locale string `json:"locale"`
		} `json:"patch"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.Patch.Locale == "fa" || payload.Patch.Locale == "en" {
		settings, err := state.LoadSettings()
		if err != nil {
			return nil, err
		}
		settings.Locale = payload.Patch.Locale
		if err := state.SaveSettings(settings); err != nil {
			return nil, err
		}
	}
	return workspaceAppSettingsSnapshot(), nil
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
