package server

import (
	"context"
	"encoding/json"
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

func TestHandleAPIUsageRefreshPersistsFreshCodexSnapshot(t *testing.T) {
	previousPath := workspaceUsageCachePath
	previousRead := workspaceReadCodexUsage
	cachePath := filepath.Join(t.TempDir(), "usage.json")
	workspaceUsageCachePath = func() string { return cachePath }
	workspaceReadCodexUsage = func(context.Context) (any, error) {
		return map[string]any{
			"planType": "plus",
			"primary":  map[string]any{"usedPercent": 83, "windowDurationMins": 10_080},
		}, nil
	}
	t.Cleanup(func() {
		workspaceUsageCachePath = previousPath
		workspaceReadCodexUsage = previousRead
	})

	request := httptest.NewRequest(http.MethodPost, "/api/usage/refresh", nil)
	recorder := httptest.NewRecorder()
	handleAPIUsageRefresh(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var snapshot workspaceUsageSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Codex == nil {
		t.Fatal("expected refreshed Codex usage")
	}
	rateLimits, ok := snapshot.Codex.RateLimits.(map[string]any)
	if !ok || rateLimits["planType"] != "plus" {
		t.Fatalf("unexpected refreshed limits: %#v", snapshot.Codex.RateLimits)
	}
}
