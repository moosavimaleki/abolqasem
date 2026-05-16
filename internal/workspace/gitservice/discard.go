package gitservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func DiscardDiffFile(ctx context.Context, localPath string, path string) (BranchActionResult, error) {
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return BranchActionResult{}, err
	}
	result := BranchActionResult{
		BranchName:      snapshot.BranchName,
		SnapshotChanged: true,
	}
	if snapshot.Status != StatusReady {
		return branchFailure(result, "Git repository not ready", "Initialize git before discarding changes.", ""), nil
	}
	path = cleanDiffPath(path)
	if path == "" {
		return branchFailure(result, "Path required", "Choose a file to discard.", ""), nil
	}
	file, ok := findDiffFile(snapshot.Files, path)
	if !ok {
		return branchFailure(result, "File not changed", "The selected file is not in the current diff.", ""), nil
	}
	if file.IsUntracked {
		absolutePath, err := safeRepoPath(snapshot.RepositoryRoot, path)
		if err != nil {
			return branchFailure(result, "Invalid path", err.Error(), ""), nil
		}
		if err := os.RemoveAll(absolutePath); err != nil {
			return branchFailure(result, "Discard failed", err.Error(), ""), nil
		}
		result.OK = true
		return result, nil
	}
	if output, err := gitOutput(ctx, snapshot.RepositoryRoot, "restore", "--staged", "--worktree", "--", path); err != nil {
		return branchFailure(result, "Discard failed", err.Error(), output), nil
	}
	result.OK = true
	return result, nil
}

func IgnoreDiffFile(ctx context.Context, localPath string, path string) (BranchActionResult, error) {
	return ignorePattern(ctx, localPath, cleanDiffPath(path))
}

func IgnoreDiffFolder(ctx context.Context, localPath string, path string) (BranchActionResult, error) {
	path = cleanDiffPath(path)
	if path == "" {
		return ignorePattern(ctx, localPath, "")
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		dir = path
	}
	return ignorePattern(ctx, localPath, strings.TrimSuffix(dir, "/")+"/")
}

func ignorePattern(ctx context.Context, localPath string, pattern string) (BranchActionResult, error) {
	snapshot, err := Detect(ctx, localPath)
	if err != nil {
		return BranchActionResult{}, err
	}
	result := BranchActionResult{
		BranchName:      snapshot.BranchName,
		SnapshotChanged: true,
	}
	if snapshot.Status != StatusReady {
		return branchFailure(result, "Git repository not ready", "Initialize git before updating .gitignore.", ""), nil
	}
	if pattern == "" {
		return branchFailure(result, "Pattern required", "Choose a file or folder to ignore.", ""), nil
	}
	gitignorePath := filepath.Join(snapshot.RepositoryRoot, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return branchFailure(result, "Ignore failed", err.Error(), ""), nil
	}
	if hasGitignorePattern(string(existing), pattern) {
		result.OK = true
		return result, nil
	}
	next := appendGitignorePattern(string(existing), pattern)
	if err := os.WriteFile(gitignorePath, []byte(next), 0o644); err != nil {
		return branchFailure(result, "Ignore failed", err.Error(), ""), nil
	}
	result.OK = true
	return result, nil
}

func findDiffFile(files []DiffFile, path string) (DiffFile, bool) {
	path = cleanDiffPath(path)
	for _, file := range files {
		if cleanDiffPath(file.Path) == path {
			return file, true
		}
	}
	return DiffFile{}, false
}

func cleanDiffPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.Trim(path, "/")
	return path
}

func safeRepoPath(root string, path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", errors.New("path must stay inside repository")
	}
	absolutePath := filepath.Join(root, filepath.FromSlash(path))
	rel, err := filepath.Rel(root, absolutePath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("path must stay inside repository")
	}
	return absolutePath, nil
}

func hasGitignorePattern(content string, pattern string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == pattern {
			return true
		}
	}
	return false
}

func appendGitignorePattern(content string, pattern string) string {
	content = strings.TrimRight(content, "\r\n")
	if content == "" {
		return pattern + "\n"
	}
	return content + "\n" + pattern + "\n"
}
