package gitservice

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	localBranchRefPrefix  = "refs/heads/"
	remoteBranchRefPrefix = "refs/remotes/"
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
	result := BranchListResult{
		Recent:             []BranchListEntry{},
		Local:              []BranchListEntry{},
		Remote:             []BranchListEntry{},
		PullRequests:       []BranchListEntry{},
		PullRequestsStatus: "unavailable",
	}
	root, err := gitOutput(ctx, localPath, "rev-parse", "--show-toplevel")
	if err != nil {
		if isNoRepoError(err) {
			return result, nil
		}
		return BranchListResult{}, err
	}
	root = filepath.Clean(root)
	if branch, err := gitOutput(ctx, root, "branch", "--show-current"); err == nil {
		result.CurrentBranchName = branch
	}
	if result.CurrentBranchName == "" {
		if branch, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && branch != "HEAD" {
			result.CurrentBranchName = branch
		}
	}
	if defaultBranch, err := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		result.DefaultBranchName = strings.TrimPrefix(defaultBranch, "origin/")
	}
	output, err := gitOutput(ctx, root, "for-each-ref", "--sort=-committerdate", "--format=%(refname)|%(refname:short)|%(committerdate:iso8601)", "refs/heads", "refs/remotes")
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
	if result.DefaultBranchName == "" {
		result.DefaultBranchName = defaultBranchFromEntries(result.Local, result.CurrentBranchName)
	}
	return result, nil
}

func defaultBranchFromEntries(local []BranchListEntry, currentBranch string) string {
	for _, candidate := range []string{"main", "master"} {
		if branchListContains(local, candidate) {
			return candidate
		}
	}
	if currentBranch != "" {
		return currentBranch
	}
	if len(local) > 0 {
		return local[0].Name
	}
	return ""
}

func branchListContains(entries []BranchListEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}

func CheckoutBranch(ctx context.Context, localPath string, branchName string) (BranchActionResult, error) {
	return branchCommand(ctx, localPath, []string{"switch", strings.TrimSpace(branchName)}, true)
}

func CheckoutRemoteTrackingBranch(ctx context.Context, localPath string, remoteRef string) (BranchActionResult, error) {
	remoteRef = strings.TrimSpace(remoteRef)
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return BranchActionResult{}, err
	}
	result := BranchActionResult{
		BranchName:      snapshot.BranchName,
		SnapshotChanged: true,
	}
	if snapshot.Status != StatusReady {
		return branchFailure(result, "Git repository not ready", "Initialize git before running branch actions.", ""), nil
	}
	if remoteRef == "" {
		return branchFailure(result, "Branch required", "Choose a branch first.", ""), nil
	}
	localName := localBranchNameForRemoteRef(remoteRef)
	if localName == "" {
		return branchFailure(result, "Branch action failed", "Remote branch name is invalid.", remoteRef), nil
	}

	if localBranchExists(ctx, snapshot.RepositoryRoot, localName) {
		return branchCommand(ctx, snapshot.RepositoryRoot, []string{"switch", localName}, true)
	}
	if output, err := gitOutput(ctx, snapshot.RepositoryRoot, "switch", "--track", remoteRef); err != nil {
		return branchFailure(result, "Branch action failed", err.Error(), output), nil
	}
	updated, _ := Detect(ctx, snapshot.RepositoryRoot)
	result.OK = true
	result.BranchName = updated.BranchName
	return result, nil
}

func CreateBranch(ctx context.Context, localPath string, branchName string) (BranchActionResult, error) {
	return branchCommand(ctx, localPath, []string{"switch", "-c", strings.TrimSpace(branchName)}, true)
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
	parts := strings.SplitN(line, "|", 3)
	if len(parts) < 2 {
		return BranchListEntry{}, false
	}
	refName := strings.TrimSpace(parts[0])
	shortName := strings.TrimSpace(parts[1])
	updatedAt := ""
	if len(parts) > 2 {
		updatedAt = strings.TrimSpace(parts[2])
	}
	if strings.HasPrefix(refName, localBranchRefPrefix) {
		name := strings.TrimPrefix(refName, localBranchRefPrefix)
		if name == "" {
			return BranchListEntry{}, false
		}
		return BranchListEntry{
			ID:          "branch:" + name,
			Kind:        "local",
			Name:        name,
			DisplayName: name,
			UpdatedAt:   updatedAt,
		}, true
	}
	if strings.HasPrefix(refName, remoteBranchRefPrefix) {
		remoteRef := strings.TrimPrefix(refName, remoteBranchRefPrefix)
		if remoteRef == "" || strings.HasSuffix(remoteRef, "/HEAD") {
			return BranchListEntry{}, false
		}
		displayName := localBranchNameForRemoteRef(remoteRef)
		if displayName == "" {
			displayName = shortName
		}
		return BranchListEntry{
			ID:          "branch:" + remoteRef,
			Kind:        "remote",
			Name:        remoteRef,
			DisplayName: displayName,
			UpdatedAt:   updatedAt,
			RemoteRef:   remoteRef,
		}, true
	}
	return BranchListEntry{}, false
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

func localBranchExists(ctx context.Context, root string, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, err := gitOutput(ctx, root, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func localBranchNameForRemoteRef(remoteRef string) string {
	remoteRef = strings.Trim(strings.TrimSpace(remoteRef), "/")
	parts := strings.SplitN(remoteRef, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.Trim(parts[1], "/")
}

func branchFailure(result BranchActionResult, title string, message string, detail string) BranchActionResult {
	result.OK = false
	result.Title = title
	result.Message = message
	result.Detail = detail
	return result
}
