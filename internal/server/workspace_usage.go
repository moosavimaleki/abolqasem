package server

import (
	"context"
	"encoding/json"
	"errors"
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
	workspaceReadCodexUsage = readWorkspaceCodexRateLimits
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

func handleAPIUsageRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rateLimits, err := workspaceReadCodexUsage(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if rateLimits == nil {
		http.Error(w, "Codex did not return usage limits", http.StatusBadGateway)
		return
	}
	workspaceStoreCodexUsage(rateLimits)
	writeJSON(w, workspaceLoadUsage())
}

func readWorkspaceCodexRateLimits(ctx context.Context) (any, error) {
	process := workspaceCodexSessions.anyLiveProcess()
	ownedProbe := false
	if process == nil {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = os.TempDir()
		}
		process, err = startWorkspaceCodexProcess(ctx, cwd, os.Environ())
		if err != nil {
			return nil, err
		}
		ownedProbe = true
		if err := process.Initialize(ctx); err != nil {
			process.Close()
			return nil, process.wrapErr(err)
		}
	}
	if ownedProbe {
		defer process.Close()
	}

	var response map[string]any
	if err := process.client.Call(ctx, "account/rateLimits/read", map[string]any{}, &response); err != nil {
		return nil, process.wrapErr(err)
	}
	if rateLimits, ok := response["rateLimits"].(map[string]any); ok && len(rateLimits) > 0 {
		return rateLimits, nil
	}
	if buckets, ok := response["rateLimitsByLimitId"].(map[string]any); ok {
		if rateLimits, ok := buckets["codex"].(map[string]any); ok && len(rateLimits) > 0 {
			return rateLimits, nil
		}
		for _, value := range buckets {
			if rateLimits, ok := value.(map[string]any); ok && len(rateLimits) > 0 {
				return rateLimits, nil
			}
		}
	}
	return nil, errors.New("Codex returned no rate-limit windows")
}
