package server

import (
	"context"
	"path/filepath"

	"abolqasem/internal/codexmanager"
	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/maintenance"
)

var codexManagerLiveAuthRoot = workspaceCodexRootDir

var startCodexManagerPostSwitchCheck = checkCodexManagerAccountAfterSwitch

func codexManagerLiveAuthPath() string { return filepath.Join(codexManagerLiveAuthRoot(), "auth.json") }

// syncCodexManagerLiveAccount imports the active native Codex identity the
// first time it is seen and promotes tokens only if the live file is newer.
func syncCodexManagerLiveAccount(ctx context.Context, repository account.Repository) (account.LiveSyncResult, error) {
	return repository.SyncLive(ctx, codexManagerLiveAuthPath())
}

func newCodexManagerCheckManager() *codexmanager.Manager {
	manager := codexmanager.New(codexManagerPaths())
	config := loadCodexManagerMaintenanceConfig()
	manager.Maintenance.Config = maintenance.Config{IncludeActive: true, Retention: config.Retention, ProxyURL: config.ProxyURL}
	manager.BeforeCheck = func(checkCtx context.Context) error {
		_, err := syncCodexManagerLiveAccount(checkCtx, manager.Accounts)
		return err
	}
	return manager
}

// switchCodexManagerLiveAccount makes manual account switching transactional
// from the user's perspective: no running turn is disrupted; live auth.json
// is first reconciled, then replaced, then every idle app-server is reset so
// the next request necessarily starts under the newly selected identity.
func switchCodexManagerLiveAccount(ctx context.Context, repository account.Repository, name string) error {
	workspaceCodexCredentialSwitch.Lock()
	defer workspaceCodexCredentialSwitch.Unlock()
	// This is local reconciliation only. It preserves a refresh token Codex
	// rotated in auth.json, but must never make switching away from a broken
	// account impossible.
	_, _ = syncCodexManagerLiveAccount(ctx, repository)
	if err := repository.ActivateLive(ctx, name, codexManagerLiveAuthPath()); err != nil {
		return err
	}
	workspaceCodexSessions.resetForCredentialSwitch()
	// Network refresh/quota checks intentionally happen after auth.json is
	// ready and outside the request path. New sessions can start immediately.
	go startCodexManagerPostSwitchCheck(name)
	return nil
}

func checkCodexManagerAccountAfterSwitch(name string) {
	manager := newCodexManagerCheckManager()
	manager.Maintenance.Config.IncludeActive = true
	manager.Maintenance.Config.Accounts = []string{name}
	_, _ = manager.Check(context.Background())
}
