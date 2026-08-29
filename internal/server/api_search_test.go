package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"abolqasem/internal/state"
	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/legacyimport"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

func TestHandleAPISearchSearchesCurrentLegacyChat(t *testing.T) {
	path := writeSearchAPITestTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"first prompt"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"assistant has needle inside a longer answer"}}`,
	}, "\n"))
	meta := state.SessionMeta{
		Key:            "codex:session-1",
		Agent:          "codex",
		SessionID:      "session-1",
		TranscriptPath: path,
		Cwd:            "/tmp/project",
		ProjectName:    "Project",
		UpdatedAt:      time.Now(),
	}
	previousLegacyState := workspaceLoadLegacyState
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}}, nil
	}
	t.Cleanup(func() {
		workspaceLoadLegacyState = previousLegacyState
	})

	response := performSearchAPIRequest(t, "/api/search?chat_id="+url.QueryEscape(legacyimport.LegacyChatID(meta))+"&q=needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Matches []chatSearchMatch `json:"matches"`
		Total   int               `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Total != 1 || len(payload.Matches) != 1 {
		t.Fatalf("expected one match, got %#v", payload)
	}
	if payload.Matches[0].Role != "assistant" || !strings.Contains(payload.Matches[0].Snippet, "needle") {
		t.Fatalf("unexpected legacy match: %#v", payload.Matches[0])
	}
}

func TestHandleAPISearchSearchesCurrentWorkspaceChat(t *testing.T) {
	withWorkspaceComposerStore(t)
	project, err := workspaceOpenProject("/tmp/project", "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chatID := "chat-workspace-search"
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Legacy workspace chat",
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 300, map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "m1",
			"kind":      transcript.KindUserPrompt,
			"createdAt": float64(300),
			"content":   "hello",
		},
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 400, map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "m2",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(400),
			"text":      "workspace needle answer",
		},
	})

	response := performSearchAPIRequest(t, "/api/search?chat_id="+url.QueryEscape(chatID)+"&q=needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Matches []chatSearchMatch `json:"matches"`
		Total   int               `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Total != 1 || len(payload.Matches) != 1 {
		t.Fatalf("expected one match, got %#v", payload)
	}
	if payload.Matches[0].EntryID != "m2" || payload.Matches[0].Role != "assistant" {
		t.Fatalf("unexpected workspace match: %#v", payload.Matches[0])
	}
}

func TestHandleAPISearchSearchesNativeWorkspaceChatWithoutStoredMessages(t *testing.T) {
	withWorkspaceComposerStore(t)
	withEmptySearchLegacyState(t)
	nativePath := writeSearchAPITestTranscript(t, strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"native prompt"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"native_search_needle answer"}}`,
	}, "\n"))
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	store := &workspaceEventStore{store: workspaceStore()}
	chat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("CreateChat returned error: %v", err)
	}
	if err := store.SetChatProvider(chat.ID, "codex"); err != nil {
		t.Fatalf("SetChatProvider returned error: %v", err)
	}
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatRuntimeSet, time.Now().UnixMilli(), map[string]any{
		"chatId":               chat.ID,
		"nativeSessionId":      "native-session",
		"nativeTranscriptPath": nativePath,
	})
	if err := os.WriteFile(filepath.Join(workspaceDataDir(), events.StreamMessages+".jsonl"), []byte("{bad json\n"), 0o644); err != nil {
		t.Fatalf("write bad messages stream: %v", err)
	}

	response := performSearchAPIRequest(t, "/api/search?chat_id="+url.QueryEscape(chat.ID)+"&q=native_search_needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	var directPayload struct {
		Matches []chatSearchMatch `json:"matches"`
		Total   int               `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &directPayload); err != nil {
		t.Fatalf("unmarshal direct response: %v", err)
	}
	if directPayload.Total != 1 || directPayload.Matches[0].Role != "assistant" {
		t.Fatalf("unexpected native direct search result: %#v", directPayload)
	}

	response = performSearchAPIRequest(t, "/api/search?q=native_search_needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected global status 200, got %d: %s", response.Code, response.Body.String())
	}
	var globalPayload struct {
		Items []sessionSearchResult `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &globalPayload); err != nil {
		t.Fatalf("unmarshal global response: %v", err)
	}
	if globalPayload.Total != 1 || globalPayload.Items[0].ChatID != chat.ID {
		t.Fatalf("unexpected native global search result: %#v", globalPayload)
	}
}

func TestHandleAPISearchReadsStoredMessagesForLegacyTmuxMetadata(t *testing.T) {
	withWorkspaceComposerStore(t)
	withEmptySearchLegacyState(t)
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatalf("workspaceCreateChat returned error: %v", err)
	}
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 500, map[string]any{
		"chatId": chat.ID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "stale-message",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(500),
			"text":      "stale_tmux_search_needle answer",
		},
	})

	response := performSearchAPIRequest(t, "/api/search?chat_id="+url.QueryEscape(chat.ID)+"&q=stale_tmux_search_needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	var directPayload struct {
		Matches []chatSearchMatch `json:"matches"`
		Total   int               `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &directPayload); err != nil {
		t.Fatalf("unmarshal direct response: %v", err)
	}
	if directPayload.Total != 1 || len(directPayload.Matches) != 1 || directPayload.Matches[0].EntryID != "stale-message" {
		t.Fatalf("expected app-server search to keep stored messages, got %#v", directPayload)
	}

	response = performSearchAPIRequest(t, "/api/search?q=stale_tmux_search_needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected global status 200, got %d: %s", response.Code, response.Body.String())
	}
	var globalPayload struct {
		Items []sessionSearchResult `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &globalPayload); err != nil {
		t.Fatalf("unmarshal global response: %v", err)
	}
	if globalPayload.Total != 1 || len(globalPayload.Items) != 1 || globalPayload.Items[0].ChatID != chat.ID {
		t.Fatalf("expected app-server global search to keep stored messages, got %#v", globalPayload)
	}
}

func TestSearchWorkspaceEntriesTargetsToolCallForToolResultMatches(t *testing.T) {
	matches := searchWorkspaceEntries([]readmodels.TranscriptEntry{
		{
			"_id":  "tool-call-1",
			"kind": transcript.KindToolCall,
			"tool": map[string]any{
				"toolId":   "call-1",
				"toolKind": "bash",
				"toolName": "Bash",
				"input":    map[string]any{"command": "printf ok"},
			},
		},
		{
			"_id":     "tool-result-1",
			"kind":    transcript.KindToolResult,
			"toolId":  "call-1",
			"content": "needle from command output",
		},
	}, "needle", 10)

	if len(matches) != 1 {
		t.Fatalf("expected one match, got %#v", matches)
	}
	if matches[0].EntryID != "tool-result-1" || matches[0].MessageID != "tool-call-1" {
		t.Fatalf("expected tool result to target rendered tool call, got %#v", matches[0])
	}
}

func TestHandleAPISearchGlobalIncludesWorkspaceChats(t *testing.T) {
	withWorkspaceComposerStore(t)
	withEmptySearchLegacyState(t)
	project, err := workspaceOpenProject("/tmp/project", "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chatID := "chat-global-workspace-search"
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatCreated, 200, map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Legacy workspace chat",
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 500, map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "m1",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(500),
			"text":      "global workspace unique_search_token_4729 answer",
		},
	})

	response := performSearchAPIRequest(t, "/api/search?q=unique_search_token_4729&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Items []sessionSearchResult `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected one global result, got %#v", payload)
	}
	if payload.Items[0].ChatID != chatID || payload.Items[0].SearchMatchCount != 1 {
		t.Fatalf("unexpected global workspace result: %#v", payload.Items[0])
	}
}

func TestHandleAPISearchGlobalKeepsStoredWorkspaceChatAfterCompaction(t *testing.T) {
	withWorkspaceComposerStore(t)
	withEmptySearchLegacyState(t)
	project, err := workspaceOpenProject("/tmp/project", "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chatID := "chat-restored-search"
	appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Stored Chat",
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 500, map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "m1",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(500),
			"text":      "compacted_search_needle answer",
		},
	})
	stateSnapshot, err := workspaceStore().LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if err := workspaceStore().Compact(stateSnapshot); err != nil {
		t.Fatalf("Compact returned error: %v", err)
	}

	response := performSearchAPIRequest(t, "/api/search?q=compacted_search_needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []sessionSearchResult `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Total != 1 || payload.Items[0].ChatID != chatID {
		t.Fatalf("unexpected compacted workspace result: %#v", payload)
	}
}

func TestHandleAPISearchGlobalSkipsArchivedChatsAndScopesToProject(t *testing.T) {
	withWorkspaceComposerStore(t)
	withEmptySearchLegacyState(t)
	firstProject, err := workspaceOpenProject(t.TempDir(), "First project")
	if err != nil {
		t.Fatalf("workspaceOpenProject first project: %v", err)
	}
	secondProject, err := workspaceOpenProject(t.TempDir(), "Second project")
	if err != nil {
		t.Fatalf("workspaceOpenProject second project: %v", err)
	}
	appendChatWithSearchEntry := func(chatID, projectID, text string, archived bool) {
		t.Helper()
		appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatCreated, time.Now().UnixMilli(), map[string]any{
			"chatId": chatID, "projectId": projectID, "title": chatID,
		})
		appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, time.Now().UnixMilli(), map[string]any{
			"chatId": chatID,
			"entry":  readmodels.TranscriptEntry{"_id": chatID + "-message", "kind": transcript.KindAssistantText, "text": text},
		})
		if archived {
			appendWorkspaceEvent(t, workspaceStore(), events.StreamChats, events.TypeChatArchived, time.Now().UnixMilli(), map[string]any{"chatId": chatID})
		}
	}
	appendChatWithSearchEntry("chat-first-active", firstProject.ID, "shared project_search_needle active", false)
	appendChatWithSearchEntry("chat-first-archived", firstProject.ID, "shared project_search_needle archived", true)
	appendChatWithSearchEntry("chat-second-active", secondProject.ID, "shared project_search_needle second", false)

	response := performSearchAPIRequest(t, "/api/search?q=project_search_needle&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected global status 200, got %d: %s", response.Code, response.Body.String())
	}
	var global struct {
		Items []sessionSearchResult `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &global); err != nil {
		t.Fatalf("unmarshal global response: %v", err)
	}
	if len(global.Items) != 2 {
		t.Fatalf("expected only active chats in global search, got %#v", global.Items)
	}
	for _, item := range global.Items {
		if item.ChatID == "chat-first-archived" {
			t.Fatalf("archived chat leaked into global search: %#v", global.Items)
		}
	}

	response = performSearchAPIRequest(t, "/api/search?q=project_search_needle&project_id="+url.QueryEscape(firstProject.ID)+"&limit=10")
	if response.Code != http.StatusOK {
		t.Fatalf("expected project status 200, got %d: %s", response.Code, response.Body.String())
	}
	var projectScoped struct {
		Items []sessionSearchResult `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &projectScoped); err != nil {
		t.Fatalf("unmarshal project response: %v", err)
	}
	if len(projectScoped.Items) != 1 || projectScoped.Items[0].ChatID != "chat-first-active" {
		t.Fatalf("expected only the active chat in requested project, got %#v", projectScoped.Items)
	}
}

func withEmptySearchLegacyState(t *testing.T) {
	t.Helper()
	previous := workspaceLoadLegacyState
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return &state.AppState{Sessions: map[string]state.SessionMeta{}}, nil
	}
	t.Cleanup(func() { workspaceLoadLegacyState = previous })
}

func performSearchAPIRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handleAPISearch(response, request)
	return response
}

func writeSearchAPITestTranscript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return path
}
