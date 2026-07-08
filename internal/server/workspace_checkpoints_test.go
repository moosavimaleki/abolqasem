package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/readmodels"
)

func TestWorkspaceCheckpointRestoreCodeAndChat(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := newGitProject(t)
	filePath := filepath.Join(projectDir, "app.txt")
	if err := os.WriteFile(filePath, []byte("checkpoint\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatal(err)
	}
	store := &workspaceEventStore{store: workspaceStore()}
	chatID := "chat-checkpoint-legacy"
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatCreated, time.Now().UnixMilli(), map[string]any{
		"chatId":    chatID,
		"projectId": project.ID,
		"title":     "Legacy checkpoint chat",
	}); err != nil {
		t.Fatal(err)
	}
	chat, err := store.RequireChat(chatID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendUserPrompt(chat.ID, "first", nil, false); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := workspaceCreateCheckpoint(workspaceCreateCheckpointArgs{
		ChatID:        chat.ID,
		Trigger:       workspaceCheckpointTriggerPrompt,
		PromptPreview: "change the file",
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Code.Kind != "git" || checkpoint.Code.Commit == "" {
		t.Fatalf("expected git checkpoint, got %#v", checkpoint.Code)
	}
	if err := store.AppendUserPrompt(chat.ID, "second", nil, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command, _ := json.Marshal(map[string]any{
		"chatId":       chat.ID,
		"checkpointId": checkpoint.ID,
		"mode":         workspaceCheckpointModeBoth,
	})
	result, projectID, err := workspaceRestoreCheckpoint(command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.ChatRestored || projectID != project.ID {
		t.Fatalf("unexpected restore result: %#v projectID=%q", result, projectID)
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "checkpoint\n" {
		t.Fatalf("expected checkpoint file content, got %q", string(content))
	}
	messages, err := workspaceChatMessages(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected checkpoint restore to avoid chat message storage, got %#v", messages)
	}
	snapshot := workspaceChatSnapshot(chat.ID, 10).(*readmodels.ChatSnapshot)
	if len(snapshot.Messages) != 0 {
		t.Fatalf("expected snapshot to avoid restored chat transcript, got %#v", snapshot.Messages)
	}
	if summaries := workspaceListCheckpointsForProject(project.ID); len(summaries) != 2 {
		t.Fatalf("expected original plus safety checkpoint, got %#v", summaries)
	}
}

func TestWorkspaceCoordinatorCreatesCheckpointBeforePrompt(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := newGitProject(t)
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatal(err)
	}
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspaceAgentCoordinator().Send(context.Background(), agent.SendCommand{
		ChatID:  chat.ID,
		Content: "implement this",
	}); err != nil {
		t.Fatal(err)
	}
	summaries := workspaceListCheckpointsForProject(project.ID)
	if len(summaries) != 1 {
		t.Fatalf("expected one checkpoint, got %#v", summaries)
	}
	if summaries[0].PromptPreview != "implement this" || summaries[0].ChatID != chat.ID {
		t.Fatalf("unexpected checkpoint summary: %#v", summaries[0])
	}
}

func TestWorkspaceCheckpointForTmuxChatSkipsTranscriptSnapshot(t *testing.T) {
	withWorkspaceComposerStore(t)
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatal(err)
	}
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chat.TmuxSession == "" {
		t.Fatalf("expected tmux chat, got %#v", chat)
	}
	if err := os.WriteFile(filepath.Join(workspaceDataDir(), "messages.jsonl"), []byte("{bad json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := workspaceCreateCheckpoint(workspaceCreateCheckpointArgs{
		ChatID:        chat.ID,
		Trigger:       workspaceCheckpointTriggerPrompt,
		PromptPreview: "tmux prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.ChatSnapshotPath != "" || checkpoint.ChatMessageCount != 0 || len(checkpoint.ChatMessageIDs) != 0 {
		t.Fatalf("expected tmux checkpoint to store only metadata/code, got %#v", checkpoint)
	}
	if _, err := os.Stat(workspaceCheckpointMessagesPath(checkpoint.ID)); !os.IsNotExist(err) {
		t.Fatalf("expected no transcript snapshot file, stat err=%v", err)
	}

	command, _ := json.Marshal(map[string]any{
		"chatId":       chat.ID,
		"checkpointId": checkpoint.ID,
		"mode":         workspaceCheckpointModeChat,
	})
	result, _, err := workspaceRestoreCheckpoint(command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChatRestored {
		t.Fatalf("expected metadata-only checkpoint restore to skip chat transcript restore, got %#v", result)
	}
}

func TestWorkspaceFilesystemCheckpointSkipsLocalToolingDirs(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignoredDirs := []string{".antigravity", ".venv", "venv", ".idea", ".vscode"}
	for _, dir := range ignoredDirs {
		nested := filepath.Join(projectDir, dir)
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "ignored.txt"), []byte("ignore"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	filesDir := filepath.Join(t.TempDir(), "files")
	code, err := workspaceCreateFilesystemCodeCheckpoint(projectDir, filesDir)
	if err != nil {
		t.Fatal(err)
	}
	if code.FileCount != 1 {
		t.Fatalf("expected only keep.txt in filesystem checkpoint, got %#v", code)
	}
	if _, err := os.Stat(filepath.Join(filesDir, "keep.txt")); err != nil {
		t.Fatalf("expected keep.txt in checkpoint: %v", err)
	}
	for _, dir := range ignoredDirs {
		if _, err := os.Stat(filepath.Join(filesDir, dir, "ignored.txt")); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be ignored, stat err=%v", dir, err)
		}
	}
}

func newGitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "core.autocrlf", "false")
	runGit(t, dir, "config", "core.eol", "lf")
	if err := os.WriteFile(filepath.Join(dir, "app.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "-c", "user.name=Abolqasem Test", "-c", "user.email=abolqasem@example.invalid", "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
