package server

import "testing"

func TestWorkspaceCodexWritableOwnerRecordsUsesLsofAccessField(t *testing.T) {
	owners := workspaceCodexWritableOwnerRecords("p1984740\nccodex\nf44\nau\n")

	if len(owners) != 1 {
		t.Fatalf("expected one writable owner, got %#v", owners)
	}
	if owners[0].PID != 1984740 || owners[0].Command != "codex" {
		t.Fatalf("unexpected writable owner: %#v", owners[0])
	}
}

func TestWorkspaceCodexWritableOwnerRecordsIgnoresReadOnlyAccess(t *testing.T) {
	owners := workspaceCodexWritableOwnerRecords("p1984740\nccodex\nf44\nar\n")

	if len(owners) != 0 {
		t.Fatalf("expected no writable owner, got %#v", owners)
	}
}

func TestWorkspaceCodexLsofExitMeansNoMatch(t *testing.T) {
	if !workspaceCodexLsofExitMeansNoMatch(1) {
		t.Fatal("expected lsof exit code 1 to mean no matching open files")
	}
	if workspaceCodexLsofExitMeansNoMatch(2) {
		t.Fatal("unexpected non-no-match lsof exit code")
	}
}
