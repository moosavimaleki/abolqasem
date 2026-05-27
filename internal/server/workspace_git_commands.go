package server

import (
	"context"
	"encoding/json"
	"strings"

	"ai-agent-manager/internal/workspace/gitservice"
	"ai-agent-manager/internal/workspace/readmodels"
)

func workspaceProjectGitSnapshot(projectID string) any {
	project, err := workspaceRuntimeProjectRequired(projectID)
	if err != nil {
		return workspaceProjectGitSnapshotWithCheckpoints("", gitservice.Snapshot{Status: gitservice.StatusUnknown})
	}
	snapshot, err := gitservice.Detect(context.Background(), project.LocalPath)
	if err != nil {
		snapshot = gitservice.Snapshot{Status: gitservice.StatusUnknown}
	}
	return workspaceProjectGitSnapshotWithCheckpoints(project.ID, snapshot)
}

func workspaceProjectGitSubscriptionSnapshot(projectID string) any {
	project, err := workspaceRuntimeProjectRequired(projectID)
	if err != nil {
		return workspaceProjectGitSnapshotWithCheckpoints("", gitservice.Snapshot{Status: gitservice.StatusUnknown})
	}
	return workspaceProjectGitSnapshotWithCheckpoints(project.ID, gitservice.Snapshot{Status: gitservice.StatusUnknown})
}

func workspaceProjectGitSnapshotWithCheckpoints(projectID string, snapshot gitservice.Snapshot) gitservice.Snapshot {
	if snapshot.Status == "" {
		snapshot.Status = gitservice.StatusUnknown
	}
	if snapshot.Files == nil {
		snapshot.Files = []gitservice.DiffFile{}
	}
	if snapshot.BranchHistory.Entries == nil {
		snapshot.BranchHistory = gitservice.BranchHistorySnapshot{Entries: []gitservice.BranchHistoryEntry{}}
	}
	snapshot.Checkpoints = workspaceListCheckpointsForProject(projectID)
	return snapshot
}

func workspaceReadDiffPatch(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		ProjectID string `json:"projectId"`
		Path      string `json:"path"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	project, err := workspaceRuntimeProjectRequired(payload.ProjectID)
	if err != nil {
		return nil, err
	}
	patch, err := gitservice.ReadPatch(context.Background(), project.LocalPath, payload.Path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"patch": patch}, nil
}

func workspaceInitGit(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	_, project, err := workspaceChatProjectFromRaw(raw)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	result, err := gitservice.Init(context.Background(), project.LocalPath)
	return result, project.ID, err
}

func workspaceRefreshDiffs(raw json.RawMessage) (gitservice.Snapshot, string, error) {
	_, project, err := workspaceChatProjectFromRaw(raw)
	if err != nil {
		return gitservice.Snapshot{}, "", err
	}
	snapshot, err := gitservice.Detect(context.Background(), project.LocalPath)
	if err != nil {
		return gitservice.Snapshot{}, project.ID, err
	}
	return workspaceProjectGitSnapshotWithCheckpoints(project.ID, snapshot), project.ID, nil
}

func workspaceGetGitHubPublishInfo(raw json.RawMessage) (gitservice.GitHubPublishInfo, error) {
	_, project, err := workspaceChatProjectFromRaw(raw)
	if err != nil {
		return gitservice.GitHubPublishInfo{}, err
	}
	return gitservice.GetGitHubPublishInfo(context.Background(), project.LocalPath)
}

func workspaceCheckGitHubRepoAvailability(raw json.RawMessage) (gitservice.GitHubRepoAvailabilityResult, error) {
	var payload struct {
		ChatID string `json:"chatId"`
		Owner  string `json:"owner"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.GitHubRepoAvailabilityResult{}, err
	}
	if _, _, err := workspaceGitChatProjectRequired(payload.ChatID); err != nil {
		return gitservice.GitHubRepoAvailabilityResult{}, err
	}
	return gitservice.CheckGitHubRepoAvailability(context.Background(), payload.Owner, payload.Name)
}

func workspacePublishToGitHub(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	var payload struct {
		ChatID      string `json:"chatId"`
		Owner       string `json:"owner"`
		Name        string `json:"name"`
		Visibility  string `json:"visibility"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	result, err := gitservice.PublishToGitHub(context.Background(), project.LocalPath, payload.Owner, payload.Name, payload.Visibility, payload.Description)
	return result, project.ID, err
}

func workspaceListBranches(raw json.RawMessage) (gitservice.BranchListResult, error) {
	_, project, err := workspaceChatProjectFromRaw(raw)
	if err != nil {
		return gitservice.BranchListResult{}, err
	}
	return gitservice.ListBranches(context.Background(), project.LocalPath)
}

func workspacePreviewMergeBranch(raw json.RawMessage) (gitservice.MergePreviewResult, error) {
	var payload struct {
		ChatID string          `json:"chatId"`
		Branch workspaceBranch `json:"branch"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.MergePreviewResult{}, err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.MergePreviewResult{}, err
	}
	return gitservice.PreviewMergeBranch(context.Background(), project.LocalPath, payload.Branch.Target())
}

func workspaceMergeBranch(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	var payload struct {
		ChatID string          `json:"chatId"`
		Branch workspaceBranch `json:"branch"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	result, err := gitservice.MergeBranch(context.Background(), project.LocalPath, payload.Branch.Target())
	return result, project.ID, err
}

func workspaceCheckoutBranch(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	var payload struct {
		ChatID string          `json:"chatId"`
		Branch workspaceBranch `json:"branch"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	var result gitservice.BranchActionResult
	if payload.Branch.Kind == "remote" || (payload.Branch.Kind == "pull_request" && strings.TrimSpace(payload.Branch.RemoteRef) != "") {
		result, err = gitservice.CheckoutRemoteTrackingBranch(context.Background(), project.LocalPath, payload.Branch.Target())
	} else {
		result, err = gitservice.CheckoutBranch(context.Background(), project.LocalPath, payload.Branch.Target())
	}
	return result, project.ID, err
}

func workspaceCreateBranch(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	var payload struct {
		ChatID         string `json:"chatId"`
		Name           string `json:"name"`
		BaseBranchName string `json:"baseBranchName"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	if strings.TrimSpace(payload.BaseBranchName) != "" {
		if result, err := gitservice.CheckoutBranch(context.Background(), project.LocalPath, payload.BaseBranchName); err != nil || !result.OK {
			return result, project.ID, err
		}
	}
	result, err := gitservice.CreateBranch(context.Background(), project.LocalPath, payload.Name)
	return result, project.ID, err
}

func workspaceSyncBranch(raw json.RawMessage) (gitservice.SyncResult, string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.SyncResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.SyncResult{}, "", err
	}
	result, err := gitservice.Sync(context.Background(), project.LocalPath, payload.Action)
	return result, project.ID, err
}

func workspaceGenerateCommitMessage(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		ChatID string   `json:"chatId"`
		Paths  []string `json:"paths"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceCommitMessageAITimeout)
	defer cancel()
	if subject, body, err := workspaceGenerateCommitMessageAI(ctx, project.LocalPath, payload.Paths); err == nil && strings.TrimSpace(subject) != "" {
		return map[string]any{"subject": subject, "body": body}, nil
	}
	subject, body := gitservice.GenerateCommitMessage(context.Background(), project.LocalPath, payload.Paths)
	return map[string]any{"subject": subject, "body": body}, nil
}

func workspaceCommitDiffs(raw json.RawMessage) (gitservice.CommitResult, string, error) {
	var payload struct {
		ChatID      string   `json:"chatId"`
		Paths       []string `json:"paths"`
		Summary     string   `json:"summary"`
		Description string   `json:"description"`
		Mode        string   `json:"mode"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.CommitResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.CommitResult{}, "", err
	}
	result, err := gitservice.Commit(context.Background(), project.LocalPath, gitservice.CommitRequest{
		Paths:       payload.Paths,
		Summary:     payload.Summary,
		Description: payload.Description,
		Mode:        payload.Mode,
	})
	return result, project.ID, err
}

func workspaceDiscardDiffFile(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	result, err := gitservice.DiscardDiffFile(context.Background(), project.LocalPath, payload.Path)
	return result, project.ID, err
}

func workspaceIgnoreDiffFile(raw json.RawMessage) (gitservice.BranchActionResult, string, error) {
	var payload struct {
		ChatID string `json:"chatId"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	if err != nil {
		return gitservice.BranchActionResult{}, "", err
	}
	result, err := gitservice.IgnoreDiffFile(context.Background(), project.LocalPath, payload.Path)
	return result, project.ID, err
}

type workspaceBranch struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	RemoteRef string `json:"remoteRef"`
}

func (b workspaceBranch) Target() string {
	if strings.TrimSpace(b.RemoteRef) != "" {
		return strings.TrimSpace(b.RemoteRef)
	}
	return strings.TrimSpace(b.Name)
}

func workspaceChatProjectFromRaw(raw json.RawMessage) (string, readmodels.ProjectRecord, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", readmodels.ProjectRecord{}, err
	}
	_, project, err := workspaceGitChatProjectRequired(payload.ChatID)
	return payload.ChatID, project, err
}

func workspaceGitChatProjectRequired(chatID string) (readmodels.ChatRecord, readmodels.ProjectRecord, error) {
	if chat, project, ok := workspaceLegacyChatProjectByID(chatID); ok && strings.TrimSpace(project.LocalPath) != "" {
		return chat, project, nil
	}
	return workspaceChatProjectRequired(chatID)
}
