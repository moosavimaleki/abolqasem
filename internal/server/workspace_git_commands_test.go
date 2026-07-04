package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/gitservice"
	"ai-agent-manager/internal/workspace/legacyimport"
)

func TestWorkspaceListBranchesResolvesLegacyChatProject(t *testing.T) {
	withWorkspaceComposerStore(t)

	projectDir := t.TempDir()
	meta := state.SessionMeta{
		Key:            "codex:legacy-git",
		Agent:          "codex",
		SessionID:      "legacy-git",
		TranscriptPath: filepath.Join(projectDir, "rollout.jsonl"),
		Cwd:            projectDir,
		ProjectName:    "Legacy Git",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})
	chatID := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Chat.ID

	raw, err := json.Marshal(map[string]any{"chatId": chatID})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	result, err := workspaceListBranches(raw)
	if err != nil {
		t.Fatalf("workspaceListBranches returned error: %v", err)
	}
	if result.Recent == nil || result.Local == nil || result.Remote == nil || result.PullRequests == nil {
		t.Fatalf("expected initialized branch lists, got %#v", result)
	}
}

func TestWorkspaceProjectGitSnapshotResolvesLegacyProject(t *testing.T) {
	withWorkspaceComposerStore(t)

	projectDir := t.TempDir()
	meta := state.SessionMeta{
		Key:            "codex:legacy-git-snapshot",
		Agent:          "codex",
		SessionID:      "legacy-git-snapshot",
		TranscriptPath: filepath.Join(projectDir, "rollout.jsonl"),
		Cwd:            projectDir,
		ProjectName:    "Legacy Git Snapshot",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})
	projectID := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project.ID

	snapshot, ok := workspaceProjectGitSnapshot(projectID).(gitservice.Snapshot)
	if !ok {
		t.Fatalf("expected git snapshot, got %#v", snapshot)
	}
	if snapshot.Status == gitservice.StatusUnknown {
		t.Fatalf("expected legacy project path to be resolved before git detect, got %#v", snapshot)
	}
}

func TestWorkspaceProjectGitSubscriptionSnapshotIsLightweight(t *testing.T) {
	withWorkspaceComposerStore(t)

	projectDir := t.TempDir()
	meta := state.SessionMeta{
		Key:            "codex:legacy-git-light",
		Agent:          "codex",
		SessionID:      "legacy-git-light",
		TranscriptPath: filepath.Join(projectDir, "rollout.jsonl"),
		Cwd:            projectDir,
		ProjectName:    "Legacy Git Light",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})
	projectID := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project.ID

	snapshot, ok := workspaceProjectGitSubscriptionSnapshot(projectID).(gitservice.Snapshot)
	if !ok {
		t.Fatalf("expected git snapshot, got %#v", snapshot)
	}
	if snapshot.Status != gitservice.StatusUnknown {
		t.Fatalf("subscription snapshot should not run git detect, got %#v", snapshot)
	}
	if snapshot.Files == nil || snapshot.BranchHistory.Entries == nil {
		t.Fatalf("expected initialized empty fields, got %#v", snapshot)
	}
}

func TestWorkspaceProjectGitSubscriptionSnapshotUsesCachedSnapshot(t *testing.T) {
	withWorkspaceComposerStore(t)
	originalCache := workspaceProjectGitSnapshots
	workspaceProjectGitSnapshots = newWorkspaceProjectGitSnapshotCache()
	t.Cleanup(func() { workspaceProjectGitSnapshots = originalCache })

	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()
	runGit(t, projectDir, "init")
	if err := os.WriteFile(filepath.Join(projectDir, "app.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write app.txt failed: %v", err)
	}
	projectID := mustCreateWorkspaceProject(t, conn, projectDir)
	chatID := mustCreateWorkspaceChat(t, conn, projectID)
	raw, err := json.Marshal(map[string]any{"chatId": chatID})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	snapshot, returnedProjectID, changed, err := workspaceRefreshDiffs(raw)
	if err != nil {
		t.Fatalf("workspaceRefreshDiffs returned error: %v", err)
	}
	if returnedProjectID != projectID || !changed {
		t.Fatalf("expected changed refresh snapshot, project=%q changed=%t", returnedProjectID, changed)
	}

	cached, ok := workspaceProjectGitSubscriptionSnapshot(projectID).(gitservice.Snapshot)
	if !ok {
		t.Fatalf("expected git snapshot, got %#v", cached)
	}
	if cached.Status == gitservice.StatusUnknown {
		t.Fatalf("expected cached subscription snapshot to keep loaded git data, got %#v", cached)
	}
	if len(cached.Files) != len(snapshot.Files) {
		t.Fatalf("expected cached files to match refreshed snapshot, cached=%#v snapshot=%#v", cached, snapshot)
	}
	if cached.BranchName != snapshot.BranchName {
		t.Fatalf("expected cached branch name to match refreshed snapshot, cached=%#v snapshot=%#v", cached, snapshot)
	}
}

func TestWorkspaceRefreshDiffsReportsOnlyRealSnapshotChanges(t *testing.T) {
	withWorkspaceComposerStore(t)
	originalCache := workspaceProjectGitSnapshots
	workspaceProjectGitSnapshots = newWorkspaceProjectGitSnapshotCache()
	t.Cleanup(func() { workspaceProjectGitSnapshots = originalCache })

	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()
	runGit(t, projectDir, "init")
	if err := os.WriteFile(filepath.Join(projectDir, "app.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write app.txt failed: %v", err)
	}
	projectID := mustCreateWorkspaceProject(t, conn, projectDir)
	chatID := mustCreateWorkspaceChat(t, conn, projectID)
	raw, err := json.Marshal(map[string]any{"chatId": chatID})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	snapshot, returnedProjectID, changed, err := workspaceRefreshDiffs(raw)
	if err != nil {
		t.Fatalf("workspaceRefreshDiffs returned error: %v", err)
	}
	if returnedProjectID != projectID || !changed || len(snapshot.Files) != 1 {
		t.Fatalf("expected initial changed snapshot, project=%q changed=%t snapshot=%#v", returnedProjectID, changed, snapshot)
	}

	_, _, changed, err = workspaceRefreshDiffs(raw)
	if err != nil {
		t.Fatalf("workspaceRefreshDiffs returned error: %v", err)
	}
	if changed {
		t.Fatalf("expected unchanged snapshot on repeated refresh")
	}

	if err := os.WriteFile(filepath.Join(projectDir, "app.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write app.txt failed: %v", err)
	}
	_, _, changed, err = workspaceRefreshDiffs(raw)
	if err != nil {
		t.Fatalf("workspaceRefreshDiffs returned error: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed snapshot after file update")
	}
}

func TestWorkspaceRefreshDiffsFallsBackOnTimeout(t *testing.T) {
	withWorkspaceComposerStore(t)
	originalCache := workspaceProjectGitSnapshots
	workspaceProjectGitSnapshots = newWorkspaceProjectGitSnapshotCache()
	t.Cleanup(func() { workspaceProjectGitSnapshots = originalCache })

	originalDetect := workspaceGitDetect
	workspaceGitDetect = func(context.Context, string) (gitservice.Snapshot, error) {
		return gitservice.Snapshot{}, context.DeadlineExceeded
	}
	t.Cleanup(func() { workspaceGitDetect = originalDetect })

	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()
	runGit(t, projectDir, "init")
	projectID := mustCreateWorkspaceProject(t, conn, projectDir)
	chatID := mustCreateWorkspaceChat(t, conn, projectID)
	raw, err := json.Marshal(map[string]any{"chatId": chatID})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	snapshot, returnedProjectID, changed, err := workspaceRefreshDiffs(raw)
	if err != nil {
		t.Fatalf("workspaceRefreshDiffs returned error: %v", err)
	}
	if returnedProjectID != projectID || !changed {
		t.Fatalf("expected timeout fallback to mark snapshot changed, project=%q changed=%t snapshot=%#v", returnedProjectID, changed, snapshot)
	}
	if snapshot.Status != gitservice.StatusUnknown {
		t.Fatalf("expected unknown status on timeout fallback, got %#v", snapshot)
	}
	if snapshot.Warning == "" || !strings.Contains(snapshot.Warning, "timed out") {
		t.Fatalf("expected timeout warning, got %#v", snapshot.Warning)
	}
	if snapshot.Files == nil || snapshot.BranchHistory.Entries == nil {
		t.Fatalf("expected initialized empty slices, got %#v", snapshot)
	}
}

func TestWorkspaceRefreshDiffsKeepsCachedSnapshotOnTimeout(t *testing.T) {
	withWorkspaceComposerStore(t)
	originalCache := workspaceProjectGitSnapshots
	workspaceProjectGitSnapshots = newWorkspaceProjectGitSnapshotCache()
	t.Cleanup(func() { workspaceProjectGitSnapshots = originalCache })

	originalDetect := workspaceGitDetect
	workspaceGitDetect = func(context.Context, string) (gitservice.Snapshot, error) {
		return gitservice.Snapshot{}, context.DeadlineExceeded
	}
	t.Cleanup(func() { workspaceGitDetect = originalDetect })

	conn := newTestWorkspaceConnection(nil)
	projectDir := t.TempDir()
	runGit(t, projectDir, "init")
	if err := os.WriteFile(filepath.Join(projectDir, "app.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write app.txt failed: %v", err)
	}
	projectID := mustCreateWorkspaceProject(t, conn, projectDir)
	cachedSnapshot := gitservice.Snapshot{
		Status:        gitservice.StatusReady,
		Files:         []gitservice.DiffFile{{Path: "app.txt", ChangeType: "modified"}},
		BranchHistory: gitservice.BranchHistorySnapshot{Entries: []gitservice.BranchHistoryEntry{{SHA: "abc"}}},
	}
	workspaceProjectGitSnapshots.store(projectID, cachedSnapshot)
	chatID := mustCreateWorkspaceChat(t, conn, projectID)
	raw, err := json.Marshal(map[string]any{"chatId": chatID})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	snapshot, returnedProjectID, changed, err := workspaceRefreshDiffs(raw)
	if err != nil {
		t.Fatalf("workspaceRefreshDiffs returned error: %v", err)
	}
	if returnedProjectID != projectID || !changed {
		t.Fatalf("expected cached fallback to report change, project=%q changed=%t snapshot=%#v", returnedProjectID, changed, snapshot)
	}
	if snapshot.Status != gitservice.StatusReady {
		t.Fatalf("expected cached status to be preserved, got %#v", snapshot)
	}
	if snapshot.Warning == "" || !strings.Contains(snapshot.Warning, "last available") {
		t.Fatalf("expected cached warning, got %#v", snapshot.Warning)
	}
	if len(snapshot.Files) != 1 || snapshot.Files[0].Path != "app.txt" {
		t.Fatalf("expected cached files to be preserved, got %#v", snapshot.Files)
	}
}

func TestWorkspaceParseCommitMessageJSON(t *testing.T) {
	subject, body, err := workspaceParseCommitMessageJSON("```json\n{\"subject\":\"Add AI commit messages.\",\"body\":\"Uses a hidden provider session.\"}\n```")
	if err != nil {
		t.Fatalf("workspaceParseCommitMessageJSON returned error: %v", err)
	}
	if subject != "Add AI commit messages" || body != "Uses a hidden provider session." {
		t.Fatalf("unexpected commit message: subject=%q body=%q", subject, body)
	}
}

func TestWorkspaceParseCommitMessageJSONExtractsObject(t *testing.T) {
	subject, body, err := workspaceParseCommitMessageJSON("Here is the JSON:\n{\"message\":\"Update git panel\\n\\nGenerate commit messages with AI.\"}")
	if err != nil {
		t.Fatalf("workspaceParseCommitMessageJSON returned error: %v", err)
	}
	if subject != "Update git panel" || body != "Generate commit messages with AI." {
		t.Fatalf("unexpected commit message: subject=%q body=%q", subject, body)
	}
}
