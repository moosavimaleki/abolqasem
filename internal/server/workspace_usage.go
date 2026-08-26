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
	RateLimits any                            `json:"rate_limits"`
	Account    *workspaceCodexAccountSnapshot `json:"account,omitempty"`
	UpdatedAt  time.Time                      `json:"updated_at"`
}

type workspaceCodexAccountSnapshot struct {
	Type     string `json:"type,omitempty"`
	Email    string `json:"email,omitempty"`
	PlanType string `json:"plan_type,omitempty"`
}

var (
	workspaceUsageMu          sync.Mutex
	workspaceReadCodexUsage   = readWorkspaceCodexRateLimits
	workspaceReadCodexAccount = readWorkspaceCodexAccount
	workspaceUsageCachePath   = func() string {
		return filepath.Join(state.GetStateDir(), "usage.json")
	}
)

func workspaceStoreCodexUsage(rateLimits any) {
	if rateLimits == nil {
		return
	}
	workspaceUsageMu.Lock()
	defer workspaceUsageMu.Unlock()

	snapshot := workspaceLoadUsageUnlocked()
	if snapshot.Codex == nil {
		snapshot.Codex = &workspaceCodexUsageSnapshot{}
	}
	snapshot.Codex.RateLimits = rateLimits
	snapshot.Codex.UpdatedAt = time.Now().UTC()
	workspaceWriteUsageUnlocked(snapshot)
}

func workspaceStoreCodexAccount(account *workspaceCodexAccountSnapshot) {
	if account == nil {
		return
	}
	workspaceUsageMu.Lock()
	defer workspaceUsageMu.Unlock()

	snapshot := workspaceLoadUsageUnlocked()
	if snapshot.Codex == nil {
		snapshot.Codex = &workspaceCodexUsageSnapshot{}
	}
	snapshot.Codex.Account = account
	if snapshot.Codex.UpdatedAt.IsZero() {
		snapshot.Codex.UpdatedAt = time.Now().UTC()
	}
	workspaceWriteUsageUnlocked(snapshot)
}

func workspaceWriteUsageUnlocked(snapshot workspaceUsageSnapshot) {
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
	return workspaceLoadUsageUnlocked()
}

func workspaceLoadUsageUnlocked() workspaceUsageSnapshot {
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
	if account, accountErr := workspaceReadCodexAccount(ctx); accountErr == nil {
		workspaceStoreCodexAccount(account)
	}
	writeJSON(w, workspaceLoadUsage())
}

func readWorkspaceCodexAccount(ctx context.Context) (*workspaceCodexAccountSnapshot, error) {
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
	if err := process.client.Call(ctx, "account/read", map[string]any{"refreshToken": false}, &response); err != nil {
		return nil, process.wrapErr(err)
	}
	account, ok := response["account"].(map[string]any)
	if !ok || len(account) == 0 {
		return &workspaceCodexAccountSnapshot{}, nil
	}
	return &workspaceCodexAccountSnapshot{
		Type:     workspaceUsageString(account["type"]),
		Email:    workspaceUsageString(account["email"]),
		PlanType: workspaceUsageString(account["planType"]),
	}, nil
}

func workspaceUsageString(value any) string {
	text, _ := value.(string)
	return text
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
