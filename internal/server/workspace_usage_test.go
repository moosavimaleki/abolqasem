package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHandleAPIUsageReturnsPersistedCodexSnapshot(t *testing.T) {
	previousPath := workspaceUsageCachePath
	cachePath := filepath.Join(t.TempDir(), "usage.json")
	workspaceUsageCachePath = func() string { return cachePath }
	t.Cleanup(func() { workspaceUsageCachePath = previousPath })

	workspaceStoreCodexUsage(map[string]any{"planType": "plus"})
	request := httptest.NewRequest(http.MethodGet, "/api/usage", nil)
	recorder := httptest.NewRecorder()
	handleAPIUsage(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); body == "" || body == "{\"codex\":null}\n" {
		t.Fatalf("expected persisted usage snapshot, got %q", body)
	}
}
