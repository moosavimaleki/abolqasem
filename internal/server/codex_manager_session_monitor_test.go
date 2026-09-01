package server

import (
	"context"
	"testing"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/browser"
)

type fakeCodexManagerSessionMonitorClient struct {
	email   string
	devices []browser.Device
	revoked []string
}

func (f *fakeCodexManagerSessionMonitorClient) AccountEmail(context.Context) (string, error) {
	return f.email, nil
}

func (f *fakeCodexManagerSessionMonitorClient) Devices(context.Context) ([]browser.Device, error) {
	return append([]browser.Device(nil), f.devices...), nil
}

func (f *fakeCodexManagerSessionMonitorClient) Revoke(_ context.Context, device browser.Device) error {
	f.revoked = append(f.revoked, device.ID)
	return nil
}

func TestSessionMonitorStillReportsCountsWhenAutoCleanupIsDisabled(t *testing.T) {
	previousStateDir := codexManagerStateDir
	previousDiscover := codexManagerDiscoverChromeProfiles
	previousCookies := codexManagerLoadChromeCookies
	previousClient := codexManagerSessionClientFor
	stateDir := t.TempDir()
	codexManagerStateDir = func() string { return stateDir }
	profile := browser.Profile{ID: "chrome/Default"}
	client := &fakeCodexManagerSessionMonitorClient{
		email:   "user@example.com",
		devices: []browser.Device{{ID: "current", Current: true, HasCodex: true}, {ID: "extra", HasCodex: true}},
	}
	codexManagerDiscoverChromeProfiles = func(...string) ([]browser.Profile, error) { return []browser.Profile{profile}, nil }
	codexManagerLoadChromeCookies = func(context.Context, browser.Profile, browser.Decryptor) ([]browser.Cookie, error) {
		return []browser.Cookie{{Name: "session"}}, nil
	}
	codexManagerSessionClientFor = func([]browser.Cookie, string) codexManagerSessionMonitorClient { return client }
	t.Cleanup(func() {
		codexManagerStateDir = previousStateDir
		codexManagerDiscoverChromeProfiles = previousDiscover
		codexManagerLoadChromeCookies = previousCookies
		codexManagerSessionClientFor = previousClient
	})
	repository := account.Repository{Paths: codexManagerPaths()}
	if err := repository.Add(context.Background(), "personal", map[string]any{"email": "user@example.com", "tokens": map[string]any{"refresh_token": "refresh"}}, false); err != nil {
		t.Fatal(err)
	}
	if accounts := redactCodexManagerAccounts(repository); len(accounts) != 1 || accounts[0].Email != "user@example.com" {
		t.Fatalf("managed account was not discoverable: %#v", accounts)
	}
	if err := setCodexManagerSessionMonitorDisabled(context.Background(), repository, "personal", true); err != nil {
		t.Fatal(err)
	}
	result := runCodexManagerSessionMonitor(context.Background(), codexManagerSessionMonitorConfig{})
	if result.profiles != 1 || result.targets != 1 || result.revoked != 0 || len(client.revoked) != 0 {
		t.Fatalf("report-only monitor result: %#v revoked=%v", result, client.revoked)
	}
	accounts := redactCodexManagerAccounts(repository)
	if len(accounts) != 1 || accounts[0].SessionMonitor == nil || accounts[0].SessionMonitor.CodexSessions == nil || *accounts[0].SessionMonitor.CodexSessions != 2 || !accounts[0].SessionMonitor.RevocationDisabled {
		t.Fatalf("report-only session status: %#v", accounts)
	}
}

func TestSessionMonitorRevokesOnlyExtraSessionsAndProtectsCurrentDevice(t *testing.T) {
	previousStateDir := codexManagerStateDir
	previousDiscover := codexManagerDiscoverChromeProfiles
	previousCookies := codexManagerLoadChromeCookies
	previousClient := codexManagerSessionClientFor
	stateDir := t.TempDir()
	codexManagerStateDir = func() string { return stateDir }
	profile := browser.Profile{ID: "chrome/Default"}
	client := &fakeCodexManagerSessionMonitorClient{
		email: "user@example.com",
		devices: []browser.Device{
			{ID: "current", Current: true, HasCodex: true},
			{ID: "one", HasCodex: true},
			{ID: "two", HasCodex: true},
			{ID: "three", HasCodex: true},
		},
	}
	codexManagerDiscoverChromeProfiles = func(...string) ([]browser.Profile, error) { return []browser.Profile{profile}, nil }
	codexManagerLoadChromeCookies = func(context.Context, browser.Profile, browser.Decryptor) ([]browser.Cookie, error) {
		return []browser.Cookie{{Name: "session"}}, nil
	}
	codexManagerSessionClientFor = func([]browser.Cookie, string) codexManagerSessionMonitorClient { return client }
	t.Cleanup(func() {
		codexManagerStateDir = previousStateDir
		codexManagerDiscoverChromeProfiles = previousDiscover
		codexManagerLoadChromeCookies = previousCookies
		codexManagerSessionClientFor = previousClient
	})
	repository := account.Repository{Paths: codexManagerPaths()}
	if err := repository.Add(context.Background(), "personal", map[string]any{
		"email":  "user@example.com",
		"plan":   "free",
		"tokens": map[string]any{"refresh_token": "refresh"},
	}, false); err != nil {
		t.Fatal(err)
	}
	result := runCodexManagerSessionMonitor(context.Background(), codexManagerSessionMonitorConfig{})
	if result.profiles != 1 || result.accounts != 1 || result.targets != 3 || result.revoked != 3 {
		t.Fatalf("cleanup monitor result: profiles=%d accounts=%d targets=%d revoked=%d error=%q", result.profiles, result.accounts, result.targets, result.revoked, result.err)
	}
	if len(client.revoked) != 3 {
		t.Fatalf("only extra devices should be revoked: %#v", client.revoked)
	}
	accounts := redactCodexManagerAccounts(repository)
	if len(accounts) != 1 || accounts[0].SessionMonitor == nil || accounts[0].SessionMonitor.CodexSessions == nil || *accounts[0].SessionMonitor.CodexSessions != 4 || accounts[0].SessionMonitor.ExcessCodexSessions != 3 || accounts[0].SessionMonitor.RevokedTotal != 3 || !accounts[0].SessionMonitor.CurrentDeviceProtected {
		t.Fatalf("per-account monitor projection = %#v", accounts)
	}
}
