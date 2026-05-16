package gitservice

import (
	"context"
	"strconv"
	"strings"
)

type BranchListEntry struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	RemoteRef   string `json:"remoteRef,omitempty"`
}

type BranchListResult struct {
	CurrentBranchName  string            `json:"currentBranchName,omitempty"`
	DefaultBranchName  string            `json:"defaultBranchName,omitempty"`
	Recent             []BranchListEntry `json:"recent"`
	Local              []BranchListEntry `json:"local"`
	Remote             []BranchListEntry `json:"remote"`
	PullRequests       []BranchListEntry `json:"pullRequests"`
	PullRequestsStatus string            `json:"pullRequestsStatus"`
	PullRequestsError  string            `json:"pullRequestsError,omitempty"`
}

type BranchActionResult struct {
	OK              bool   `json:"ok"`
	BranchName      string `json:"branchName,omitempty"`
	SnapshotChanged bool   `json:"snapshotChanged"`
	Title           string `json:"title,omitempty"`
	Message         string `json:"message,omitempty"`
	Detail          string `json:"detail,omitempty"`
	Cancelled       bool   `json:"cancelled,omitempty"`
}

type MergePreviewResult struct {
	CurrentBranchName string `json:"currentBranchName,omitempty"`
	TargetBranchName  string `json:"targetBranchName"`
	TargetDisplayName string `json:"targetDisplayName"`
	Status            string `json:"status"`
	CommitCount       int    `json:"commitCount"`
	HasConflicts      bool   `json:"hasConflicts"`
	Message           string `json:"message"`
	Detail            string `json:"detail,omitempty"`
}

func ListBranches(ctx context.Context, localPath string) (BranchListResult, error) {
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return BranchListResult{}, err
	}
	result := BranchListResult{
		CurrentBranchName:  snapshot.BranchName,
		DefaultBranchName:  snapshot.DefaultBranchName,
		Recent:             []BranchListEntry{},
		Local:              []BranchListEntry{},
		Remote:             []BranchListEntry{},
		PullRequests:       []BranchListEntry{},
		PullRequestsStatus: "unavailable",
	}
	if snapshot.Status != StatusReady {
		return result, nil
	}
	output, err := gitOutput(ctx, snapshot.RepositoryRoot, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)|%(committerdate:iso8601)", "refs/heads", "refs/remotes")
	if err != nil {
		return result, nil
	}
	for _, line := range strings.Split(output, "\n") {
		entry, ok := parseBranchLine(line)
		if !ok {
			continue
		}
		if entry.Kind == "remote" {
			result.Remote = append(result.Remote, entry)
		} else {
			result.Local = append(result.Local, entry)
			if len(result.Recent) < 5 {
				result.Recent = append(result.Recent, entry)
			}
		}
	}
	return result, nil
}

func CheckoutBranch(ctx context.Context, localPath string, branchName string) (BranchActionResult, error) {
	return branchCommand(ctx, localPath, []string{"checkout", strings.TrimSpace(branchName)}, true)
}

func CreateBranch(ctx context.Context, localPath string, branchName string) (BranchActionResult, error) {
	return branchCommand(ctx, localPath, []string{"checkout", "-b", strings.TrimSpace(branchName)}, true)
}

func PreviewMergeBranch(ctx context.Context, localPath string, targetBranch string) (MergePreviewResult, error) {
	targetBranch = strings.TrimSpace(targetBranch)
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return MergePreviewResult{}, err
	}
	result := MergePreviewResult{
		CurrentBranchName: snapshot.BranchName,
		TargetBranchName:  targetBranch,
		TargetDisplayName: targetBranch,
		Status:            "error",
		Message:           "Unable to preview merge.",
	}
	if snapshot.Status != StatusReady || targetBranch == "" {
		result.Detail = "Git repository is not ready or target branch is empty."
		return result, nil
	}
	result.CommitCount = countCommits(ctx, snapshot.RepositoryRoot, snapshot.BranchName+".."+targetBranch)
	if result.CommitCount == 0 {
		result.Status = "up_to_date"
		result.Message = "Already up to date."
		return result, nil
	}
	base, err := gitOutput(ctx, snapshot.RepositoryRoot, "merge-base", snapshot.BranchName, targetBranch)
	if err != nil {
		result.Detail = err.Error()
		return result, nil
	}
	mergeTree, err := gitOutput(ctx, snapshot.RepositoryRoot, "merge-tree", base, snapshot.BranchName, targetBranch)
	if err != nil {
		result.Detail = err.Error()
		return result, nil
	}
	if strings.Contains(mergeTree, "<<<<<<<") || strings.Contains(mergeTree, "changed in both") {
		result.Status = "conflicts"
		result.HasConflicts = true
		result.Message = "Merge has conflicts."
		result.Detail = mergeTree
		return result, nil
	}
	result.Status = "mergeable"
	result.Message = "Merge can be applied."
	return result, nil
}

func MergeBranch(ctx context.Context, localPath string, targetBranch string) (BranchActionResult, error) {
	return branchCommand(ctx, localPath, []string{"merge", "--no-edit", strings.TrimSpace(targetBranch)}, true)
}

func branchCommand(ctx context.Context, localPath string, args []string, snapshotChanged bool) (BranchActionResult, error) {
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return BranchActionResult{}, err
	}
	result := BranchActionResult{
		BranchName:      snapshot.BranchName,
		SnapshotChanged: snapshotChanged,
	}
	if snapshot.Status != StatusReady {
		return branchFailure(result, "Git repository not ready", "Initialize git before running branch actions.", ""), nil
	}
	if len(args) == 0 || strings.TrimSpace(args[len(args)-1]) == "" {
		return branchFailure(result, "Branch required", "Choose a branch first.", ""), nil
	}
	if output, err := gitOutput(ctx, snapshot.RepositoryRoot, args...); err != nil {
		return branchFailure(result, "Branch action failed", err.Error(), output), nil
	}
	updated, _ := Detect(ctx, snapshot.RepositoryRoot)
	result.OK = true
	result.BranchName = updated.BranchName
	return result, nil
}

func parseBranchLine(line string) (BranchListEntry, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return BranchListEntry{}, false
	}
	parts := strings.SplitN(line, "|", 2)
	name := strings.TrimSpace(parts[0])
	if name == "" || name == "origin/HEAD" {
		return BranchListEntry{}, false
	}
	entry := BranchListEntry{
		ID:          "branch:" + name,
		Kind:        "local",
		Name:        name,
		DisplayName: name,
	}
	if len(parts) > 1 {
		entry.UpdatedAt = strings.TrimSpace(parts[1])
	}
	if strings.Contains(name, "/") && !strings.HasPrefix(name, "heads/") {
		entry.Kind = "remote"
		entry.RemoteRef = name
		entry.DisplayName = strings.TrimPrefix(name, "origin/")
	}
	return entry, true
}

func countCommits(ctx context.Context, root string, revisionRange string) int {
	output, err := gitOutput(ctx, root, "rev-list", "--count", revisionRange)
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0
	}
	return count
}

func branchFailure(result BranchActionResult, title string, message string, detail string) BranchActionResult {
	result.OK = false
	result.Title = title
	result.Message = message
	result.Detail = detail
	return result
}
