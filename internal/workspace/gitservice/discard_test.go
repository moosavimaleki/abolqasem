package gitservice

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscardDiffFileRestoresTrackedFile(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "tracked.txt"), "original\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, "tracked.txt"), "changed\n")

	result, err := DiscardDiffFile(context.Background(), root, "tracked.txt")
	if err != nil {
		t.Fatalf("DiscardDiffFile returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected discard success, got %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "tracked.txt"))
	if err != nil {
		t.Fatalf("read tracked file failed: %v", err)
	}
	if string(content) != "original\n" {
		t.Fatalf("expected original content, got %q", content)
	}
}

func TestDiscardDiffFileRemovesUntrackedFile(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "tracked.txt"), "tracked\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, "new.txt"), "new\n")

	result, err := DiscardDiffFile(context.Background(), root, "new.txt")
	if err != nil {
		t.Fatalf("DiscardDiffFile returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected discard success, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected untracked file to be removed, err=%v", err)
	}
}

func TestIgnoreDiffFileAndFolderAppendGitignore(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "tracked.txt"), "tracked\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	writeFile(t, filepath.Join(root, "tmp", "cache.txt"), "cache\n")

	fileResult, err := IgnoreDiffFile(context.Background(), root, "tmp/cache.txt")
	if err != nil {
		t.Fatalf("IgnoreDiffFile returned error: %v", err)
	}
	if !fileResult.OK {
		t.Fatalf("expected ignore file success, got %#v", fileResult)
	}
	folderResult, err := IgnoreDiffFolder(context.Background(), root, "tmp/cache.txt")
	if err != nil {
		t.Fatalf("IgnoreDiffFolder returned error: %v", err)
	}
	if !folderResult.OK {
		t.Fatalf("expected ignore folder success, got %#v", folderResult)
	}
	content, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore failed: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "tmp/cache.txt\n") || !strings.Contains(text, "tmp/\n") {
		t.Fatalf("unexpected .gitignore content: %q", text)
	}
}
