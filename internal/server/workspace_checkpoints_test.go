package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ai-agent-manager/internal/workspace/agent"
	"ai-agent-manager/internal/workspace/readmodels"
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
	chat, err := workspaceCreateChat(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	store := &workspaceEventStore{store: workspaceStore()}
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
	if !result.OK || !result.ChatRestored || projectID != project.ID {
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
	if len(messages) != 1 {
		t.Fatalf("expected chat restored to one message, got %#v", messages)
	}
	snapshot := workspaceChatSnapshot(chat.ID, 10).(*readmodels.ChatSnapshot)
	if len(snapshot.Messages) != 1 {
		t.Fatalf("expected snapshot to reflect restored chat, got %#v", snapshot.Messages)
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

func newGitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
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
