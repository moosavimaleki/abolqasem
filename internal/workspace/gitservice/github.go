package gitservice

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type GitHubPublishInfo struct {
	GHInstalled        bool     `json:"ghInstalled"`
	Authenticated      bool     `json:"authenticated"`
	ActiveAccountLogin string   `json:"activeAccountLogin,omitempty"`
	Owners             []string `json:"owners"`
	SuggestedRepoName  string   `json:"suggestedRepoName"`
}

type GitHubRepoAvailabilityResult struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

var (
	lookPath = exec.LookPath
	ghOutput = runGHOutput
)

func GetGitHubPublishInfo(ctx context.Context, localPath string) (GitHubPublishInfo, error) {
	info := GitHubPublishInfo{
		Owners:            []string{},
		SuggestedRepoName: suggestedRepoName(localPath),
	}
	if _, err := lookPath("gh"); err != nil {
		return info, nil
	}
	info.GHInstalled = true
	if _, err := ghOutput(ctx, "auth", "status", "-h", "github.com"); err != nil {
		return info, nil
	}
	info.Authenticated = true
	if login, err := ghOutput(ctx, "api", "user", "--jq", ".login"); err == nil {
		info.ActiveAccountLogin = strings.TrimSpace(login)
	}
	if info.ActiveAccountLogin != "" {
		info.Owners = append(info.Owners, info.ActiveAccountLogin)
	}
	if orgs, err := ghOutput(ctx, "api", "user/orgs", "--jq", ".[].login"); err == nil {
		for _, org := range strings.Split(orgs, "\n") {
			org = strings.TrimSpace(org)
			if org != "" && !containsString(info.Owners, org) {
				info.Owners = append(info.Owners, org)
			}
		}
	}
	return info, nil
}

func CheckGitHubRepoAvailability(ctx context.Context, owner string, name string) (GitHubRepoAvailabilityResult, error) {
	owner = strings.TrimSpace(owner)
	name = strings.TrimSpace(name)
	if owner == "" || name == "" {
		return GitHubRepoAvailabilityResult{Available: false, Message: "Owner and repository name are required."}, nil
	}
	if _, err := lookPath("gh"); err != nil {
		return GitHubRepoAvailabilityResult{Available: false, Message: "GitHub CLI is not installed."}, nil
	}
	_, err := ghOutput(ctx, "repo", "view", owner+"/"+name, "--json", "name")
	if err == nil {
		return GitHubRepoAvailabilityResult{Available: false, Message: "Repository already exists."}, nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") || strings.Contains(message, "could not resolve") || strings.Contains(message, "404") {
		return GitHubRepoAvailabilityResult{Available: true, Message: "Repository name is available."}, nil
	}
	return GitHubRepoAvailabilityResult{Available: false, Message: err.Error()}, nil
}

func PublishToGitHub(ctx context.Context, localPath string, owner string, name string, visibility string, description string) (BranchActionResult, error) {
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return BranchActionResult{}, err
	}
	result := BranchActionResult{
		BranchName:      snapshot.BranchName,
		SnapshotChanged: true,
	}
	if snapshot.Status != StatusReady {
		return branchFailure(result, "Git repository not ready", "Initialize git before publishing to GitHub.", ""), nil
	}
	if _, err := lookPath("gh"); err != nil {
		return branchFailure(result, "GitHub CLI missing", "Install GitHub CLI before publishing.", ""), nil
	}
	repo := strings.TrimSpace(owner) + "/" + strings.TrimSpace(name)
	if strings.Trim(repo, "/") == "" || strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
		return branchFailure(result, "Repository required", "Choose an owner and repository name.", ""), nil
	}
	visibilityFlag := "--private"
	if visibility == "public" {
		visibilityFlag = "--public"
	}
	args := []string{
		"repo", "create", repo,
		"--source", snapshot.RepositoryRoot,
		"--remote", "origin",
		"--description", strings.TrimSpace(description),
		visibilityFlag,
	}
	if output, err := ghOutput(ctx, args...); err != nil {
		return branchFailure(result, "GitHub publish failed", err.Error(), output), nil
	}
	pushResult := pushCurrentBranch(ctx, snapshot.RepositoryRoot, snapshot.BranchName)
	if !pushResult.OK {
		return branchFailure(result, pushResult.Title, pushResult.Message, pushResult.Detail), nil
	}
	result.OK = true
	return result, nil
}

func runGHOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", gitCommandError{err: err, output: strings.TrimSpace(string(output))}
	}
	return strings.TrimSpace(string(output)), nil
}

func suggestedRepoName(localPath string) string {
	snapshot, err := Detect(context.Background(), localPath)
	basePath := strings.TrimSpace(localPath)
	if err == nil && snapshot.RepositoryRoot != "" {
		basePath = snapshot.RepositoryRoot
	}
	name := filepath.Base(filepath.Clean(basePath))
	name = strings.TrimSpace(name)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "new-repository"
	}
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	name = re.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-_")
	if name == "" {
		return "new-repository"
	}
	return name
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
