package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/legacyimport"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"
)

func withLegacyState(t *testing.T, appState *state.AppState) {
	t.Helper()
	t.Setenv("ABOLQASEM_TMUX_CODEX_COMMAND", "true")
	t.Setenv("ABOLQASEM_TMUX_CLAUDE_COMMAND", "true")
	t.Setenv("ABOLQASEM_TMUX_GEMINI_COMMAND", "true")
	previousLoad := workspaceLoadLegacyState
	previousSave := workspaceSaveLegacyState
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return appState, nil
	}
	workspaceSaveLegacyState = func(next *state.AppState) error {
		appState = next
		return nil
	}
	t.Cleanup(func() {
		workspaceLoadLegacyState = previousLoad
		workspaceSaveLegacyState = previousSave
	})
}

func TestWorkspaceRecentTranscriptEntriesReturnsEmptySliceForNilEntries(t *testing.T) {
	messages, history := workspaceRecentTranscriptEntries(nil, 20)
	if messages == nil {
		t.Fatal("expected empty messages slice, got nil")
	}
	if len(messages) != 0 {
		t.Fatalf("expected no messages, got %d", len(messages))
	}
	if history.HasOlder || history.OlderCursor != nil || history.RecentLimit != 20 {
		t.Fatalf("unexpected history: %#v", history)
	}
}

func TestMergeLegacySidebarDataSkipsLegacySessionsWithoutRealProjectRoot(t *testing.T) {
	updatedAt := time.Unix(1700000000, 0)
	home := t.TempDir()
	setTestUserHome(t, home)
	appState := &state.AppState{Sessions: map[string]state.SessionMeta{
		"codex:bad-1": {
			Key:            "codex:bad-1",
			Agent:          "codex",
			SessionID:      "bad-1",
			SessionName:    "Bad Session",
			TranscriptPath: filepath.Join(home, ".codex", "sessions", "2025", "09", "01", "rollout.jsonl"),
			Cwd:            filepath.Join(home, ".codex", "sessions", "2025", "09", "01"),
			ProjectName:    "01",
			UpdatedAt:      updatedAt,
			MetadataOnly:   true,
		},
	}}
	withLegacyState(t, appState)

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 0 {
		t.Fatalf("expected malformed legacy session to be hidden, got %#v", sidebar.ProjectGroups)
	}
}

func TestMergeLegacySidebarDataAddsImportedSessionsAsNormalChats(t *testing.T) {
	updatedAt := time.Unix(1700000000, 0)
	appState := &state.AppState{Sessions: map[string]state.SessionMeta{
		"claude:legacy-1": {
			Key:            "claude:legacy-1",
			Agent:          "claude",
			SessionID:      "legacy-1",
			SessionName:    "Legacy Session",
			TranscriptPath: "/tmp/project/transcript.jsonl",
			Cwd:            "/tmp/project",
			ProjectName:    "Project",
			UpdatedAt:      updatedAt,
			MetadataOnly:   true,
		},
	}}
	withLegacyState(t, appState)

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 1 {
		t.Fatalf("expected one project group, got %#v", sidebar.ProjectGroups)
	}
	chats := sidebar.ProjectGroups[0].Chats
	if len(chats) != 1 {
		t.Fatalf("expected one imported chat, got %#v", chats)
	}
	if len(sidebar.ProjectGroups[0].PreviewChats) != 1 {
		t.Fatalf("expected one visible imported chat, got %#v", sidebar.ProjectGroups[0].PreviewChats)
	}
	if chats[0].ReadOnly || chats[0].LegacySessionKey != "" {
		t.Fatalf("unexpected imported chat row metadata: %#v", chats[0])
	}
	if !strings.HasPrefix(chats[0].ChatID, "chat-") || strings.HasPrefix(chats[0].ChatID, "legacy-") {
		t.Fatalf("expected normal chat id, got %q", chats[0].ChatID)
	}
}

func TestMergeLegacySidebarDataUsesLatestLegacyActivityAndUnreadFlag(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-23T11-09-51-019e53c6-b496-7320-8735-711ec53cb9fc.jsonl")
	userAt := time.Date(2026, 5, 23, 7, 39, 57, 247000000, time.UTC)
	assistantAt := time.Date(2026, 5, 23, 7, 39, 58, 16000000, time.UTC)
	body := `{"timestamp":"2026-05-23T07:39:57.247Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"عنوان از پیام کاربر"}]}}` + "\n" +
		`{"timestamp":"2026-05-23T07:39:58.016Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"پاسخ تازه دستیار"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:019e53c6-b496-7320-8735-711ec53cb9fc",
		Agent:          "codex",
		SessionID:      "019e53c6-b496-7320-8735-711ec53cb9fc",
		SessionName:    "019e53c6-b496-7320-8735-711ec53cb9fc",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      assistantAt,
	}
	appState := &state.AppState{
		Sessions:          map[string]state.SessionMeta{meta.Key: meta},
		UnreadSessionKeys: map[string]bool{meta.Key: true},
		LatestSessionKey:  meta.Key,
		LatestSessionID:   meta.SessionID,
	}
	withLegacyState(t, appState)

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected one imported chat, got %#v", sidebar.ProjectGroups)
	}
	chat := sidebar.ProjectGroups[0].Chats[0]
	if !chat.Unread {
		t.Fatalf("expected imported legacy chat to surface unread state, got %#v", chat)
	}
	if chat.LastMessageAt == nil || *chat.LastMessageAt != assistantAt.UnixMilli() {
		t.Fatalf("expected assistant activity timestamp %d, got %#v", assistantAt.UnixMilli(), chat.LastMessageAt)
	}
	if chat.LastMessageAt != nil && *chat.LastMessageAt == userAt.UnixMilli() {
		t.Fatalf("expected assistant activity to outrank the last user prompt, got %#v", chat.LastMessageAt)
	}
}

func TestMergeLegacySidebarDataUsesFirstUserMessageTitle(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"agent_message","message":"assistant preface"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"اولین پیام کاربر"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		SessionName:    "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected one imported chat, got %#v", sidebar.ProjectGroups)
	}
	if got := sidebar.ProjectGroups[0].Chats[0].Title; got != "اولین پیام کاربر" {
		t.Fatalf("expected title from first user message, got %q", got)
	}
}

func TestMergeLegacySidebarDataUsesCodexResponseItemUserTitle(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-23T11-09-51-019e53c6-b496-7320-8735-711ec53cb9fc.jsonl")
	body := `{"timestamp":"2026-05-23T07:39:57.247Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"عنوان از پیام کاربر"}]}}` + "\n" +
		`{"timestamp":"2026-05-23T07:39:58.016Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"پاسخ دستیار"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:019e53c6-b496-7320-8735-711ec53cb9fc",
		Agent:          "codex",
		SessionID:      "019e53c6-b496-7320-8735-711ec53cb9fc",
		SessionName:    "019e53c6-b496-7320-8735-711ec53cb9fc",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected one imported chat, got %#v", sidebar.ProjectGroups)
	}
	if got := sidebar.ProjectGroups[0].Chats[0].Title; got != "عنوان از پیام کاربر" {
		t.Fatalf("expected title from codex response_item user message, got %q", got)
	}
}

func TestMergeLegacySidebarDataUsesCodexResponseItemUserTitleWhenSessionNameEmpty(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-23T11-09-51-019e53c6-b496-7320-8735-711ec53cb9fc.jsonl")
	bootstrap := "# AGENTS.md instructions for /home/h-mousavi/Projects/Hamed/codex-rtl-plugin\n\n<INSTRUCTIONS>\n@/home/h-mousavi/.codex/RTK.md\n</INSTRUCTIONS>\n\n<environment_context>\n  <cwd>/home/h-mousavi/Projects/Hamed/codex-rtl-plugin</cwd>\n  <shell>zsh</shell>\n</environment_context>"
	body := `{"timestamp":"2026-05-23T07:39:57.247Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":` + strconv.Quote(bootstrap) + `}]}}` + "\n" +
		`{"timestamp":"2026-05-23T07:40:57.247Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"عنوان از پیام کاربر با نام خالی"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:019e53c6-b496-7320-8735-711ec53cb9fc",
		Agent:          "codex",
		SessionID:      "019e53c6-b496-7320-8735-711ec53cb9fc",
		SessionName:    "",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	sidebar := mergeLegacySidebarData(readmodels.SidebarData{})
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected one imported chat, got %#v", sidebar.ProjectGroups)
	}
	if got := sidebar.ProjectGroups[0].Chats[0].Title; got != "عنوان از پیام کاربر با نام خالی" {
		t.Fatalf("expected title from codex response_item user message when session name empty, got %q", got)
	}
}

func TestMergeLegacySidebarDataRepairsGeneratedStoredTitle(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	t.Cleanup(func() { workspaceDataDir = previousDataDir })

	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"اولین پیام کاربر"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		SessionName:    "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	imported := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{})
	store := workspaceStore()
	if err := appendWorkspaceStoreEvent(store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": imported.Project.ID,
		"localPath": imported.Project.LocalPath,
		"title":     imported.Project.Title,
	}); err != nil {
		t.Fatalf("append project: %v", err)
	}
	if err := appendWorkspaceStoreEvent(store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    imported.Chat.ID,
		"projectId": imported.Project.ID,
		"title":     meta.SessionID,
	}); err != nil {
		t.Fatalf("append chat: %v", err)
	}
	storeState, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}

	sidebar := mergeLegacySidebarData(readmodels.DeriveSidebarData(storeState))
	if got := sidebar.ProjectGroups[0].Chats[0].Title; got != "اولین پیام کاربر" {
		t.Fatalf("expected sidebar title repaired from first user message, got %q", got)
	}
	storeState, err = store.LoadState()
	if err != nil {
		t.Fatalf("LoadState after repair returned error: %v", err)
	}
	if got := storeState.ChatsByID[imported.Chat.ID].Title; got != "اولین پیام کاربر" {
		t.Fatalf("expected stored chat title to be repaired, got %q", got)
	}
}

func TestMergeLegacySidebarDataRepairsBootstrapStoredTitle(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	t.Cleanup(func() { workspaceDataDir = previousDataDir })

	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"اولین پیام واقعی"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	imported := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{})
	store := workspaceStore()
	bootstrapTitle := "AGENTS.md instructions for /home/h-mousavi/Projects/Hamed/codex-rtl-plugin <INSTRUCTIONS> @/home/h-mousavi/.codex/RTK.md </INSTRUCTIONS> <environment_context> <cwd>/home/h-mousavi/Projects/Hamed/codex-rtl-plugin</cwd> <shell>zsh</shell> </environment_context>"
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": imported.Project.ID,
		"localPath": imported.Project.LocalPath,
		"title":     imported.Project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    imported.Chat.ID,
		"projectId": imported.Project.ID,
		"title":     bootstrapTitle,
	})

	storeState, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	sidebar := mergeLegacySidebarData(readmodels.DeriveSidebarData(storeState))
	if got := sidebar.ProjectGroups[0].Chats[0].Title; got != "اولین پیام واقعی" {
		t.Fatalf("expected sidebar bootstrap title to be repaired, got %q", got)
	}
	storeState, err = store.LoadState()
	if err != nil {
		t.Fatalf("LoadState after repair returned error: %v", err)
	}
	if got := storeState.ChatsByID[imported.Chat.ID].Title; got != "اولین پیام واقعی" {
		t.Fatalf("expected stored bootstrap title to be repaired, got %q", got)
	}
}

func TestWorkspaceLegacyChatSnapshotImportsTranscript(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"hello"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"hi"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	snapshot := workspaceLegacyChatSnapshot(legacyimport.LegacyChatID(meta), 10).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.LegacySessionKey != "" || snapshot.Runtime.ReadOnly {
		t.Fatalf("unexpected imported runtime: %#v", snapshot.Runtime)
	}
	if snapshot.Runtime.SessionToken == nil || *snapshot.Runtime.SessionToken != meta.SessionID {
		t.Fatalf("expected imported runtime session token %q, got %#v", meta.SessionID, snapshot.Runtime.SessionToken)
	}
	if !strings.HasPrefix(snapshot.Runtime.ChatID, "chat-") || strings.HasPrefix(snapshot.Runtime.ChatID, "legacy-") {
		t.Fatalf("expected normal chat id, got %q", snapshot.Runtime.ChatID)
	}
	if snapshot.Runtime.TmuxSession == "" || snapshot.Runtime.NativeSessionID != meta.SessionID || snapshot.Runtime.NativeTranscriptPath != transcriptPath {
		t.Fatalf("expected legacy snapshot to expose native runtime metadata, got %#v", snapshot.Runtime)
	}
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected legacy snapshot to avoid imported transcript messages, got %#v", snapshot.Messages)
	}
}

func TestWorkspaceLoadChatHistorySupportsLegacyChats(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"first"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"second"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"third"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	payload, err := json.Marshal(map[string]any{
		"chatId":       legacyimport.LegacyChatID(meta),
		"beforeCursor": "3",
		"limit":        1,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	page, err := workspaceLoadChatHistory(payload)
	if err != nil {
		t.Fatalf("workspaceLoadChatHistory returned error: %v", err)
	}
	messages, ok := page["messages"].([]readmodels.TranscriptEntry)
	if !ok || len(messages) != 0 {
		t.Fatalf("expected no legacy history entries, got %#v", page["messages"])
	}
	if page["hasOlder"] != false || page["olderCursor"] != nil {
		t.Fatalf("expected empty legacy history, got %#v", page)
	}
}

func TestWorkspaceLoadChatHistoryAroundSupportsLegacyChats(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-legacy-around.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"first"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"second"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"third"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:legacy-around",
		Agent:          "codex",
		SessionID:      "legacy-around",
		TranscriptPath: transcriptPath,
		Cwd:            dir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	payload, err := json.Marshal(map[string]any{
		"chatId":       legacyimport.LegacyChatID(meta),
		"targetCursor": "2",
		"limit":        1,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	page, err := workspaceLoadChatHistoryAround(payload)
	if err != nil {
		t.Fatalf("workspaceLoadChatHistoryAround returned error: %v", err)
	}
	messages, ok := page["messages"].([]readmodels.TranscriptEntry)
	if !ok || len(messages) != 0 {
		t.Fatalf("expected no legacy history entries, got %#v", page["messages"])
	}
	if page["targetFound"] != false {
		t.Fatalf("expected targetFound=false, got %#v", page)
	}
}

func TestWorkspaceLoadChatHistoryAroundSupportsStoredChats(t *testing.T) {
	withWorkspaceComposerStore(t)
	store := workspaceStore()
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": "project-1",
		"localPath": t.TempDir(),
		"title":     "Project",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 101, map[string]any{
		"chatId":    "chat-1",
		"projectId": "project-1",
		"title":     "Chat",
	})
	for index, content := range []string{"first", "second", "third"} {
		appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, int64(200+index), map[string]any{
			"chatId": "chat-1",
			"entry": transcript.New(transcript.KindUserPrompt, map[string]any{
				"_id":     "message-" + strconv.Itoa(index+1),
				"content": content,
			}),
		})
	}

	payload, err := json.Marshal(map[string]any{
		"chatId":       "chat-1",
		"targetCursor": "message-2",
		"limit":        1,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	page, err := workspaceLoadChatHistoryAround(payload)
	if err != nil {
		t.Fatalf("workspaceLoadChatHistoryAround returned error: %v", err)
	}
	messages, ok := page["messages"].([]readmodels.TranscriptEntry)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one stored history entry, got %#v", page["messages"])
	}
	if messages[0]["_id"] != "message-2" {
		t.Fatalf("expected target message, got %#v", messages[0])
	}
	if page["targetFound"] != true {
		t.Fatalf("expected targetFound=true, got %#v", page)
	}
}

func TestWorkspaceMaterializeLegacyChatCreatesWritableChat(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"hello"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"hi"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	chatID, err := workspaceMaterializeLegacyChat(legacyimport.LegacyChatAliasID(meta))
	if err != nil {
		t.Fatalf("workspaceMaterializeLegacyChat returned error: %v", err)
	}
	if chatID != legacyimport.ImportedChatID(meta) {
		t.Fatalf("expected imported chat id, got %q", chatID)
	}

	storeState, err := workspaceStore().LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	chat, ok := storeState.ChatsByID[chatID]
	if !ok {
		t.Fatalf("expected materialized chat %q in store", chatID)
	}
	if chat.SessionToken == nil || *chat.SessionToken != meta.SessionID {
		t.Fatalf("expected codex session token %q, got %#v", meta.SessionID, chat.SessionToken)
	}
	if chat.Provider == nil || *chat.Provider != "codex" {
		t.Fatalf("expected codex provider, got %#v", chat.Provider)
	}
	if chat.TmuxSession != workspaceChatTmuxSession(chatID) || chat.NativeSessionID != meta.SessionID || chat.NativeTranscriptPath != transcriptPath {
		t.Fatalf("expected tmux/native runtime metadata, got %#v", chat)
	}
	messageEvents, err := workspaceStore().Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay messages returned error: %v", err)
	}
	if len(messageEvents) != 0 {
		t.Fatalf("expected materialize to avoid message events, got %#v", messageEvents)
	}

	snapshot := workspaceChatSnapshot(chatID, 10).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.ReadOnly {
		t.Fatalf("materialized chat should be writable: %#v", snapshot.Runtime)
	}
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected materialized native chat to avoid stored transcript messages, got %#v", snapshot.Messages)
	}
}

func TestWorkspaceRecordHookPromptCheckpointMaterializesTUIChat(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write project file: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"event_msg","payload":{"type":"user_message","message":"old prompt"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"old answer"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		SessionName:    "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	record, err := workspaceRecordHookPromptCheckpoint(meta, state.HookEvent{
		Agent:         "codex",
		HookEventName: "UserPromptSubmit",
		PromptPreview: "new prompt",
	})
	if err != nil {
		t.Fatalf("workspaceRecordHookPromptCheckpoint returned error: %v", err)
	}
	if record.ID == "" || record.ChatID != legacyimport.ImportedChatID(meta) {
		t.Fatalf("unexpected checkpoint record: %#v", record)
	}
	if record.PromptPreview != "new prompt" {
		t.Fatalf("expected prompt preview, got %q", record.PromptPreview)
	}
	if len(record.Messages) != 0 || record.ChatMessageCount != 0 || record.ChatSnapshotPath != "" {
		t.Fatalf("expected checkpoint to skip chat transcript storage, got %#v", record)
	}

	storeState, err := workspaceStore().LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	chat := storeState.ChatsByID[legacyimport.ImportedChatID(meta)]
	if chat.Title != "old prompt" {
		t.Fatalf("expected materialized chat title from first prompt, got %q", chat.Title)
	}
	if chat.TmuxSession == "" || chat.NativeSessionID != meta.SessionID {
		t.Fatalf("expected materialized hook chat to keep native runtime, got %#v", chat)
	}
}

func TestWorkspaceMaterializeLegacyChatReusesStoredChatBySessionToken(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"old prompt"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	store := workspaceStore()
	project := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": project.ID,
		"localPath": project.LocalPath,
		"title":     project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    "chat-existing",
		"projectId": project.ID,
		"title":     "New Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   "chat-existing",
		"provider": "codex",
	})
	appendWorkspaceEvent(t, store, events.StreamTurns, events.TypeSessionTokenSet, 102, map[string]any{
		"chatId":       "chat-existing",
		"sessionToken": meta.SessionID,
	})

	chatID, err := workspaceMaterializeLegacyChat(legacyimport.LegacyChatAliasID(meta))
	if err != nil {
		t.Fatalf("workspaceMaterializeLegacyChat returned error: %v", err)
	}
	if chatID != "chat-existing" {
		t.Fatalf("expected existing chat to be reused, got %q", chatID)
	}

	storeState, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if _, ok := storeState.ChatsByID[legacyimport.ImportedChatID(meta)]; ok {
		t.Fatalf("expected no duplicate imported chat, got %#v", storeState.ChatsByID[legacyimport.ImportedChatID(meta)])
	}
}

func TestWorkspaceLegacyBroadcastChatIDReusesStoredChatBySessionToken(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	store := workspaceStore()
	project := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": project.ID,
		"localPath": project.LocalPath,
		"title":     project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    "chat-existing",
		"projectId": project.ID,
		"title":     "New Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   "chat-existing",
		"provider": "codex",
	})
	appendWorkspaceEvent(t, store, events.StreamTurns, events.TypeSessionTokenSet, 102, map[string]any{
		"chatId":       "chat-existing",
		"sessionToken": meta.SessionID,
	})

	if chatID := workspaceLegacyBroadcastChatID(meta); chatID != "chat-existing" {
		t.Fatalf("expected hook broadcast to target existing chat, got %q", chatID)
	}
}

func TestWorkspaceRecordHookPromptCheckpointReusesStoredChatBySessionToken(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"old prompt"}]}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old answer"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	store := workspaceStore()
	project := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": project.ID,
		"localPath": project.LocalPath,
		"title":     project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    "chat-existing",
		"projectId": project.ID,
		"title":     "New Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   "chat-existing",
		"provider": "codex",
	})
	appendWorkspaceEvent(t, store, events.StreamTurns, events.TypeSessionTokenSet, 102, map[string]any{
		"chatId":       "chat-existing",
		"sessionToken": meta.SessionID,
	})

	record, err := workspaceRecordHookPromptCheckpoint(meta, state.HookEvent{
		Agent:         "codex",
		HookEventName: "UserPromptSubmit",
		PromptPreview: "new prompt",
	})
	if err != nil {
		t.Fatalf("workspaceRecordHookPromptCheckpoint returned error: %v", err)
	}
	if record.ChatID != "chat-existing" {
		t.Fatalf("expected hook checkpoint to target existing chat, got %#v", record)
	}

	storeState, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	sidebar := mergeLegacySidebarData(readmodels.DeriveSidebarData(storeState))
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected sidebar to keep one chat row, got %#v", sidebar.ProjectGroups)
	}
}

func TestWorkspaceRecordHookPromptCheckpointReusesStoredChatByPendingForkSessionToken(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	body := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"old prompt"}]}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old answer"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	store := workspaceStore()
	project := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": project.ID,
		"localPath": project.LocalPath,
		"title":     project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    "chat-existing",
		"projectId": project.ID,
		"title":     "New Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   "chat-existing",
		"provider": "codex",
	})
	appendWorkspaceEvent(t, store, events.StreamTurns, events.TypePendingForkSessionTokenSet, 102, map[string]any{
		"chatId":                  "chat-existing",
		"pendingForkSessionToken": meta.SessionID,
	})

	record, err := workspaceRecordHookPromptCheckpoint(meta, state.HookEvent{
		Agent:         "codex",
		HookEventName: "UserPromptSubmit",
		PromptPreview: "new prompt",
	})
	if err != nil {
		t.Fatalf("workspaceRecordHookPromptCheckpoint returned error: %v", err)
	}
	if record.ChatID != "chat-existing" {
		t.Fatalf("expected hook checkpoint to target existing chat, got %#v", record)
	}

	storeState, err := store.LoadState()
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	sidebar := mergeLegacySidebarData(readmodels.DeriveSidebarData(storeState))
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected sidebar to keep one chat row, got %#v", sidebar.ProjectGroups)
	}
}

func TestWorkspaceMaterializeLegacyClaudeChatKeepsNativeRuntime(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699.jsonl")
	body := `{"type":"user","message":{"role":"user","content":"ببین این claude-flow چیه؟"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"دارم بررسی می‌کنم"},{"type":"tool_use","id":"call_1","name":"TaskCreate","input":{"description":"Locate claude-flow artifacts"}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"Task #1 created successfully"}]}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"خلاصه را آماده می‌کنم"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "claude:1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		Agent:          "claude",
		SessionID:      "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		SessionName:    "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	chatID, err := workspaceMaterializeLegacyChat(legacyimport.LegacyChatAliasID(meta))
	if err != nil {
		t.Fatalf("workspaceMaterializeLegacyChat returned error: %v", err)
	}
	snapshot := workspaceChatSnapshot(chatID, 20).(*readmodels.ChatSnapshot)
	if snapshot.Runtime.Provider == nil || *snapshot.Runtime.Provider != "claude" {
		t.Fatalf("expected claude runtime, got %#v", snapshot.Runtime)
	}
	if snapshot.Runtime.TmuxSession != workspaceChatTmuxSession(chatID) || snapshot.Runtime.NativeSessionID != meta.SessionID || snapshot.Runtime.NativeTranscriptPath != transcriptPath {
		t.Fatalf("expected native tmux runtime metadata, got %#v", snapshot.Runtime)
	}
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected claude materialize to avoid stored transcript messages, got %#v", snapshot.Messages)
	}
}

func TestWorkspaceSyncLegacyBackedChatMarksUnreadOnExternalUpdate(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-23T11-09-51-019e53c6-b496-7320-8735-711ec53cb9fc.jsonl")
	writeTranscript := func(body string) {
		if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
			t.Fatalf("write transcript: %v", err)
		}
	}
	userAt := time.Date(2026, 5, 23, 7, 39, 57, 247000000, time.UTC)
	assistantAt := time.Date(2026, 5, 23, 7, 39, 58, 16000000, time.UTC)
	writeTranscript(`{"timestamp":"2026-05-23T07:39:57.247Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"عنوان از پیام کاربر"}]}}` + "\n")

	meta := state.SessionMeta{
		Key:            "codex:019e53c6-b496-7320-8735-711ec53cb9fc",
		Agent:          "codex",
		SessionID:      "019e53c6-b496-7320-8735-711ec53cb9fc",
		SessionName:    "019e53c6-b496-7320-8735-711ec53cb9fc",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      userAt,
	}
	appState := &state.AppState{
		Sessions:         map[string]state.SessionMeta{meta.Key: meta},
		LatestSessionKey: meta.Key,
		LatestSessionID:  meta.SessionID,
	}
	withLegacyState(t, appState)

	chatID, err := workspaceMaterializeLegacyChat(legacyimport.LegacyChatAliasID(meta))
	if err != nil {
		t.Fatalf("workspaceMaterializeLegacyChat returned error: %v", err)
	}

	writeTranscript(
		`{"timestamp":"2026-05-23T07:39:57.247Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"عنوان از پیام کاربر"}]}}` + "\n" +
			`{"timestamp":"2026-05-23T07:39:58.016Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"پاسخ تازه دستیار"}]}}` + "\n",
	)
	meta.UpdatedAt = assistantAt
	appState.Sessions[meta.Key] = meta

	if err := workspaceSyncLegacyBackedChat(chatID, meta); err != nil {
		t.Fatalf("workspaceSyncLegacyBackedChat returned error: %v", err)
	}

	sidebar := workspaceSidebarSnapshot().(readmodels.SidebarData)
	if len(sidebar.ProjectGroups) != 1 || len(sidebar.ProjectGroups[0].Chats) != 1 {
		t.Fatalf("expected one materialized chat row, got %#v", sidebar.ProjectGroups)
	}
	chat := sidebar.ProjectGroups[0].Chats[0]
	if !chat.Unread {
		t.Fatalf("expected synced chat to become unread after external update, got %#v", chat)
	}
	if chat.LastMessageAt == nil || *chat.LastMessageAt < assistantAt.UnixMilli() {
		t.Fatalf("expected synced chat activity to move forward from %d, got %#v", assistantAt.UnixMilli(), chat.LastMessageAt)
	}
}

func TestWorkspaceMarkChatReadClearsLegacyUnreadState(t *testing.T) {
	meta := state.SessionMeta{
		Key:            "codex:legacy-session-1",
		Agent:          "codex",
		SessionID:      "legacy-session-1",
		TranscriptPath: "/tmp/rollout.jsonl",
		Cwd:            "/tmp/project",
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	appState := &state.AppState{
		Sessions:          map[string]state.SessionMeta{meta.Key: meta},
		UnreadSessionKeys: map[string]bool{meta.Key: true},
		LatestSessionKey:  meta.Key,
		LatestSessionID:   meta.SessionID,
	}
	withLegacyState(t, appState)

	if err := workspaceMarkChatRead(legacyimport.ImportedChatID(meta)); err != nil {
		t.Fatalf("workspaceMarkChatRead returned error: %v", err)
	}
	if appState.UnreadSessionKeys[meta.Key] {
		t.Fatalf("expected mark read to clear legacy unread state, got %#v", appState.UnreadSessionKeys)
	}
}

func TestWorkspaceChatSnapshotLinksMalformedMaterializedClaudeImportToNativeRuntime(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699.jsonl")
	body := `{"type":"user","message":{"role":"user","content":"ببین این claude-flow چیه؟"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"دارم بررسی می‌کنم"},{"type":"tool_use","id":"call_1","name":"TaskCreate","input":{"description":"Locate claude-flow artifacts"}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"Task #1 created successfully"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "claude:1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		Agent:          "claude",
		SessionID:      "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		SessionName:    "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	imported := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{})
	store := workspaceStore()
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": imported.Project.ID,
		"localPath": imported.Project.LocalPath,
		"title":     imported.Project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    imported.Chat.ID,
		"projectId": imported.Project.ID,
		"title":     "New Chat",
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 101, map[string]any{
		"chatId": imported.Chat.ID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "bad-user",
			"kind":      transcript.KindUserPrompt,
			"messageId": "evt_1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699_1",
			"createdAt": float64(101),
			"content":   "ببین این claude-flow چیه؟",
		},
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 102, map[string]any{
		"chatId": imported.Chat.ID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "bad-tool-name",
			"kind":      transcript.KindAssistantText,
			"messageId": "evt_1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699_2",
			"createdAt": float64(102),
			"text":      "TaskCreate",
		},
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 103, map[string]any{
		"chatId": imported.Chat.ID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "bad-tool-result",
			"kind":      transcript.KindUserPrompt,
			"messageId": "evt_1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699_3",
			"createdAt": float64(103),
			"content":   "Task #1 created successfully",
		},
	})

	beforeMessages, err := store.Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay before returned error: %v", err)
	}

	snapshot := workspaceChatSnapshot(imported.Chat.ID, 20).(*readmodels.ChatSnapshot)
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected native runtime snapshot to ignore malformed stored messages, got %#v", snapshot.Messages)
	}
	if snapshot.Runtime.TmuxSession == "" || snapshot.Runtime.NativeSessionID != meta.SessionID || snapshot.Runtime.NativeTranscriptPath != transcriptPath {
		t.Fatalf("expected native runtime metadata, got %#v", snapshot.Runtime)
	}
	afterMessages, err := store.Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay after returned error: %v", err)
	}
	if len(afterMessages) != len(beforeMessages) {
		t.Fatalf("expected snapshot to avoid rewriting messages, before=%d after=%d", len(beforeMessages), len(afterMessages))
	}
}

func TestWorkspaceChatSnapshotMarksPreviouslyRestoredLegacyChatSynced(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "rollout-2026-05-16T00-00-00-000Z-aaaa-bbbb-cccc-dddd-eeee.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(`{"type":"event_msg","payload":{"type":"user_message","message":"new from legacy"}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:aaaa-bbbb-cccc-dddd-eeee",
		Agent:          "codex",
		SessionID:      "aaaa-bbbb-cccc-dddd-eeee",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000100, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	imported := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{})
	store := workspaceStore()
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": imported.Project.ID,
		"localPath": imported.Project.LocalPath,
		"title":     imported.Project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    imported.Chat.ID,
		"projectId": imported.Project.ID,
		"title":     imported.Chat.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   imported.Chat.ID,
		"provider": "codex",
	})
	appendWorkspaceEvent(t, store, events.StreamTurns, events.TypeSessionTokenSet, 102, map[string]any{
		"chatId":       imported.Chat.ID,
		"sessionToken": meta.SessionID,
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeChatRestoredToCheckpoint, meta.UpdatedAt.UnixMilli(), map[string]any{
		"chatId": imported.Chat.ID,
		"messages": []readmodels.TranscriptEntry{{
			"_id":       "already-restored",
			"kind":      transcript.KindUserPrompt,
			"content":   "already restored",
			"createdAt": meta.UpdatedAt.UnixMilli(),
		}},
	})

	beforeMessages, err := store.Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay before returned error: %v", err)
	}

	snapshot := workspaceChatSnapshot(imported.Chat.ID, 20).(*readmodels.ChatSnapshot)
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected native runtime snapshot to ignore restored message stream, got %#v", snapshot.Messages)
	}
	if snapshot.Runtime.TmuxSession == "" || snapshot.Runtime.NativeSessionID != meta.SessionID {
		t.Fatalf("expected restored legacy chat to link native runtime, got %#v", snapshot.Runtime)
	}

	afterMessages, err := store.Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay after returned error: %v", err)
	}
	if len(afterMessages) != len(beforeMessages) {
		t.Fatalf("expected snapshot to skip duplicate restore, before=%d after=%d", len(beforeMessages), len(afterMessages))
	}

	lightState, err := store.LoadStateLight()
	if err != nil {
		t.Fatalf("LoadStateLight returned error: %v", err)
	}
	chat := lightState.ChatsByID[imported.Chat.ID]
	if workspaceLegacySessionNeedsSync(chat, meta) {
		t.Fatalf("expected marker to satisfy legacy sync state, chatUpdatedAt=%d metaUpdatedAt=%d", chat.UpdatedAt, meta.UpdatedAt.UnixMilli())
	}
}

func TestWorkspaceChatSnapshotLinksForkedClaudeLegacyChatBySessionToken(t *testing.T) {
	dir := t.TempDir()
	previousDataDir := workspaceDataDir
	previousCoordinator := workspaceCoordinator
	previousCoordinatorDir := workspaceCoordinatorDir
	workspaceDataDir = func() string { return filepath.Join(dir, "workspace-data") }
	workspaceCoordinator = nil
	workspaceCoordinatorDir = ""
	t.Cleanup(func() {
		workspaceDataDir = previousDataDir
		workspaceCoordinator = previousCoordinator
		workspaceCoordinatorDir = previousCoordinatorDir
	})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	transcriptPath := filepath.Join(dir, "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699.jsonl")
	body := `{"type":"user","message":{"role":"user","content":"ببین این claude-flow چیه؟"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"دارم بررسی می‌کنم"},{"type":"tool_use","id":"call_1","name":"TaskCreate","input":{"description":"Locate claude-flow artifacts"}}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"Task #1 created successfully"}]}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "claude:1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		Agent:          "claude",
		SessionID:      "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		SessionName:    "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})

	project := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project
	chatID := "chat-forked-claude"
	store := workspaceStore()
	appendWorkspaceEvent(t, store, events.StreamProjects, events.TypeProjectOpened, 100, map[string]any{
		"projectId": project.ID,
		"localPath": project.LocalPath,
		"title":     project.Title,
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatCreated, 100, map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Fork",
	})
	appendWorkspaceEvent(t, store, events.StreamChats, events.TypeChatProviderSet, 101, map[string]any{
		"chatId":   chatID,
		"provider": "claude",
	})
	appendWorkspaceEvent(t, store, events.StreamTurns, events.TypeSessionTokenSet, 101, map[string]any{
		"chatId":       chatID,
		"sessionToken": "1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699",
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 102, map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "bad-user",
			"kind":      transcript.KindUserPrompt,
			"messageId": "evt_1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699_1",
			"createdAt": float64(102),
			"content":   "ببین این claude-flow چیه؟",
		},
	})
	appendWorkspaceEvent(t, store, events.StreamMessages, events.TypeMessageAppended, 103, map[string]any{
		"chatId": chatID,
		"entry": readmodels.TranscriptEntry{
			"_id":       "bad-tool-name",
			"kind":      transcript.KindAssistantText,
			"messageId": "evt_1efa1ee2-3f6f-4093-9e3f-cd1e7fa3a699_2",
			"createdAt": float64(103),
			"text":      "TaskCreate",
		},
	})

	snapshot := workspaceChatSnapshot(chatID, 20).(*readmodels.ChatSnapshot)
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected forked native runtime to ignore stored messages, got %#v", snapshot.Messages)
	}
	if snapshot.Runtime.TmuxSession == "" || snapshot.Runtime.NativeSessionID != meta.SessionID || snapshot.Runtime.NativeTranscriptPath != transcriptPath {
		t.Fatalf("expected forked chat native runtime metadata, got %#v", snapshot.Runtime)
	}
}
