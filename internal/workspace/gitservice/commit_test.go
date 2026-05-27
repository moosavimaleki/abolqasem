package gitservice

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCommitCommitsOnlySelectedFiles(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "a.txt"), "a\n")
	writeFile(t, filepath.Join(root, "b.txt"), "b\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")

	writeFile(t, filepath.Join(root, "a.txt"), "a\nselected\n")
	writeFile(t, filepath.Join(root, "b.txt"), "b\nnot selected\n")

	result, err := Commit(context.Background(), root, CommitRequest{
		Paths:   []string{"a.txt"},
		Summary: "commit selected file",
		Mode:    CommitOnly,
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if !result.OK || result.Mode != CommitOnly || result.Pushed {
		t.Fatalf("unexpected result: %#v", result)
	}
	subject, err := gitOutput(context.Background(), root, "log", "-1", "--pretty=%s")
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if subject != "commit selected file" {
		t.Fatalf("expected commit subject, got %q", subject)
	}
	remaining, err := gitOutput(context.Background(), root, "diff", "--name-only")
	if err != nil {
		t.Fatalf("git diff failed: %v", err)
	}
	if remaining != "b.txt" {
		t.Fatalf("expected only b.txt to remain changed, got %q", remaining)
	}
}

func TestCommitRejectsEmptySelection(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)

	result, err := Commit(context.Background(), root, CommitRequest{
		Summary: "nothing",
		Mode:    CommitOnly,
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if result.OK || result.Phase != "commit" {
		t.Fatalf("expected commit failure, got %#v", result)
	}
}

func TestCommitAndPushCreatesUpstreamWhenMissing(t *testing.T) {
	requireGit(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "", "init", "--bare", remote)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	runGit(t, root, "remote", "add", "origin", remote)
	writeFile(t, filepath.Join(root, "a.txt"), "a\n")

	result, err := Commit(context.Background(), root, CommitRequest{
		Paths:   []string{"a.txt"},
		Summary: "commit and push",
		Mode:    CommitAndPush,
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if !result.OK || !result.Pushed {
		t.Fatalf("expected pushed commit, got %#v", result)
	}
	upstream, err := gitOutput(context.Background(), root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		t.Fatalf("expected upstream after push: %v", err)
	}
	if upstream == "" {
		t.Fatal("expected upstream branch")
	}
}

func TestSyncFetchAgainstOrigin(t *testing.T) {
	requireGit(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, "", "init", "--bare", remote)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	runGit(t, root, "remote", "add", "origin", remote)

	result, err := Sync(context.Background(), root, SyncFetch)
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if !result.OK || result.Action != SyncFetch {
		t.Fatalf("expected fetch success, got %#v", result)
	}
}

func initRepoWithIdentity(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	runGit(t, root, "config", "core.autocrlf", "false")
	runGit(t, root, "config", "core.eol", "lf")
}
