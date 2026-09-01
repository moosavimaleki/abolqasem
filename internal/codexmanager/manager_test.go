package codexmanager

import (
	"context"
	"testing"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/browser"
	"abolqasem/internal/codexmanager/maintenance"
	"abolqasem/internal/codexmanager/storage"
)

func TestManagerLifecycleAndAccountFacade(t *testing.T) {
	manager := New(storage.Paths{Home: t.TempDir()})
	if err := manager.Accounts.Add(context.Background(), "first", map[string]any{"tokens": map[string]any{"refresh_token": "refresh"}}, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Accounts.Activate(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx, &maintenance.Scheduler{Interval: time.Hour})
	manager.Shutdown()
	cancel()
	if active, err := manager.Accounts.Active(); err != nil || active != "first" {
		t.Fatalf("active=%q err=%v", active, err)
	}
	_ = account.PlanPlus
}

func TestManagerBrowserFacadeRequiresConfiguredClient(t *testing.T) {
	manager := New(storage.Paths{Home: t.TempDir()})
	if _, err := manager.BrowserDevices(context.Background()); err == nil {
		t.Fatal("expected unconfigured browser client error")
	}
	if err := manager.RevokeBrowserDevice(context.Background(), browser.Device{ID: "device"}); err == nil {
		t.Fatal("expected unconfigured browser client error")
	}
}
