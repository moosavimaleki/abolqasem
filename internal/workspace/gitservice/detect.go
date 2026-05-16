package gitservice

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"mime"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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
	snapshot.Files = diffFiles(ctx, root)
	return snapshot, nil
}

func diffFiles(ctx context.Context, root string) []DiffFile {
	statusOutput, err := gitRawOutput(ctx, root, "status", "--porcelain=v1")
	if err != nil || strings.TrimSpace(statusOutput) == "" {
		return []DiffFile{}
	}

	filesByPath := map[string]DiffFile{}
	for _, line := range strings.Split(statusOutput, "\n") {
		file, ok := parseStatusLine(root, line)
		if !ok {
			continue
		}
		additions, deletions := numstatForPath(ctx, root, file.Path)
		file.Additions = additions
		file.Deletions = deletions
		file.PatchDigest = patchDigest(file)
		filesByPath[file.Path] = file
	}

	files := make([]DiffFile, 0, len(filesByPath))
	for _, file := range filesByPath {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func parseStatusLine(root string, line string) (DiffFile, bool) {
	if len(line) < 4 {
		return DiffFile{}, false
	}
	status := line[:2]
	rawPath := strings.TrimSpace(line[3:])
	if rawPath == "" {
		return DiffFile{}, false
	}
	path := rawPath
	if strings.Contains(rawPath, " -> ") {
		parts := strings.Split(rawPath, " -> ")
		path = parts[len(parts)-1]
	}
	path = strings.Trim(path, `"`)

	file := DiffFile{
		Path:        filepath.ToSlash(path),
		ChangeType:  changeType(status),
		IsUntracked: strings.Contains(status, "?"),
	}
	fillFileMetadata(root, &file)
	return file, true
}

func changeType(status string) string {
	switch {
	case strings.Contains(status, "R"):
		return "renamed"
	case strings.Contains(status, "D"):
		return "deleted"
	case strings.Contains(status, "?") || strings.Contains(status, "A"):
		return "added"
	default:
		return "modified"
	}
}

func fillFileMetadata(root string, file *DiffFile) {
	if file.ChangeType == "deleted" {
		return
	}
	absolutePath := filepath.Join(root, filepath.FromSlash(file.Path))
	info, err := os.Stat(absolutePath)
	if err != nil || info.IsDir() {
		return
	}
	file.Size = info.Size()
	if mimeType := mime.TypeByExtension(filepath.Ext(file.Path)); mimeType != "" {
		file.MimeType = mimeType
	}
}

func numstatForPath(ctx context.Context, root string, path string) (int, int) {
	additions, deletions := parseNumstat(mustGitOutput(ctx, root, "diff", "--numstat", "--", path))
	cachedAdditions, cachedDeletions := parseNumstat(mustGitOutput(ctx, root, "diff", "--cached", "--numstat", "--", path))
	return additions + cachedAdditions, deletions + cachedDeletions
}

func mustGitOutput(ctx context.Context, root string, args ...string) string {
	output, err := gitOutput(ctx, root, args...)
	if err != nil {
		return ""
	}
	return output
}

func parseNumstat(output string) (int, int) {
	additions := 0
	deletions := 0
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if value, err := strconv.Atoi(fields[0]); err == nil {
			additions += value
		}
		if value, err := strconv.Atoi(fields[1]); err == nil {
			deletions += value
		}
	}
	return additions, deletions
}

func patchDigest(file DiffFile) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		file.Path,
		file.ChangeType,
		strconv.Itoa(file.Additions),
		strconv.Itoa(file.Deletions),
	}, "\n")))
	return hex.EncodeToString(sum[:])
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	text, err := gitRawOutput(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func gitRawOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(output), "\r\n")
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
