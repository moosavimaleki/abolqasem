package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestListWorkspaceLocalHTTPServersProbesAndMarksSameProject(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := t.TempDir()
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><title>Dev Server</title></html>"))
	}))
	t.Cleanup(testServer.Close)
	port := testServerPort(t, testServer.URL)

	previous := workspaceListListeningPorts
	workspaceListListeningPorts = func(context.Context) ([]workspaceListeningPort, error) {
		return []workspaceListeningPort{{
			Port:        port,
			PID:         1234,
			ProcessName: "vite",
			OwnerPath:   filepath.Join(projectDir, "web"),
		}}, nil
	}
	t.Cleanup(func() { workspaceListListeningPorts = previous })

	raw, _ := json.Marshal(map[string]any{"projectId": project.ID})
	servers, err := listWorkspaceLocalHTTPServers(raw)
	if err != nil {
		t.Fatalf("listWorkspaceLocalHTTPServers returned error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected one server, got %#v", servers)
	}
	if servers[0].Title != "Dev Server" || servers[0].Status != http.StatusOK || !servers[0].SameProject {
		t.Fatalf("unexpected server info: %#v", servers[0])
	}
}

func TestListWorkspaceLocalHTTPServersMarksSameProjectByTerminalPID(t *testing.T) {
	withWorkspaceComposerStore(t)
	projectDir := t.TempDir()
	project, err := workspaceOpenProject(projectDir, "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><title>Terminal Server</title></html>"))
	}))
	t.Cleanup(testServer.Close)
	port := testServerPort(t, testServer.URL)

	previousList := workspaceListListeningPorts
	previousParents := workspaceReadParentProcessMap
	previousRoots := workspaceTerminalRootPIDsByCWD
	workspaceListListeningPorts = func(context.Context) ([]workspaceListeningPort, error) {
		return []workspaceListeningPort{{
			Port:        port,
			PID:         333,
			ProcessName: "node",
			OwnerPath:   t.TempDir(),
		}}, nil
	}
	workspaceReadParentProcessMap = func(context.Context) (map[int]int, error) {
		return map[int]int{333: 222, 222: 111}, nil
	}
	workspaceTerminalRootPIDsByCWD = func(cwd string) []int {
		if cwd != projectDir {
			return nil
		}
		return []int{111}
	}
	t.Cleanup(func() {
		workspaceListListeningPorts = previousList
		workspaceReadParentProcessMap = previousParents
		workspaceTerminalRootPIDsByCWD = previousRoots
	})

	raw, _ := json.Marshal(map[string]any{"projectId": project.ID})
	servers, err := listWorkspaceLocalHTTPServers(raw)
	if err != nil {
		t.Fatalf("listWorkspaceLocalHTTPServers returned error: %v", err)
	}
	if len(servers) != 1 || !servers[0].SameProject {
		t.Fatalf("expected terminal descendant server to be same project, got %#v", servers)
	}
}

func TestKillWorkspaceLocalHTTPServerKillsMatchingPort(t *testing.T) {
	previousList := workspaceListListeningPorts
	previousKill := workspaceKillProcess
	killedPID := 0
	workspaceListListeningPorts = func(context.Context) ([]workspaceListeningPort, error) {
		return []workspaceListeningPort{{Port: 5173, PID: 4321}}, nil
	}
	workspaceKillProcess = func(pid int) error {
		killedPID = pid
		return nil
	}
	t.Cleanup(func() {
		workspaceListListeningPorts = previousList
		workspaceKillProcess = previousKill
	})

	raw, _ := json.Marshal(map[string]any{"port": 5173})
	if err := killWorkspaceLocalHTTPServer(raw); err != nil {
		t.Fatalf("killWorkspaceLocalHTTPServer returned error: %v", err)
	}
	if killedPID != 4321 {
		t.Fatalf("expected pid 4321 to be killed, got %d", killedPID)
	}
}

func TestParsePortFromAddress(t *testing.T) {
	for address, expected := range map[string]int{
		"127.0.0.1:3000":       3000,
		"[::1]:5173":           5173,
		"*:8080":               8080,
		"localhost:12345":      12345,
		"not-a-listener":       0,
		"127.0.0.1:not-number": 0,
	} {
		if got := parsePortFromAddress(address); got != expected {
			t.Fatalf("parsePortFromAddress(%q) = %d, expected %d", address, got, expected)
		}
	}
}

func TestParseLsofListeningEntries(t *testing.T) {
	output := `
COMMAND   PID USER   FD   TYPE DEVICE SIZE/OFF NODE NAME
node    12345 user   23u  IPv4 123456      0t0  TCP *:5174 (LISTEN)
bun     12346 user   23u  IPv4 123457      0t0  TCP localhost:3210 (LISTEN)
other   12347 user   23u  IPv6 123458      0t0  TCP [::1]:8080 (LISTEN)
`
	entries := parseLsofListeningEntries(output)
	if len(entries) != 3 {
		t.Fatalf("expected three entries, got %#v", entries)
	}
	if entries[0].ProcessName != "node" || entries[0].PID != 12345 || entries[0].Port != 5174 {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].ProcessName != "bun" || entries[1].PID != 12346 || entries[1].Port != 3210 {
		t.Fatalf("unexpected second entry: %#v", entries[1])
	}
	if entries[2].ProcessName != "other" || entries[2].PID != 12347 || entries[2].Port != 8080 {
		t.Fatalf("unexpected third entry: %#v", entries[2])
	}
}

func TestIsDescendantPID(t *testing.T) {
	roots := map[int]bool{10: true}
	parents := map[int]int{30: 20, 20: 10}
	if !isDescendantPID(30, roots, parents) {
		t.Fatalf("expected pid 30 to be a descendant of 10")
	}
	if !isDescendantPID(10, roots, parents) {
		t.Fatalf("expected root pid to match itself")
	}
	if isDescendantPID(40, roots, parents) {
		t.Fatalf("expected unrelated pid not to match")
	}
}

func testServerPort(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	port := parsePortFromAddress(parsed.Host)
	if port <= 0 {
		t.Fatalf("failed to parse port from %q", rawURL)
	}
	return port
}
