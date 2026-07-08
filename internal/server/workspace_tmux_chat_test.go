package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/legacyimport"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/tmuxruntime"
	"abolqasem/internal/workspace/transcript"
)

func TestWorkspaceSendTmuxChatCreatesSessionAndSendsPrompt(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	withWorkspaceComposerStore(t)
	t.Setenv("ABOLQASEM_TMUX_CODEX_COMMAND", "sh")

	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	result, handled, err := workspaceSendTmuxChat(agent.SendCommand{
		ProjectID: project.ID,
		Content:   "printf tmux-send-ok\\n",
		Provider:  "codex",
	})
	if err != nil {
		t.Fatalf("workspaceSendTmuxChat returned error: %v", err)
	}
	if !handled || result.ChatID == "" {
		t.Fatalf("expected tmux send to handle new chat, handled=%v result=%#v", handled, result)
	}

	state, err := workspaceStore().LoadStateLight()
	if err != nil {
		t.Fatalf("LoadStateLight returned error: %v", err)
	}
	chat := state.ChatsByID[result.ChatID]
	if chat.TmuxSession == "" {
		t.Fatalf("expected chat tmux session, got %#v", chat)
	}
	messageEvents, err := workspaceStore().Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay messages returned error: %v", err)
	}
	if len(messageEvents) != 0 {
		t.Fatalf("expected tmux-first send to avoid message events, got %#v", messageEvents)
	}
	defer exec.Command("tmux", "kill-session", "-t", chat.TmuxSession).Run()

	var output string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		output, _ = tmuxruntime.Capture(context.Background(), chat.TmuxSession, 40)
		if strings.Contains(output, "tmux-send-ok") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected tmux output to contain prompt result, got:\n%s", output)
}

func TestWorkspaceSendTmuxChatFallsBackForLegacyRecordWithoutTmuxSession(t *testing.T) {
	withWorkspaceComposerStore(t)
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chatID := "chat-legacy-no-tmux"
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatCreated, time.Now().UnixMilli(), map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Legacy",
	}); err != nil {
		t.Fatalf("appendWorkspaceStoreEvent returned error: %v", err)
	}

	_, handled, err := workspaceSendTmuxChat(agent.SendCommand{ChatID: chatID, Content: "hello"})
	if err != nil {
		t.Fatalf("workspaceSendTmuxChat returned error: %v", err)
	}
	if handled {
		t.Fatal("expected legacy chat without tmuxSession to fall back to old coordinator")
	}
}

func TestWorkspaceResolveTmuxProviderCommandUsesWorkingBunCodex(t *testing.T) {
	providerexec.SetConfiguredExecutables(nil)
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setWorkspaceTestHome(t, home)
	codexPath := filepath.Join(home, ".bun", "bin", workspaceTestExecutableName("codex"))
	writeWorkspaceExecutable(t, codexPath, "#!/bin/sh\nexit 0\n")
	providerexec.SetConfiguredExecutables(map[string]string{"codex": codexPath})

	command := workspaceResolveTmuxProviderCommand("codex", "codex --sandbox workspace-write")
	if command != codexPath+" --sandbox workspace-write" {
		t.Fatalf("expected bun codex command, got %q", command)
	}
}

func TestWorkspaceResolveTmuxProviderCommandUsesWorkingLocalClaude(t *testing.T) {
	providerexec.SetConfiguredExecutables(nil)
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setWorkspaceTestHome(t, home)
	localClaudePath := filepath.Join(home, ".local", "bin", workspaceTestExecutableName("claude"))
	writeWorkspaceExecutable(t, localClaudePath, "#!/bin/sh\nexit 0\n")
	writeWorkspaceExecutable(t, filepath.Join(home, ".bun", "bin", workspaceTestExecutableName("claude")), "#!/bin/sh\nexit 1\n")
	providerexec.SetConfiguredExecutables(map[string]string{"claude": localClaudePath})

	command := workspaceResolveTmuxProviderCommand("claude", "claude")
	if command != localClaudePath {
		t.Fatalf("expected local claude command, got %q", command)
	}
}

func TestWorkspaceTmuxCommandForChatDefaultsCodexProviderAndResumes(t *testing.T) {
	providerexec.SetConfiguredExecutables(nil)
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setWorkspaceTestHome(t, home)
	codexPath := filepath.Join(home, ".bun", "bin", workspaceTestExecutableName("codex"))
	writeWorkspaceExecutable(t, codexPath, "#!/bin/sh\nexit 0\n")
	providerexec.SetConfiguredExecutables(map[string]string{"codex": codexPath})
	t.Setenv("ABOLQASEM_TMUX_CODEX_COMMAND", "codex --sandbox workspace-write")

	command := workspaceTmuxCommandForChat(readmodels.ChatRecord{
		NativeSessionID:      "codex-session-1",
		NativeTranscriptPath: filepath.Join(home, ".codex", "sessions", "2026", "07", "04", "rollout.jsonl"),
	}, "")

	if !strings.HasSuffix(command, " --sandbox workspace-write resume 'codex-session-1'") {
		t.Fatalf("expected codex resume command, got %q", command)
	}
}

func TestWorkspaceTmuxCommandForChatInfersClaudeProviderFromTranscriptPath(t *testing.T) {
	providerexec.SetConfiguredExecutables(nil)
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setWorkspaceTestHome(t, home)
	claudePath := filepath.Join(home, ".local", "bin", workspaceTestExecutableName("claude"))
	writeWorkspaceExecutable(t, claudePath, "#!/bin/sh\nexit 0\n")
	providerexec.SetConfiguredExecutables(map[string]string{"claude": claudePath})
	t.Setenv("ABOLQASEM_TMUX_CLAUDE_COMMAND", "claude --permission-mode auto")

	command := workspaceTmuxCommandForChat(readmodels.ChatRecord{
		NativeSessionID:      "claude-session-1",
		NativeTranscriptPath: filepath.Join(home, ".claude", "projects", "project", "session.jsonl"),
	}, "")

	if !strings.HasSuffix(command, " --permission-mode auto --resume 'claude-session-1'") {
		t.Fatalf("expected claude resume command, got %q", command)
	}
}

func TestWorkspaceTmuxCommandForChatInfersGeminiProviderFromTranscriptPath(t *testing.T) {
	providerexec.SetConfiguredExecutables(nil)
	t.Cleanup(func() { providerexec.SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setWorkspaceTestHome(t, home)
	geminiPath := filepath.Join(home, ".bun", "bin", workspaceTestExecutableName("gemini"))
	writeWorkspaceExecutable(t, geminiPath, "#!/bin/sh\nexit 0\n")
	providerexec.SetConfiguredExecutables(map[string]string{"gemini": geminiPath})
	t.Setenv("ABOLQASEM_TMUX_GEMINI_COMMAND", "gemini --model gemini-3-pro")

	command := workspaceTmuxCommandForChat(readmodels.ChatRecord{
		NativeSessionID:      "gemini-session-1",
		NativeTranscriptPath: filepath.Join(home, ".gemini", "tmp", "session.json"),
	}, "")

	if !strings.HasSuffix(command, " --model gemini-3-pro --resume 'gemini-session-1'") {
		t.Fatalf("expected gemini resume command, got %q", command)
	}
}

func TestWorkspaceMigrateChatsToTmuxArchivesEventstoreTranscript(t *testing.T) {
	withWorkspaceComposerStore(t)
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chatID := "chat-eventstore-old"
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatCreated, time.Now().UnixMilli(), map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Old eventstore chat",
	}); err != nil {
		t.Fatalf("append chat created returned error: %v", err)
	}
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamMessages, events.TypeMessageAppended, time.Now().UnixMilli(), map[string]any{
		"chatId": chatID,
		"entry": transcript.New(transcript.KindUserPrompt, map[string]any{
			"content": "old prompt",
		}),
	}); err != nil {
		t.Fatalf("append user prompt returned error: %v", err)
	}
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamMessages, events.TypeMessageAppended, time.Now().UnixMilli(), map[string]any{
		"chatId": chatID,
		"entry": transcript.New(transcript.KindAssistantText, map[string]any{
			"text": "assistant reply",
		}),
	}); err != nil {
		t.Fatalf("append assistant text returned error: %v", err)
	}
	if err := (&workspaceEventStore{store: workspaceStore()}).AppendTranscriptEntry(chatID, transcript.New(transcript.KindUserPrompt, map[string]any{
		"content": "old prompt",
	})); err != nil {
		t.Fatalf("AppendTranscriptEntry returned error: %v", err)
	}

	dryRun, err := workspaceMigrateChatsToTmux(json.RawMessage(`{"chatIds":["chat-eventstore-old"],"dryRun":true}`))
	if err != nil {
		t.Fatalf("dry run migration returned error: %v", err)
	}
	if dryRun.MigratedCount != 1 || dryRun.Compacted {
		t.Fatalf("unexpected dry run result: %#v", dryRun)
	}
	stateBefore, err := workspaceStore().LoadStateLight()
	if err != nil {
		t.Fatalf("LoadStateLight before migration returned error: %v", err)
	}
	if stateBefore.ChatsByID[chatID].TmuxSession != "" {
		t.Fatalf("dry run should not write tmux metadata, got %#v", stateBefore.ChatsByID[chatID])
	}

	result, err := workspaceMigrateChatsToTmux(json.RawMessage(`{"chatIds":["chat-eventstore-old"]}`))
	if err != nil {
		t.Fatalf("workspaceMigrateChatsToTmux returned error: %v", err)
	}
	if result.MigratedCount != 1 || !result.Compacted {
		t.Fatalf("unexpected migration result: %#v", result)
	}
	stateAfter, err := workspaceStore().LoadStateLight()
	if err != nil {
		t.Fatalf("LoadStateLight after migration returned error: %v", err)
	}
	chat := stateAfter.ChatsByID[chatID]
	if chat.TmuxSession != workspaceChatTmuxSession(chatID) || chat.LastSummary != "assistant reply" {
		t.Fatalf("expected tmux metadata and summary, got %#v", chat)
	}
	messageEvents, err := workspaceStore().Replay(events.StreamMessages)
	if err != nil {
		t.Fatalf("Replay messages returned error: %v", err)
	}
	if len(messageEvents) != 0 {
		t.Fatalf("expected active messages stream to be empty after migration compact, got %#v", messageEvents)
	}
	archives, err := filepath.Glob(filepath.Join(workspaceDataDir(), "messages.jsonl.archived-*"))
	if err != nil {
		t.Fatalf("glob archives returned error: %v", err)
	}
	if len(archives) == 0 {
		t.Fatal("expected migration compact to archive old messages.jsonl")
	}
}

func TestWorkspaceHookSyncsNativeRuntimeToTmuxChat(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := t.TempDir()
	project, err := workspaceOpenProject(projectDir, "Project")
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
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatRuntimeSet, time.Now().UnixMilli(), map[string]any{
		"chatId":      chat.ID,
		"lastSummary": "hello from tmux",
	}); err != nil {
		t.Fatalf("append runtime event returned error: %v", err)
	}

	meta := state.SessionMeta{
		Agent:          "codex",
		SessionID:      "native-session-1",
		TranscriptPath: "/tmp/native-session-1.jsonl",
		Cwd:            projectDir,
		ProjectName:    "Project",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	chatID, synced, err := workspaceSyncTmuxRuntimeFromHook(meta, state.HookEvent{
		Agent:         "codex",
		HookEventName: "UserPromptSubmit",
		PromptPreview: "hello from tmux",
	})
	if err != nil {
		t.Fatalf("workspaceSyncTmuxRuntimeFromHook returned error: %v", err)
	}
	if !synced || chatID != chat.ID {
		t.Fatalf("expected hook to sync tmux chat %q, synced=%v chatID=%q", chat.ID, synced, chatID)
	}

	stateSnapshot, err := workspaceStore().LoadStateLight()
	if err != nil {
		t.Fatalf("LoadStateLight returned error: %v", err)
	}
	updated := stateSnapshot.ChatsByID[chat.ID]
	if updated.NativeSessionID != meta.SessionID || updated.NativeTranscriptPath != meta.TranscriptPath {
		t.Fatalf("expected native runtime metadata, got %#v", updated)
	}
	if updated.SessionToken == nil || *updated.SessionToken != meta.SessionID {
		t.Fatalf("expected session token to be linked, got %#v", updated.SessionToken)
	}
	if _, ok := stateSnapshot.ChatsByID[legacyimport.ImportedChatID(meta)]; ok {
		t.Fatalf("expected hook sync to avoid creating legacy materialized chat")
	}
}

func writeWorkspaceExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	exitCode := 0
	if strings.Contains(content, "exit 1") {
		exitCode = 1
	}
	sourcePath := filepath.Join(t.TempDir(), "main.go")
	source := fmt.Sprintf("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(%d) }\n", exitCode)
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile source returned error: %v", err)
	}
	command := exec.Command("go", "build", "-o", path, sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build fake executable failed: %v\n%s", err, output)
	}
}

func setWorkspaceTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func workspaceTestExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
