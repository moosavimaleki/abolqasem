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

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/legacyimport"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
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
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatalf("workspaceCreateChat returned error: %v", err)
	}
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 300, map[string]any{
		"chatId": chat.ID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "m1",
			"kind":      transcript.KindUserPrompt,
			"createdAt": float64(300),
			"content":   "hello",
		},
	})
	appendWorkspaceEvent(t, workspaceStore(), events.StreamMessages, events.TypeMessageAppended, 400, map[string]any{
		"chatId": chat.ID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "m2",
			"kind":      transcript.KindAssistantText,
			"createdAt": float64(400),
			"text":      "workspace needle answer",
		},
	})

	response := performSearchAPIRequest(t, "/api/search?chat_id="+url.QueryEscape(chat.ID)+"&q=needle&limit=10")
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
	project, err := workspaceOpenProject("/tmp/project", "Project")
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
	if payload.Items[0].ChatID != chat.ID || payload.Items[0].SearchMatchCount != 1 {
		t.Fatalf("unexpected global workspace result: %#v", payload.Items[0])
	}
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
