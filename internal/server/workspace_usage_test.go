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
	previousAccountRead := workspaceReadCodexAccount
	cachePath := filepath.Join(t.TempDir(), "usage.json")
	workspaceUsageCachePath = func() string { return cachePath }
	workspaceReadCodexUsage = func(context.Context) (any, error) {
		return map[string]any{
			"planType": "plus",
			"primary":  map[string]any{"usedPercent": 83, "windowDurationMins": 10_080},
		}, nil
	}
	workspaceReadCodexAccount = func(context.Context) (*workspaceCodexAccountSnapshot, error) {
		return &workspaceCodexAccountSnapshot{Type: "chatgpt", Email: "active@example.com", PlanType: "plus"}, nil
	}
	t.Cleanup(func() {
		workspaceUsageCachePath = previousPath
		workspaceReadCodexUsage = previousRead
		workspaceReadCodexAccount = previousAccountRead
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
	if snapshot.Codex.Account == nil || snapshot.Codex.Account.Email != "active@example.com" {
		t.Fatalf("expected active Codex account email, got %#v", snapshot.Codex.Account)
	}
}

func TestWorkspaceStoreCodexUsagePreservesAccount(t *testing.T) {
	previousPath := workspaceUsageCachePath
	cachePath := filepath.Join(t.TempDir(), "usage.json")
	workspaceUsageCachePath = func() string { return cachePath }
	t.Cleanup(func() { workspaceUsageCachePath = previousPath })

	workspaceStoreCodexAccount(&workspaceCodexAccountSnapshot{Email: "active@example.com"})
	workspaceStoreCodexUsage(map[string]any{"planType": "plus"})

	snapshot := workspaceLoadUsage()
	if snapshot.Codex == nil || snapshot.Codex.Account == nil || snapshot.Codex.Account.Email != "active@example.com" {
		t.Fatalf("expected rate-limit updates to preserve account identity, got %#v", snapshot.Codex)
	}
}
