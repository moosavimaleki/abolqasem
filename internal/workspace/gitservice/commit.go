package gitservice

import (
	"context"
	"strings"
)

const (
	CommitOnly    = "commit_only"
	CommitAndPush = "commit_and_push"

	SyncFetch = "fetch"
	SyncPull  = "pull"
	SyncPush  = "push"
)

type CommitRequest struct {
	Paths       []string
	Summary     string
	Description string
	Mode        string
}

type CommitResult struct {
	OK                 bool   `json:"ok"`
	BranchName         string `json:"branchName,omitempty"`
	SnapshotChanged    bool   `json:"snapshotChanged"`
	Mode               string `json:"mode"`
	Pushed             bool   `json:"pushed,omitempty"`
	Title              string `json:"title,omitempty"`
	Message            string `json:"message,omitempty"`
	Detail             string `json:"detail,omitempty"`
	Cancelled          bool   `json:"cancelled,omitempty"`
	Phase              string `json:"phase,omitempty"`
	LocalCommitCreated bool   `json:"localCommitCreated,omitempty"`
}

type SyncResult struct {
	OK              bool   `json:"ok"`
	Action          string `json:"action"`
	BranchName      string `json:"branchName,omitempty"`
	SnapshotChanged bool   `json:"snapshotChanged"`
	Title           string `json:"title,omitempty"`
	Message         string `json:"message,omitempty"`
	Detail          string `json:"detail,omitempty"`
	Cancelled       bool   `json:"cancelled,omitempty"`
}

func Commit(ctx context.Context, localPath string, request CommitRequest) (CommitResult, error) {
	mode := normalizeCommitMode(request.Mode)
	result := CommitResult{Mode: mode}
	summary := strings.TrimSpace(request.Summary)
	paths := cleanPaths(request.Paths)
	if summary == "" {
		return commitFailure(result, "commit", "Commit message required", "Enter a summary before committing.", "", false), nil
	}
	if len(paths) == 0 {
		return commitFailure(result, "commit", "No files selected", "Select at least one file to commit.", "", false), nil
	}

	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return CommitResult{}, err
	}
	result.BranchName = snapshot.BranchName
	if snapshot.Status != StatusReady {
		return commitFailure(result, "commit", "Git repository not ready", "Initialize git before committing.", "", false), nil
	}

	if output, err := gitOutput(ctx, snapshot.RepositoryRoot, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return commitFailure(result, "commit", "Stage files failed", err.Error(), output, false), nil
	}

	commitArgs := []string{"commit", "--only", "-m", summary}
	if description := strings.TrimSpace(request.Description); description != "" {
		commitArgs = append(commitArgs, "-m", description)
	}
	commitArgs = append(commitArgs, "--")
	commitArgs = append(commitArgs, paths...)
	if output, err := gitOutput(ctx, snapshot.RepositoryRoot, commitArgs...); err != nil {
		return commitFailure(result, "commit", "Commit failed", err.Error(), output, false), nil
	}
	result.SnapshotChanged = true

	if mode != CommitAndPush {
		result.OK = true
		return result, nil
	}
	if pushResult := pushCurrentBranch(ctx, snapshot.RepositoryRoot, snapshot.BranchName); !pushResult.OK {
		return commitFailure(result, "push", pushResult.Title, pushResult.Message, pushResult.Detail, true), nil
	}
	result.OK = true
	result.Pushed = true
	return result, nil
}

func Sync(ctx context.Context, localPath string, action string) (SyncResult, error) {
	action = strings.TrimSpace(action)
	if action == "" {
		action = SyncFetch
	}
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{
		Action:          action,
		BranchName:      snapshot.BranchName,
		SnapshotChanged: true,
	}
	if snapshot.Status != StatusReady {
		return syncFailure(result, "Git repository not ready", "Initialize git before syncing.", ""), nil
	}
	switch action {
	case SyncFetch:
		if output, err := gitOutput(ctx, snapshot.RepositoryRoot, "fetch", "--prune"); err != nil {
			return syncFailure(result, "Fetch failed", err.Error(), output), nil
		}
	case SyncPull:
		if output, err := gitOutput(ctx, snapshot.RepositoryRoot, "pull", "--ff-only"); err != nil {
			return syncFailure(result, "Pull failed", err.Error(), output), nil
		}
	case SyncPush:
		return pushCurrentBranch(ctx, snapshot.RepositoryRoot, snapshot.BranchName), nil
	default:
		return syncFailure(result, "Unsupported sync action", "Unsupported sync action: "+action, ""), nil
	}
	result.OK = true
	return result, nil
}

func pushCurrentBranch(ctx context.Context, root string, branchName string) SyncResult {
	result := SyncResult{
		Action:          SyncPush,
		BranchName:      branchName,
		SnapshotChanged: true,
	}
	if branchName == "" {
		return syncFailure(result, "Push failed", "Cannot push without a current branch.", "")
	}
	if _, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		if output, err := gitOutput(ctx, root, "push"); err != nil {
			return syncFailure(result, "Push failed", err.Error(), output)
		}
		result.OK = true
		return result
	}
	if output, err := gitOutput(ctx, root, "push", "-u", "origin", branchName); err != nil {
		return syncFailure(result, "Push failed", err.Error(), output)
	}
	result.OK = true
	return result
}

func normalizeCommitMode(mode string) string {
	if strings.TrimSpace(mode) == CommitAndPush {
		return CommitAndPush
	}
	return CommitOnly
}

func cleanPaths(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(strings.Trim(path, `/\`))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		cleaned = append(cleaned, path)
	}
	return cleaned
}

func commitFailure(result CommitResult, phase string, title string, message string, detail string, localCommitCreated bool) CommitResult {
	result.OK = false
	result.Title = title
	result.Message = message
	result.Detail = detail
	result.Phase = phase
	result.LocalCommitCreated = localCommitCreated
	return result
}

func syncFailure(result SyncResult, title string, message string, detail string) SyncResult {
	result.OK = false
	result.Title = title
	result.Message = message
	result.Detail = detail
	return result
}
