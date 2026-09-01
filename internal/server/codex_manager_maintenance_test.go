package server

import (
	"context"
	"testing"
	"time"

	"abolqasem/internal/codexmanager"
	"abolqasem/internal/codexmanager/storage"
)

func TestStartCodexManagerMaintenanceConfiguresActiveAccountRefresh(t *testing.T) {
	previous := newCodexManagerMaintenanceManager
	manager := codexmanager.New(storage.Paths{Home: t.TempDir()})
	newCodexManagerMaintenanceManager = func(storage.Paths) *codexmanager.Manager { return manager }
	t.Cleanup(func() { newCodexManagerMaintenanceManager = previous })

	ctx, cancel := context.WithCancel(context.Background())
	stop := startCodexManagerMaintenance(ctx)
	if !manager.Maintenance.Config.IncludeActive {
		t.Fatal("background maintenance must refresh the selected managed account")
	}
	if manager.Maintenance.Config.Retention != codexManagerHistoryRetention {
		t.Fatalf("retention=%s", manager.Maintenance.Config.Retention)
	}
	stop()
	cancel()
}

func TestCodexManagerMaintenanceRefreshesAccountsEveryFifteenMinutes(t *testing.T) {
	if codexManagerMaintenanceInterval != 15*time.Minute {
		t.Fatalf("interval=%s", codexManagerMaintenanceInterval)
	}
	if codexManagerMaintenanceJitter != 5*time.Minute {
		t.Fatalf("jitter=%s", codexManagerMaintenanceJitter)
	}
}
