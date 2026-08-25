package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"abolqasem/internal/state"
)

type workspaceUsageSnapshot struct {
	Codex *workspaceCodexUsageSnapshot `json:"codex"`
}

type workspaceCodexUsageSnapshot struct {
	RateLimits any       `json:"rate_limits"`
	UpdatedAt  time.Time `json:"updated_at"`
}

var (
	workspaceUsageMu        sync.Mutex
	workspaceUsageCachePath = func() string {
		return filepath.Join(state.GetStateDir(), "usage.json")
	}
)

func workspaceStoreCodexUsage(rateLimits any) {
	if rateLimits == nil {
		return
	}
	workspaceUsageMu.Lock()
	defer workspaceUsageMu.Unlock()

	snapshot := workspaceUsageSnapshot{Codex: &workspaceCodexUsageSnapshot{
		RateLimits: rateLimits,
		UpdatedAt:  time.Now().UTC(),
	}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	path := workspaceUsageCachePath()
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	tempPath := path + ".tmp"
	if os.WriteFile(tempPath, data, 0o600) != nil {
		return
	}
	_ = os.Rename(tempPath, path)
}

func workspaceLoadUsage() workspaceUsageSnapshot {
	workspaceUsageMu.Lock()
	defer workspaceUsageMu.Unlock()
	data, err := os.ReadFile(workspaceUsageCachePath())
	if err != nil {
		return workspaceUsageSnapshot{}
	}
	var snapshot workspaceUsageSnapshot
	if json.Unmarshal(data, &snapshot) != nil {
		return workspaceUsageSnapshot{}
	}
	return snapshot
}

func handleAPIUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, workspaceLoadUsage())
}
