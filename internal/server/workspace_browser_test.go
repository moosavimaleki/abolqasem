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
