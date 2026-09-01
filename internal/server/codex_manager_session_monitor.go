package server

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/browser"
	"abolqasem/internal/codexmanager/storage"
	"abolqasem/internal/state"
)

const codexManagerSessionMonitorPoll = time.Minute

type codexManagerSessionMonitorConfig struct {
	Enabled    bool
	Interval   time.Duration
	DryRun     bool
	ProxyURL   string
	ChromeRoot string
}

type codexManagerSessionMonitorClient interface {
	AccountEmail(context.Context) (string, error)
	browser.DeviceClient
}

var (
	codexManagerDiscoverChromeProfiles = browser.DiscoverProfiles
	codexManagerLoadChromeCookies      = browser.LoadChatGPTCookies
	codexManagerSessionClientFor       = func(cookies []browser.Cookie, proxyURL string) codexManagerSessionMonitorClient {
		return browser.SessionClient{Cookies: cookies, ProxyURL: proxyURL}
	}
)

var codexManagerSessionMonitorRuntime = struct {
	sync.RWMutex
	Enabled         bool
	Running         bool
	DryRun          bool
	NextRun         time.Time
	LastRun         time.Time
	LastError       string
	ProfilesChecked int
	AccountsChecked int
	Targets         int
	Revoked         int
}{}

func codexManagerSessionMonitorStatus() map[string]any {
	codexManagerSessionMonitorRuntime.RLock()
	defer codexManagerSessionMonitorRuntime.RUnlock()
	return map[string]any{
		"enabled":         codexManagerSessionMonitorRuntime.Enabled,
		"running":         codexManagerSessionMonitorRuntime.Running,
		"dryRun":          codexManagerSessionMonitorRuntime.DryRun,
		"nextRun":         codexManagerSessionMonitorRuntime.NextRun,
		"lastRun":         codexManagerSessionMonitorRuntime.LastRun,
		"lastError":       codexManagerSessionMonitorRuntime.LastError,
		"profilesChecked": codexManagerSessionMonitorRuntime.ProfilesChecked,
		"accountsChecked": codexManagerSessionMonitorRuntime.AccountsChecked,
		"targets":         codexManagerSessionMonitorRuntime.Targets,
		"revoked":         codexManagerSessionMonitorRuntime.Revoked,
	}
}

func setCodexManagerSessionMonitorStatus(config codexManagerSessionMonitorConfig, running bool, nextRun, lastRun time.Time, lastError string, profiles, accounts, targets, revoked int) {
	codexManagerSessionMonitorRuntime.Lock()
	codexManagerSessionMonitorRuntime.Enabled = config.Enabled
	codexManagerSessionMonitorRuntime.Running = running
	codexManagerSessionMonitorRuntime.DryRun = config.DryRun
	codexManagerSessionMonitorRuntime.NextRun = nextRun
	if !lastRun.IsZero() {
		codexManagerSessionMonitorRuntime.LastRun = lastRun
	}
	if lastError != "" || !lastRun.IsZero() {
		codexManagerSessionMonitorRuntime.LastError = lastError
	}
	if profiles >= 0 {
		codexManagerSessionMonitorRuntime.ProfilesChecked = profiles
		codexManagerSessionMonitorRuntime.AccountsChecked = accounts
		codexManagerSessionMonitorRuntime.Targets = targets
		codexManagerSessionMonitorRuntime.Revoked = revoked
	}
	codexManagerSessionMonitorRuntime.Unlock()
}

func loadCodexManagerSessionMonitorConfig() codexManagerSessionMonitorConfig {
	settings, err := state.LoadSettings()
	if err != nil {
		settings = state.DefaultAppSettings()
	}
	value := settings.CodexBackend.SessionMonitor
	return codexManagerSessionMonitorConfig{
		// Cleanup is controlled on each managed account. Older global settings
		// are intentionally ignored here so a stale global dry-run flag cannot
		// make an account switch appear enabled while silently doing nothing.
		Enabled:    true,
		Interval:   time.Duration(value.IntervalSeconds) * time.Second,
		DryRun:     false,
		ProxyURL:   settings.CodexBackend.Maintenance.ProxyURL,
		ChromeRoot: value.ChromeRoot,
	}
}

// startCodexManagerSessionMonitor always records the safe session counts for
// managed Chrome profiles. The user-facing switch controls automatic revoke,
// not visibility: turning cleanup off must never erase the last known counts.
func startCodexManagerSessionMonitor(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer func() {
			config := loadCodexManagerSessionMonitorConfig()
			setCodexManagerSessionMonitorStatus(config, false, time.Time{}, time.Time{}, "", -1, -1, -1, -1)
		}()
		ticker := time.NewTicker(codexManagerSessionMonitorPoll)
		defer ticker.Stop()
		config := loadCodexManagerSessionMonitorConfig()
		var nextRun time.Time
		for {
			if nextRun.IsZero() {
				nextRun = time.Now().Add(config.Interval)
			}
			setCodexManagerSessionMonitorStatus(config, true, nextRun, time.Time{}, "", -1, -1, -1, -1)
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				next := loadCodexManagerSessionMonitorConfig()
				if next != config {
					config = next
					nextRun = time.Time{}
					continue
				}
				if now.Before(nextRun) {
					continue
				}
				result := runCodexManagerSessionMonitor(ctx, config)
				nextRun = time.Now().Add(config.Interval)
				setCodexManagerSessionMonitorStatus(config, true, nextRun, time.Now().UTC(), result.err, result.profiles, result.accounts, result.targets, result.revoked)
			}
		}
	}()
	return func() {
		cancel()
		wait.Wait()
	}
}

type codexManagerSessionMonitorResult struct {
	profiles int
	accounts int
	targets  int
	revoked  int
	err      string
}

func runCodexManagerSessionMonitor(ctx context.Context, config codexManagerSessionMonitorConfig) codexManagerSessionMonitorResult {
	roots := []string(nil)
	if config.ChromeRoot != "" {
		roots = []string{config.ChromeRoot}
	}
	profiles, err := codexManagerDiscoverChromeProfiles(roots...)
	if err != nil {
		return codexManagerSessionMonitorResult{err: "could not discover Chrome profiles"}
	}
	managed := codexManagerAccountsByEmail()
	result := codexManagerSessionMonitorResult{}
	for _, profile := range profiles {
		if err := ctx.Err(); err != nil {
			result.err = "session monitor was cancelled"
			return result
		}
		result.profiles++
		cookies, cookieErr := codexManagerLoadChromeCookies(ctx, profile, nil)
		if cookieErr != nil {
			result.err = safeCodexManagerSessionMonitorError(cookieErr)
			continue
		}
		client := codexManagerSessionClientFor(cookies, config.ProxyURL)
		email, emailErr := client.AccountEmail(ctx)
		if emailErr != nil {
			result.err = safeCodexManagerSessionMonitorError(emailErr)
			continue
		}
		managedAccount, found := managed[strings.ToLower(strings.TrimSpace(email))]
		if !found {
			// A Chrome account not in this manager is intentionally ignored.
			continue
		}
		result.accounts++
		disabled := codexManagerSessionMonitorDisabled(managedAccount.Name)
		cleanup := browser.Cleanup(ctx, managedAccount, client, browser.CleanupPolicy{
			Enabled:        true,
			DryRun:         disabled,
			MaxCodexByPlan: map[account.Plan]int{managedAccount.Plan: defaultCodexManagerSessionLimit(managedAccount.Plan)},
			DisabledFor:    map[string]bool{managedAccount.Name: disabled},
		})
		result.targets += len(cleanup.Targets)
		result.revoked += len(cleanup.Revoked)
		if cleanup.Error != "" {
			result.err = safeCodexManagerSessionMonitorError(errors.New(cleanup.Error))
			_ = recordCodexManagerSessionMonitorStatus(managedAccount.Name, cleanup, disabled, cleanup.Error)
			continue
		}
		_ = recordCodexManagerSessionMonitorStatus(managedAccount.Name, cleanup, disabled, "")
	}
	return result
}

// recordCodexManagerSessionMonitorStatus keeps the same small, per-account
// audit projection the former TUI showed: current Codex session count, the
// latest cleanup result, and an accumulating revoke total. Credentials,
// cookies, device IDs, and Chrome paths never enter this file.
func recordCodexManagerSessionMonitorStatus(name string, cleanup browser.CleanupResult, preview bool, monitorErr string) error {
	paths := codexManagerPaths()
	path, err := paths.Status(name)
	if err != nil {
		return err
	}
	var status map[string]any
	if err := storage.ReadJSON(path, &status); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if status == nil {
		status = map[string]any{}
	}
	previousTotal := 0
	previousHistory := make([]any, 0, 3)
	if previous, ok := status["session_monitor"].(map[string]any); ok {
		switch value := previous["revokedTotal"].(type) {
		case float64:
			previousTotal = int(value)
		case int:
			previousTotal = value
		}
		if history, ok := previous["checkHistory"].([]any); ok {
			previousHistory = append(previousHistory, history...)
		}
	}
	now := time.Now().UTC()
	entry := map[string]any{
		"lastCheckedAt":          now,
		"codexSessions":          cleanup.CodexSessions,
		"excessCodexSessions":    len(cleanup.Targets),
		"revokedLastRun":         len(cleanup.Revoked),
		"revokedTotal":           previousTotal + len(cleanup.Revoked),
		"revocationDisabled":     preview,
		"currentDeviceProtected": cleanup.CurrentDeviceProtected,
		"outcome":                "ok",
	}
	if monitorErr != "" {
		entry["outcome"] = "error"
		entry["error"] = safeCodexManagerSessionMonitorError(errors.New(monitorErr))
	}
	check := map[string]any{
		"checkedAt":           now,
		"codexSessions":       cleanup.CodexSessions,
		"excessCodexSessions": len(cleanup.Targets),
		"revokedLastRun":      len(cleanup.Revoked),
		"outcome":             entry["outcome"],
	}
	if monitorErr != "" {
		check["error"] = entry["error"]
	}
	entry["checkHistory"] = append([]any{check}, previousHistory[:min(len(previousHistory), 2)]...)
	status["session_monitor"] = entry
	return storage.WriteJSON(paths, path, status)
}

func codexManagerSessionMonitorDisabled(name string) bool {
	path, err := codexManagerPaths().Status(name)
	if err != nil {
		return false
	}
	var status struct {
		Disabled bool `json:"session_monitor_disabled"`
	}
	return storage.ReadJSON(path, &status) == nil && status.Disabled
}

func codexManagerAccountsByEmail() map[string]account.Account {
	accounts := make(map[string]account.Account)
	for _, item := range redactCodexManagerAccounts(account.Repository{Paths: codexManagerPaths()}) {
		if email := strings.ToLower(strings.TrimSpace(item.Email)); email != "" {
			accounts[email] = item
		}
	}
	return accounts
}

func safeCodexManagerSessionMonitorError(err error) string {
	if errors.Is(err, browser.ErrEncryptedCookie) {
		return "unlock the selected Chrome profile and retry"
	}
	if errors.Is(err, browser.ErrNotSignedIn) {
		return "sign in to ChatGPT in the selected Chrome profile and retry"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Chrome session check timed out; it will retry later"
	}
	return "could not inspect a Chrome session; open and unlock the profile, then retry"
}
