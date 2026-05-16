package gitservice

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	StatusUnknown = "unknown"
	StatusReady   = "ready"
	StatusNoRepo  = "no_repo"
)

type DiffFile struct {
	Path        string `json:"path"`
	ChangeType  string `json:"changeType"`
	IsUntracked bool   `json:"isUntracked"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	PatchDigest string `json:"patchDigest"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type Snapshot struct {
	Status            string     `json:"status"`
	Files             []DiffFile `json:"files"`
	RepositoryRoot    string     `json:"repositoryRoot,omitempty"`
	BranchName        string     `json:"branchName,omitempty"`
	DefaultBranchName string     `json:"defaultBranchName,omitempty"`
	HasOriginRemote   *bool      `json:"hasOriginRemote,omitempty"`
	OriginRepoSlug    string     `json:"originRepoSlug,omitempty"`
	HasUpstream       *bool      `json:"hasUpstream,omitempty"`
	AheadCount        *int       `json:"aheadCount,omitempty"`
	BehindCount       *int       `json:"behindCount,omitempty"`
	LastFetchedAt     string     `json:"lastFetchedAt,omitempty"`
}

func Detect(ctx context.Context, localPath string) (Snapshot, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return Snapshot{Status: StatusUnknown, Files: []DiffFile{}}, nil
	}

	root, err := gitOutput(ctx, localPath, "rev-parse", "--show-toplevel")
	if err != nil {
		if isNoRepoError(err) {
			return Snapshot{Status: StatusNoRepo, Files: []DiffFile{}}, nil
		}
		return Snapshot{}, err
	}
	root = filepath.Clean(root)

	snapshot := Snapshot{
		Status:         StatusReady,
		Files:          []DiffFile{},
		RepositoryRoot: root,
	}
	if branch, err := gitOutput(ctx, root, "branch", "--show-current"); err == nil {
		snapshot.BranchName = branch
	}
	if snapshot.BranchName == "" {
		if branch, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && branch != "HEAD" {
			snapshot.BranchName = branch
		}
	}
	if defaultBranch, err := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		snapshot.DefaultBranchName = strings.TrimPrefix(defaultBranch, "origin/")
	}
	if originURL, err := gitOutput(ctx, root, "remote", "get-url", "origin"); err == nil {
		value := true
		snapshot.HasOriginRemote = &value
		snapshot.OriginRepoSlug = githubSlug(originURL)
	} else {
		value := false
		snapshot.HasOriginRemote = &value
	}
	if _, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		value := true
		snapshot.HasUpstream = &value
	} else {
		value := false
		snapshot.HasUpstream = &value
	}
	return snapshot, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return "", gitCommandError{err: err, output: text}
	}
	return text, nil
}

func isNoRepoError(err error) bool {
	var commandErr gitCommandError
	if !errors.As(err, &commandErr) {
		return false
	}
	output := strings.ToLower(commandErr.output)
	return strings.Contains(output, "not a git repository") ||
		strings.Contains(output, "not a git work tree")
}

func githubSlug(remote string) string {
	remote = strings.TrimSpace(strings.TrimSuffix(remote, ".git"))
	if strings.HasPrefix(remote, "git@github.com:") {
		return strings.TrimPrefix(remote, "git@github.com:")
	}
	parsed, err := url.Parse(remote)
	if err != nil || !strings.EqualFold(parsed.Host, "github.com") {
		return ""
	}
	return strings.TrimPrefix(parsed.Path, "/")
}

type gitCommandError struct {
	err    error
	output string
}

func (e gitCommandError) Error() string {
	if e.output == "" {
		return e.err.Error()
	}
	return e.err.Error() + ": " + e.output
}

func (e gitCommandError) Unwrap() error {
	return e.err
}
