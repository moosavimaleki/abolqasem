package gitservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectReturnsNoRepoForPlainDirectory(t *testing.T) {
	requireGit(t)
	snapshot, err := Detect(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if snapshot.Status != StatusNoRepo {
		t.Fatalf("expected no_repo, got %#v", snapshot)
	}
	if len(snapshot.Files) != 0 {
		t.Fatalf("expected empty files, got %#v", snapshot.Files)
	}
}

func TestDetectFindsRepositoryRootAndBranch(t *testing.T) {
	requireGit(t)
	root := canonicalGitserviceTestPath(t, t.TempDir())
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "git@github.com:owner/repo.git")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	snapshot, err := Detect(context.Background(), nested)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if snapshot.Status != StatusReady {
		t.Fatalf("expected ready, got %#v", snapshot)
	}
	if snapshot.RepositoryRoot != root {
		t.Fatalf("expected root %q, got %q", root, snapshot.RepositoryRoot)
	}
	if snapshot.BranchName == "" {
		t.Fatalf("expected branch name, got %#v", snapshot)
	}
	if snapshot.HasOriginRemote == nil || !*snapshot.HasOriginRemote {
		t.Fatalf("expected origin remote, got %#v", snapshot.HasOriginRemote)
	}
	if snapshot.OriginRepoSlug != "owner/repo" {
		t.Fatalf("expected GitHub slug owner/repo, got %q", snapshot.OriginRepoSlug)
	}
}

func TestDetectDiffSnapshotIncludesChangedFileTypes(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(root, "modified.txt"), "one\n")
	writeFile(t, filepath.Join(root, "deleted.txt"), "remove\n")
	writeFile(t, filepath.Join(root, "renamed.txt"), "move\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")

	writeFile(t, filepath.Join(root, "modified.txt"), "one\ntwo\n")
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	runGit(t, root, "mv", "renamed.txt", "renamed-new.txt")
	writeFile(t, filepath.Join(root, "untracked.txt"), "new\n")

	snapshot, err := Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	files := map[string]DiffFile{}
	for _, file := range snapshot.Files {
		files[file.Path] = file
	}

	assertChange(t, files, "modified.txt", "modified", false)
	if files["modified.txt"].Additions == 0 {
		t.Fatalf("expected additions for modified file, got %#v", files["modified.txt"])
	}
	assertChange(t, files, "deleted.txt", "deleted", false)
	assertChange(t, files, "renamed-new.txt", "renamed", false)
	assertChange(t, files, "untracked.txt", "added", true)
	for path, file := range files {
		if file.PatchDigest == "" {
			t.Fatalf("expected patch digest for %s", path)
		}
	}
}

func TestDetectIncludesBranchHistory(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/repo.git")
	writeFile(t, filepath.Join(root, "README.md"), "hello\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "Initial commit", "-m", "Set up repository")
	sha := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "tag", "v1.0.0")

	snapshot, err := Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if len(snapshot.BranchHistory.Entries) != 1 {
		t.Fatalf("expected one history entry, got %#v", snapshot.BranchHistory)
	}
	entry := snapshot.BranchHistory.Entries[0]
	if entry.SHA != sha || entry.Summary != "Initial commit" || entry.Description != "Set up repository" {
		t.Fatalf("unexpected history entry: %#v", entry)
	}
	if entry.AuthorName != "Test User" || entry.AuthoredAt == "" {
		t.Fatalf("expected author metadata, got %#v", entry)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "v1.0.0" {
		t.Fatalf("expected tag, got %#v", entry.Tags)
	}
	if entry.GitHubURL != "https://github.com/acme/repo/commit/"+sha {
		t.Fatalf("unexpected GitHub URL: %q", entry.GitHubURL)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
}

func assertChange(t *testing.T, files map[string]DiffFile, path string, changeType string, untracked bool) {
	t.Helper()
	file, ok := files[path]
	if !ok {
		t.Fatalf("expected %s in diff files, got %#v", path, files)
	}
	if file.ChangeType != changeType || file.IsUntracked != untracked {
		t.Fatalf("unexpected change for %s: %#v", path, file)
	}
}
