package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/browser"
	"abolqasem/internal/codexmanager/storage"
	"abolqasem/internal/state"
)

// Chrome sign-in state is useful even when the account cleanup monitor is
// disabled. The scan has a fixed, conservative cadence: it keeps the profile
// table accurate without giving the user another scheduling knob to manage.
const codexManagerChromeScanInterval = 15 * time.Minute

type codexManagerChromeScanSnapshot struct {
	Profiles  []map[string]any `json:"profiles"`
	ScannedAt time.Time        `json:"scannedAt"`
}

var codexManagerChromeScanRuntime = struct {
	sync.RWMutex
	Snapshot codexManagerChromeScanSnapshot
	Loaded   bool
}{}

func codexManagerChromeScanPath() string {
	return filepath.Join(codexManagerPaths().Home, "chrome-scan.json")
}

func codexManagerChromeScanCached() (codexManagerChromeScanSnapshot, bool) {
	codexManagerChromeScanRuntime.RLock()
	if codexManagerChromeScanRuntime.Loaded && !codexManagerChromeScanRuntime.Snapshot.ScannedAt.IsZero() {
		value := codexManagerChromeScanRuntime.Snapshot
		codexManagerChromeScanRuntime.RUnlock()
		return value, true
	}
	codexManagerChromeScanRuntime.RUnlock()

	var value codexManagerChromeScanSnapshot
	if err := storage.ReadJSON(codexManagerChromeScanPath(), &value); err != nil || value.ScannedAt.IsZero() {
		return codexManagerChromeScanSnapshot{}, false
	}
	codexManagerChromeScanRuntime.Lock()
	if !codexManagerChromeScanRuntime.Loaded || codexManagerChromeScanRuntime.Snapshot.ScannedAt.Before(value.ScannedAt) {
		codexManagerChromeScanRuntime.Snapshot = value
	}
	codexManagerChromeScanRuntime.Loaded = true
	value = codexManagerChromeScanRuntime.Snapshot
	codexManagerChromeScanRuntime.Unlock()
	return value, true
}

func codexManagerStoreChromeScan(value codexManagerChromeScanSnapshot) {
	codexManagerChromeScanRuntime.Lock()
	codexManagerChromeScanRuntime.Snapshot = value
	codexManagerChromeScanRuntime.Loaded = true
	codexManagerChromeScanRuntime.Unlock()
	// A cache write failure must never discard a successful in-memory scan.
	_ = storage.WriteJSON(codexManagerPaths(), codexManagerChromeScanPath(), value)
}

// startCodexManagerChromeScanWorker refreshes the status table shortly after
// startup and then every fifteen minutes. It has no user-controlled cleanup
// behavior: it only reads local cookies and the current ChatGPT sign-in.
func startCodexManagerChromeScanWorker(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		run := func() {
			scanCtx, scanCancel := context.WithTimeout(ctx, 45*time.Second)
			defer scanCancel()
			_, _ = codexManagerRefreshChromeScan(scanCtx)
		}
		// Do the initial scan asynchronously. Server readiness and chat loading
		// must not wait for potentially locked Chrome profiles.
		run()
		ticker := time.NewTicker(codexManagerChromeScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return func() {
		cancel()
		wait.Wait()
	}
}

func codexManagerRefreshChromeScan(ctx context.Context) (codexManagerChromeScanSnapshot, error) {
	profiles, err := codexManagerBrowserProfiles()
	if err != nil {
		return codexManagerChromeScanSnapshot{}, err
	}
	settings, err := state.LoadSettings()
	if err != nil {
		settings = state.DefaultAppSettings()
	}
	managed := make(map[string]account.Account)
	for _, item := range redactCodexManagerAccounts(account.Repository{Paths: codexManagerPaths()}) {
		if email := strings.ToLower(strings.TrimSpace(item.Email)); email != "" {
			managed[email] = item
		}
	}
	items := make([]map[string]any, len(profiles))
	semaphore := make(chan struct{}, 3)
	var wait sync.WaitGroup
	for index, profile := range profiles {
		wait.Add(1)
		go func(index int, profile browser.Profile) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				items[index] = map[string]any{"id": profile.ID, "name": profile.Name, "outcome": "error", "reason": "scan cancelled"}
				return
			}
			defer func() { <-semaphore }()
			items[index] = codexManagerScanChromeProfile(ctx, profile, managed, settings.CodexBackend.Maintenance.ProxyURL)
		}(index, profile)
	}
	wait.Wait()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return codexManagerChromeScanSnapshot{}, err
	}
	value := codexManagerChromeScanSnapshot{Profiles: items, ScannedAt: time.Now().UTC()}
	if err := codexManagerStoreChromeProfileAssociations(value); err != nil {
		return codexManagerChromeScanSnapshot{}, err
	}
	codexManagerStoreChromeScan(value)
	return value, nil
}

// codexManagerStoreChromeProfileAssociations retains the last profile known
// for every managed account. A subsequent scan updates its outcome even if a
// different ChatGPT account is now signed in, which makes a broken/moved
// profile visible instead of making the association disappear.
func codexManagerStoreChromeProfileAssociations(snapshot codexManagerChromeScanSnapshot) error {
	paths := codexManagerPaths()
	repository := account.Repository{Paths: paths}
	accounts := redactCodexManagerAccounts(repository)
	profilesByID := make(map[string]map[string]any, len(snapshot.Profiles))
	linked := make(map[string]map[string]any)
	for _, profile := range snapshot.Profiles {
		id, _ := profile["id"].(string)
		if id != "" {
			profilesByID[id] = profile
		}
		if name, _ := profile["managedAccount"].(string); name != "" {
			linked[name] = profile
		}
	}
	for _, item := range accounts {
		path, err := paths.Status(item.Name)
		if err != nil {
			continue
		}
		var status map[string]any
		if err := storage.ReadJSON(path, &status); err != nil {
			// A missing status file is expected for a newly imported account;
			// a malformed existing status must not be silently overwritten.
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			status = map[string]any{}
		}
		if status == nil {
			status = map[string]any{}
		}
		previous, _ := status["chrome_profile"].(map[string]any)
		profile := linked[item.Name]
		if profile == nil && previous != nil {
			if id, _ := previous["id"].(string); id != "" {
				profile = profilesByID[id]
			}
		}
		if profile == nil {
			if previous == nil {
				continue
			}
			previous["outcome"] = "missing"
			previous["lastCheckedAt"] = snapshot.ScannedAt
			status["chrome_profile"] = previous
			if err := storage.WriteJSON(paths, path, status); err != nil {
				return err
			}
			continue
		}
		id, _ := profile["id"].(string)
		name, _ := profile["name"].(string)
		outcome, _ := profile["outcome"].(string)
		activeEmail, _ := profile["activeEmail"].(string)
		if linked[item.Name] == nil && outcome == "signed_in" {
			// The profile still works, but it is signed in as somebody else. Keep
			// the historical association and make the mismatch explicit to the UI.
			outcome = "changed"
		}
		entry := map[string]any{"id": id, "name": name, "outcome": outcome, "activeEmail": activeEmail, "lastCheckedAt": snapshot.ScannedAt}
		if linked[item.Name] != nil {
			entry["lastSeenAt"] = snapshot.ScannedAt
		} else if previous != nil {
			entry["lastSeenAt"] = previous["lastSeenAt"]
		}
		status["chrome_profile"] = entry
		if err := storage.WriteJSON(paths, path, status); err != nil {
			return err
		}
	}
	return nil
}
