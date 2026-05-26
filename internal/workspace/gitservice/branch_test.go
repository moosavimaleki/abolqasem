package gitservice

import (
	"context"
	"path/filepath"
	"testing"
)

func TestListCreateAndCheckoutBranches(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "file.txt"), "base\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	initialBranch, err := gitOutput(context.Background(), root, "branch", "--show-current")
	if err != nil {
		t.Fatalf("branch --show-current failed: %v", err)
	}

	created, err := CreateBranch(context.Background(), root, "feature")
	if err != nil {
		t.Fatalf("CreateBranch returned error: %v", err)
	}
	if !created.OK || created.BranchName != "feature" {
		t.Fatalf("unexpected create result: %#v", created)
	}
	checkedOut, err := CheckoutBranch(context.Background(), root, initialBranch)
	if err != nil {
		t.Fatalf("CheckoutBranch returned error: %v", err)
	}
	if !checkedOut.OK {
		t.Fatalf("unexpected checkout result: %#v", checkedOut)
	}

	branches, err := ListBranches(context.Background(), root)
	if err != nil {
		t.Fatalf("ListBranches returned error: %v", err)
	}
	if branches.CurrentBranchName != initialBranch {
		t.Fatalf("expected %s branch, got %#v", initialBranch, branches)
	}
	if !containsBranch(branches.Local, "feature") {
		t.Fatalf("expected feature branch in %#v", branches.Local)
	}
}

func TestListBranchesKeepsSlashedLocalBranchesLocal(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "file.txt"), "base\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	runGit(t, root, "switch", "-c", "feature/local")

	branches, err := ListBranches(context.Background(), root)
	if err != nil {
		t.Fatalf("ListBranches returned error: %v", err)
	}
	if !containsBranch(branches.Local, "feature/local") {
		t.Fatalf("expected feature/local branch in local branches, got %#v", branches.Local)
	}
	if containsBranch(branches.Remote, "feature/local") {
		t.Fatalf("did not expect feature/local branch in remote branches: %#v", branches.Remote)
	}
}

func TestCheckoutRemoteTrackingBranchCreatesLocalTrackingBranch(t *testing.T) {
	requireGit(t)
	remote := t.TempDir()
	runGit(t, remote, "init", "--bare")

	source := t.TempDir()
	initRepoWithIdentity(t, source)
	writeFile(t, filepath.Join(source, "file.txt"), "base\n")
	runGit(t, source, "add", ".")
	runGit(t, source, "commit", "-m", "initial")
	runGit(t, source, "checkout", "-b", "feature/remote")
	writeFile(t, filepath.Join(source, "feature.txt"), "feature\n")
	runGit(t, source, "add", ".")
	runGit(t, source, "commit", "-m", "feature")
	runGit(t, source, "remote", "add", "origin", remote)
	runGit(t, source, "push", "-u", "origin", "feature/remote")

	clone := t.TempDir()
	runGit(t, clone, "clone", remote, ".")

	checkedOut, err := CheckoutRemoteTrackingBranch(context.Background(), clone, "origin/feature/remote")
	if err != nil {
		t.Fatalf("CheckoutRemoteTrackingBranch returned error: %v", err)
	}
	if !checkedOut.OK || checkedOut.BranchName != "feature/remote" {
		t.Fatalf("unexpected checkout result: %#v", checkedOut)
	}
	if head, err := gitOutput(context.Background(), clone, "rev-parse", "--abbrev-ref", "HEAD"); err != nil || head != "feature/remote" {
		t.Fatalf("expected attached feature/remote branch, got head=%q err=%v", head, err)
	}
	upstream, err := gitOutput(context.Background(), clone, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		t.Fatalf("expected upstream after remote checkout: %v", err)
	}
	if upstream != "origin/feature/remote" {
		t.Fatalf("expected origin/feature/remote upstream, got %q", upstream)
	}
}

func TestPreviewAndMergeBranch(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepoWithIdentity(t, root)
	writeFile(t, filepath.Join(root, "file.txt"), "base\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	initialBranch, err := gitOutput(context.Background(), root, "branch", "--show-current")
	if err != nil {
		t.Fatalf("branch --show-current failed: %v", err)
	}
	runGit(t, root, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(root, "feature.txt"), "feature\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "feature")
	runGit(t, root, "checkout", initialBranch)

	preview, err := PreviewMergeBranch(context.Background(), root, "feature")
	if err != nil {
		t.Fatalf("PreviewMergeBranch returned error: %v", err)
	}
	if preview.Status != "mergeable" || preview.CommitCount != 1 || preview.HasConflicts {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	merged, err := MergeBranch(context.Background(), root, "feature")
	if err != nil {
		t.Fatalf("MergeBranch returned error: %v", err)
	}
	if !merged.OK || !merged.SnapshotChanged {
		t.Fatalf("unexpected merge result: %#v", merged)
	}
	if _, err := gitOutput(context.Background(), root, "rev-parse", "--verify", "HEAD:feature.txt"); err != nil {
		t.Fatalf("expected merged file: %v", err)
	}
}

func containsBranch(entries []BranchListEntry, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}
