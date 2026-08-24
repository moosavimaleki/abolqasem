package terminal

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagerCreateInputResizeClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows pipe fallback is covered by build; interactive shell behavior is platform-specific")
	}
	var mu sync.Mutex
	events := []Event{}
	manager := NewManager(func(event Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	snapshot, err := manager.Create(context.Background(), CreateRequest{
		TerminalID: "term-1",
		Cols:       100,
		Rows:       30,
		Scrollback: 500,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if snapshot.TerminalID != "term-1" || snapshot.Status != "running" || snapshot.Cols != 100 || snapshot.Rows != 30 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if err := manager.Resize("term-1", 90, 20); err != nil {
		t.Fatalf("Resize returned error: %v", err)
	}
	if err := manager.Input("term-1", "printf ready\\n\r"); err != nil {
		t.Fatalf("Input returned error: %v", err)
	}
	if !waitForEvent(t, &mu, &events, "terminal.output", "ready") {
		t.Fatalf("expected terminal output event, got %#v", events)
	}
	if err := manager.Close("term-1"); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestManagerReturnsExistingSnapshotForSameTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows pipe fallback is covered by build; interactive shell behavior is platform-specific")
	}
	manager := NewManager(nil)
	first, err := manager.Create(context.Background(), CreateRequest{TerminalID: "term-1", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	defer manager.Close("term-1")
	second, err := manager.Create(context.Background(), CreateRequest{TerminalID: "term-1", Cols: 120, Rows: 40})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if first.TerminalID != second.TerminalID {
		t.Fatalf("expected same terminal id, got %#v %#v", first, second)
	}
	if second.Cols != 120 || second.Rows != 40 {
		t.Fatalf("expected resized snapshot, got %#v", second)
	}
}

func TestManagerRootPIDsByCWD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows pipe fallback is covered by build; interactive shell behavior is platform-specific")
	}
	manager := NewManager(nil)
	cwd := t.TempDir()
	snapshot, err := manager.Create(context.Background(), CreateRequest{TerminalID: "term-1", CWD: cwd})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	defer manager.Close("term-1")
	if snapshot.CWD != cwd {
		t.Fatalf("expected cwd %q, got %q", cwd, snapshot.CWD)
	}
	pids := manager.RootPIDsByCWD(cwd)
	if len(pids) != 1 || pids[0] <= 0 {
		t.Fatalf("expected one root pid for cwd %q, got %#v", cwd, pids)
	}
	if other := manager.RootPIDsByCWD(t.TempDir()); len(other) != 0 {
		t.Fatalf("expected no root pids for unrelated cwd, got %#v", other)
	}
}

func TestTerminalStateIsBounded(t *testing.T) {
	state := newTerminalState(8)
	state.WriteString("12345")
	state.WriteString("67890")
	if got := state.String(); got != "34567890" {
		t.Fatalf("expected bounded terminal state, got %q", got)
	}
	state.setLimit(4)
	if got := state.String(); got != "7890" {
		t.Fatalf("expected terminal state to shrink after limit change, got %q", got)
	}
}

func waitForEvent(t *testing.T, mu *sync.Mutex, events *[]Event, eventType string, contains string) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, event := range *events {
			if event.Type == eventType && strings.Contains(event.Data, contains) {
				mu.Unlock()
				return true
			}
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
