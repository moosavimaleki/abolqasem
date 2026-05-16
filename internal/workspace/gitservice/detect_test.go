package gitservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	root := t.TempDir()
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
