package server

import (
	"os"
	"os/exec"
	"testing"

	"abolqasem/internal/workspace/agent"
)

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

func TestWorkspaceCodexSessionManagerRecognizesItsWriterPID(t *testing.T) {
	manager := newWorkspaceCodexSessionManager()
	manager.sessions["chat-1"] = &workspaceCodexSession{
		threadID:      "session-1",
		executionMode: "standard",
		process: &workspaceCodexProcess{
			cmd:  &exec.Cmd{Process: &os.Process{Pid: 4242}},
			done: make(chan struct{}),
		},
	}

	if mode, owned := manager.ownedExecutionModeByWriterPID("chat-1", "session-1", 4242); !owned || mode != "standard" {
		t.Fatalf("writer pid ownership = %q %v", mode, owned)
	}
	if _, owned := manager.ownedExecutionModeByWriterPID("chat-1", "session-1", 99); owned {
		t.Fatal("unexpected ownership for a different writer pid")
	}
}

func TestWorkspaceCodexSessionManagerRecognizesWriterThroughChatAlias(t *testing.T) {
	manager := newWorkspaceCodexSessionManager()
	manager.sessions["chat-original"] = &workspaceCodexSession{
		threadID:      "session-1",
		executionMode: "dangerous",
		process: &workspaceCodexProcess{
			cmd:  &exec.Cmd{Process: &os.Process{Pid: 4242}},
			done: make(chan struct{}),
		},
	}

	if mode, owned := manager.ownedExecutionModeByWriterPID("chat-alias", "session-1", 4242); !owned || mode != "dangerous" {
		t.Fatalf("alias writer ownership = %q %v", mode, owned)
	}
	if mode, owned := manager.ownedExecutionMode("chat-alias", "session-1"); !owned || mode != "dangerous" {
		t.Fatalf("alias in-memory ownership = %q %v", mode, owned)
	}
	if ownerChatID := manager.ownerChatID("chat-alias", "session-1"); ownerChatID != "chat-original" {
		t.Fatalf("alias owner chat id = %q", ownerChatID)
	}
	if _, owned := manager.ownedExecutionModeByWriterPID("chat-alias", "session-other", 4242); owned {
		t.Fatal("a different session id must not inherit ownership")
	}
}

func TestWorkspaceCodexSessionManagerRemovesAliasedProcess(t *testing.T) {
	manager := newWorkspaceCodexSessionManager()
	process := &workspaceCodexProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 4242}}, done: make(chan struct{})}
	manager.sessions["chat-original"] = &workspaceCodexSession{threadID: "session-1", process: process}

	manager.remove("chat-alias", process)
	if len(manager.sessions) != 0 {
		t.Fatalf("aliased process was not removed: %#v", manager.sessions)
	}
}

func TestWorkspaceCodexSessionReuseRequiresSameThread(t *testing.T) {
	session := &workspaceCodexSession{
		cwd:           "/project",
		threadID:      "session-1",
		executionMode: "standard",
		process:       &workspaceCodexProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 4242}}, done: make(chan struct{})},
	}
	if session.reusableFor(agent.TurnRequest{LocalPath: "/project", SessionToken: "session-other", ExecutionMode: "standard"}) {
		t.Fatal("session must not be reused for a different thread")
	}
}

func TestWorkspaceCodexOwnersContainOnlyRequestedPID(t *testing.T) {
	owners := []workspaceCodexLockOwner{{PID: 10}, {PID: 20}}
	if !workspaceCodexOwnersContainPID(owners, 20) {
		t.Fatal("expected requested owner pid")
	}
	if workspaceCodexOwnersContainPID(owners, 30) {
		t.Fatal("unexpected owner pid match")
	}
}
