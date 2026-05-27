package server

import (
	"ai-agent-manager/internal/buildinfo"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func resetWorkspaceUpdateState(t *testing.T) {
	t.Helper()
	workspaceUpdateMu.Lock()
	previous := workspaceUpdateState
	workspaceUpdateState = nil
	workspaceUpdateMu.Unlock()
	t.Cleanup(func() {
		workspaceUpdateMu.Lock()
		workspaceUpdateState = previous
		workspaceUpdateMu.Unlock()
	})
}

func TestWorkspaceCheckUpdateDetectsRelease(t *testing.T) {
	resetWorkspaceUpdateState(t)
	previousVersion := buildinfo.Version
	previousClient := appUpdateHTTPClient
	buildinfo.Version = "0.1.2"
	appUpdateHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"0.1.3"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
		appUpdateHTTPClient = previousClient
	})

	snapshot := workspaceCheckUpdate()
	if snapshot["status"] != "available" {
		t.Fatalf("expected available update, got %#v", snapshot["status"])
	}
	if snapshot["latestVersion"] != "0.1.3" {
		t.Fatalf("expected latest version 0.1.3, got %#v", snapshot["latestVersion"])
	}
	if snapshot["updateAvailable"] != true {
		t.Fatalf("expected updateAvailable true, got %#v", snapshot["updateAvailable"])
	}
	if persisted := workspaceUpdateSnapshot(); persisted["status"] != "available" || persisted["latestVersion"] != "0.1.3" {
		t.Fatalf("expected update snapshot to persist check result, got %#v", persisted)
	}
}

func TestWorkspaceCheckUpdateAllowsDevLocalBuildToInstallRelease(t *testing.T) {
	resetWorkspaceUpdateState(t)
	previousVersion := buildinfo.Version
	previousClient := appUpdateHTTPClient
	buildinfo.Version = "dev-local"
	appUpdateHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"1.0.4"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
		appUpdateHTTPClient = previousClient
	})

	snapshot := workspaceCheckUpdate()
	if snapshot["status"] != "available" || snapshot["updateAvailable"] != true {
		t.Fatalf("expected dev-local build to show available release update, got %#v", snapshot)
	}
}

func TestWorkspaceInstallUpdateSchedulesDetachedCommand(t *testing.T) {
	resetWorkspaceUpdateState(t)
	previousExecutablePath := executablePath
	previousStartDetached := startDetached
	started := make(chan []string, 1)
	executablePath = func() (string, error) {
		return "/tmp/abolqasem", nil
	}
	startDetached = func(exe string, args ...string) error {
		started <- append([]string{exe}, args...)
		return nil
	}
	t.Cleanup(func() {
		executablePath = previousExecutablePath
		startDetached = previousStartDetached
	})

	result := workspaceInstallUpdate()
	if result["ok"] != true {
		t.Fatalf("expected successful install scheduling, got %#v", result)
	}
	command := <-started
	if len(command) != 2 || command[0] != "/tmp/abolqasem" || command[1] != "update" {
		t.Fatalf("unexpected scheduled command: %#v", command)
	}
	snapshot := workspaceUpdateSnapshot()
	if snapshot["status"] != "restart_pending" || snapshot["reloadRequestedAt"] == nil {
		t.Fatalf("expected restart_pending update snapshot, got %#v", snapshot)
	}
}

func TestUpdateVersionNewerAllowsDevelopmentBuildsToInstallLatestRelease(t *testing.T) {
	if !updateVersionNewer("0.1.3", "dev") {
		t.Fatal("development builds should be able to install the latest release")
	}
	if !updateVersionNewer("0.1.3", "dev-local") {
		t.Fatal("local development builds should be able to install the latest release")
	}
}

func TestUpdateVersionNewerComparesReleaseVersions(t *testing.T) {
	if !updateVersionNewer("0.1.3", "0.1.2") {
		t.Fatal("expected 0.1.3 to be newer than 0.1.2")
	}
	if updateVersionNewer("0.1.2", "0.1.3") {
		t.Fatal("expected older latest version to be ignored")
	}
	if !updateVersionNewer("0.1.3", "0.1.3-beta.1") {
		t.Fatal("expected stable release to be newer than prerelease of the same version")
	}
}
