package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"abolqasem/internal/codexmanager"
	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/auth"
	"abolqasem/internal/codexmanager/browser"
	"abolqasem/internal/codexmanager/history"
	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/migrate"
	"abolqasem/internal/codexmanager/recommendation"
	"abolqasem/internal/codexmanager/storage"
	"abolqasem/internal/secrets"
	"abolqasem/internal/state"
)

func handleAPICodexManager(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/codex-manager"), "/")
	switch action {
	case "":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, codexManagerSnapshot())
	case "activate":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		status, err := activateCodexManager(ctx)
		if err != nil {
			http.Error(w, "Could not activate Codex Manager: "+err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"status": status, "snapshot": codexManagerSnapshot()})
	case "disable":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := disableCodexManager(); err != nil {
			http.Error(w, "Could not disable Codex Manager", http.StatusInternalServerError)
			return
		}
		writeJSON(w, codexManagerSnapshot())
	case "check":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()
		manager := newCodexManagerCheckManager()
		// This is an explicit user action from the account dashboard. Unlike
		// background maintenance, it must include the selected account too so
		// the displayed balance is never silently stale.
		manager.Maintenance.Config.IncludeActive = true
		summary, err := manager.Check(ctx)
		if err != nil {
			http.Error(w, "Could not refresh account status: "+err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, summary)
	case "accounts":
		handleAPICodexManagerAccounts(w, r)
	case "recommendation":
		if r.Method == http.MethodPost {
			handleAPICodexManagerActivateBest(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, codexManagerRecommendation())
	case "history":
		handleAPICodexManagerHistory(w, r)
	case "migration":
		handleAPICodexManagerMigration(w, r)
	case "browser/profiles":
		handleAPICodexManagerBrowserProfiles(w, r)
	case "browser/scan":
		handleAPICodexManagerBrowserScan(w, r)
	case "browser/profiles/open":
		handleAPICodexManagerBrowserProfileAction(w, r)
	case "browser/devices":
		handleAPICodexManagerBrowserDevices(w, r, false)
	case "browser/devices/revoke":
		handleAPICodexManagerBrowserDevices(w, r, true)
	case "browser/cleanup":
		handleAPICodexManagerBrowserCleanup(w, r)
	case "login":
		handleAPICodexManagerLogin(w, r, "")
	default:
		if strings.HasPrefix(action, "accounts/") {
			handleAPICodexManagerAccountAction(w, r, strings.TrimPrefix(action, "accounts/"))
			return
		}
		if strings.HasPrefix(action, "login/") {
			handleAPICodexManagerLogin(w, r, strings.TrimPrefix(action, "login/"))
			return
		}
		http.NotFound(w, r)
	}
}

// handleAPICodexManagerBrowserProfileAction opens only a discovered local
// profile. The profile ID is resolved server-side, so a web request can never
// provide an arbitrary executable argument or filesystem path.
func handleAPICodexManagerBrowserProfileAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	profile, ok := codexManagerBrowserProfile(strings.TrimSpace(r.URL.Query().Get("profileId")))
	if !ok {
		http.Error(w, "Chrome profile was not found", http.StatusNotFound)
		return
	}
	if err := codexManagerOpenChromeProfile(profile); err != nil {
		http.Error(w, "Could not open Chrome profile: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]bool{"opened": true})
}

var codexManagerOpenChromeProfile = openCodexManagerChromeProfile

func openCodexManagerChromeProfile(profile browser.Profile) error {
	argument := "--profile-directory=" + profile.Directory
	var commands [][]string
	switch runtime.GOOS {
	case "darwin":
		commands = [][]string{{"open", "-a", "Google Chrome", "--args", argument}}
	case "windows":
		commands = [][]string{{"cmd", "/c", "start", "", "chrome", argument}}
	default:
		commands = [][]string{{"google-chrome", argument}, {"google-chrome-stable", argument}, {"chromium", argument}, {"chromium-browser", argument}}
	}
	var lastErr error
	for _, command := range commands {
		if _, err := exec.LookPath(command[0]); err != nil {
			lastErr = err
			continue
		}
		if err := exec.Command(command[0], command[1:]...).Start(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		return fmt.Errorf("Chrome executable was not found")
	}
	return lastErr
}

func handleAPICodexManagerMigration(w http.ResponseWriter, r *http.Request) {
	target := codexManagerPaths()
	if r.Method == http.MethodGet {
		items := make([]migrate.Plan, 0)
		for _, source := range migrate.Candidates("") {
			if plan, err := migrate.BuildPlan(source, target); err == nil && len(plan.Files) > 0 {
				items = append(items, plan)
			}
		}
		writeJSON(w, map[string]any{"plans": items})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Source string `json:"source"`
		DryRun bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&payload); err != nil {
		http.Error(w, "Invalid migration JSON", http.StatusBadRequest)
		return
	}
	plan, err := migrate.BuildPlan(payload.Source, target)
	if err != nil {
		http.Error(w, "Could not inspect migration source: "+err.Error(), http.StatusBadRequest)
		return
	}
	if payload.DryRun {
		writeJSON(w, map[string]any{"plan": plan, "copied": []string{}})
		return
	}
	copied, err := migrate.Import(r.Context(), plan, target)
	if err != nil {
		http.Error(w, "Migration failed after copying some files: "+err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"plan": plan, "copied": copied})
}

// handleAPICodexManagerBrowserProfiles intentionally exposes only the profile
// label and known account association. Browser paths, cookies, and tokens stay
// local and are never serialized to the web client.
func handleAPICodexManagerBrowserProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// A full sign-in scan is intentionally performed in the background: it can
	// make one network request per profile. Returning its last safe result keeps
	// the page fast and, crucially, avoids presenting every profile as signed out
	// until a user presses the manual scan button.
	if cached, ok := codexManagerChromeScanCached(); ok {
		writeJSON(w, map[string]any{"profiles": cached.Profiles, "scannedAt": cached.ScannedAt})
		return
	}
	profiles, err := codexManagerBrowserProfiles()
	if err != nil {
		http.Error(w, "Could not inspect Chrome profiles", http.StatusInternalServerError)
		return
	}
	items := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, map[string]any{
			"id": profile.ID, "name": profile.Name, "outcome": "pending",
			"reason": "background Chrome scan is in progress",
		})
	}
	writeJSON(w, map[string]any{"profiles": items})
}

// handleAPICodexManagerBrowserScan performs the fuller, explicit Chrome scan
// from the former TUI. It is deliberately separate from /browser/profiles:
// listing profiles is cheap, whereas verifying a ChatGPT login can make a
// network request for every profile.
func handleAPICodexManagerBrowserScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	cached, err := codexManagerRefreshChromeScan(ctx)
	if err != nil {
		http.Error(w, "Could not inspect Chrome profiles: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"profiles": cached.Profiles, "scannedAt": cached.ScannedAt})
}

func codexManagerScanChromeProfile(ctx context.Context, profile browser.Profile, managed map[string]account.Account, proxyURL string) map[string]any {
	savedAccounts, _ := browser.DiscoverSwitchAccounts(profile)
	result := map[string]any{
		"id":            profile.ID,
		"name":          profile.Name,
		"savedAccounts": savedAccounts,
		"outcome":       "signed_out",
	}
	cookies, err := browser.LoadChatGPTCookies(ctx, profile, nil)
	if err != nil {
		result["outcome"] = "error"
		if errors.Is(err, browser.ErrEncryptedCookie) {
			result["reason"] = "unlock the selected Chrome profile and scan again"
		} else {
			result["reason"] = "could not read the selected Chrome profile"
		}
		return result
	}
	if len(cookies) == 0 {
		result["reason"] = "no active ChatGPT sign-in"
		return result
	}
	email, err := (browser.SessionClient{Cookies: cookies, ProxyURL: proxyURL}).AccountEmail(ctx)
	if err != nil {
		if errors.Is(err, browser.ErrNotSignedIn) {
			result["outcome"] = "partial"
			result["reason"] = "ChatGPT cookies exist but the sign-in is incomplete"
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result["outcome"] = "error"
			result["reason"] = "the sign-in check timed out"
		} else {
			result["outcome"] = "error"
			result["reason"] = "could not verify the ChatGPT sign-in"
		}
		return result
	}
	result["outcome"] = "signed_in"
	result["activeEmail"] = email
	if item, found := managed[strings.ToLower(strings.TrimSpace(email))]; found {
		result["managedAccount"] = item.Name
		result["managedPlan"] = item.Plan
	}
	return result
}

func codexManagerBrowserProfiles() ([]browser.Profile, error) {
	settings, err := state.LoadSettings()
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(settings.CodexBackend.SessionMonitor.ChromeRoot)
	if root == "" {
		return browser.DiscoverProfiles()
	}
	return browser.DiscoverProfiles(root)
}

func codexManagerPaths() storage.Paths {
	return storage.Paths{Home: filepath.Join(codexManagerStateDir(), "codex-manager")}
}

var codexManagerStateDir = state.GetStateDir

func handleAPICodexManagerAccounts(w http.ResponseWriter, r *http.Request) {
	repository := account.Repository{Paths: codexManagerPaths()}
	if r.Method == http.MethodGet {
		if _, err := syncCodexManagerLiveAccount(r.Context(), repository); err != nil {
			// A sync problem must not hide the inventory needed to resolve it.
			// In particular, duplicate imports were previously returned as 409,
			// leaving the Accounts UI empty with no way to remove the duplicate.
			payload := map[string]any{
				"accounts": redactCodexManagerAccounts(repository),
				"sync":     map[string]any{"error": err.Error()},
			}
			writeJSON(w, payload)
			return
		}
		writeJSON(w, map[string]any{"accounts": redactCodexManagerAccounts(repository)})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Name        string         `json:"name"`
		Credentials map[string]any `json:"credentials"`
		Activate    bool           `json:"activate"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&payload); err != nil {
		http.Error(w, "Invalid account JSON", http.StatusBadRequest)
		return
	}
	if err := repository.Add(r.Context(), payload.Name, payload.Credentials, false); err != nil {
		http.Error(w, "Could not add account: "+err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Activate {
		if err := switchCodexManagerLiveAccount(r.Context(), repository, payload.Name); err != nil {
			http.Error(w, "Account was added but could not be activated", http.StatusConflict)
			return
		}
	}
	writeJSON(w, map[string]any{"accounts": redactCodexManagerAccounts(repository)})
}

func handleAPICodexManagerAccountAction(w http.ResponseWriter, r *http.Request, rawAction string) {
	parts := strings.Split(strings.Trim(rawAction, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	name, action := parts[0], parts[1]
	repository := account.Repository{Paths: codexManagerPaths()}
	switch {
	case r.Method == http.MethodPost && action == "check":
		var payload struct {
			ForceRefresh bool `json:"forceRefresh"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&payload); err != nil && err.Error() != "EOF" {
			http.Error(w, "Invalid check JSON", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()
		manager := newCodexManagerCheckManager()
		manager.Maintenance.Config.IncludeActive = true
		manager.Maintenance.Config.ForceRefresh = payload.ForceRefresh
		manager.Maintenance.Config.Accounts = []string{name}
		summary, err := manager.Check(ctx)
		if err != nil {
			http.Error(w, "Could not refresh account status: "+err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, summary)
		return
	case r.Method == http.MethodPost && action == "activate":
		if err := switchCodexManagerLiveAccount(r.Context(), repository, name); err != nil {
			http.Error(w, "Could not activate account: "+err.Error(), http.StatusBadRequest)
			return
		}
	case r.Method == http.MethodPost && action == "session-monitor":
		var payload struct {
			Disabled bool `json:"disabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&payload); err != nil {
			http.Error(w, "Invalid session monitor JSON", http.StatusBadRequest)
			return
		}
		if err := setCodexManagerSessionMonitorDisabled(r.Context(), repository, name, payload.Disabled); err != nil {
			http.Error(w, "Could not update session monitoring: "+err.Error(), http.StatusBadRequest)
			return
		}
	case r.Method == http.MethodPost && action == "rename":
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&payload); err != nil {
			http.Error(w, "Invalid rename JSON", http.StatusBadRequest)
			return
		}
		if err := repository.Rename(r.Context(), name, payload.Name); err != nil {
			http.Error(w, "Could not rename account: "+err.Error(), http.StatusBadRequest)
			return
		}
	case r.Method == http.MethodDelete && action == "delete":
		if err := repository.Delete(r.Context(), name, false, false); err != nil {
			http.Error(w, "Could not delete account: "+err.Error(), http.StatusConflict)
			return
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"accounts": redactCodexManagerAccounts(repository)})
}

func setCodexManagerSessionMonitorDisabled(ctx context.Context, repository account.Repository, name string, disabled bool) error {
	path, err := repository.Paths.Status(name)
	if err != nil {
		return err
	}
	return storage.WithLock(ctx, repository.Paths, func() error {
		var status map[string]any
		if err := storage.ReadJSON(path, &status); err != nil && !os.IsNotExist(err) {
			return err
		}
		if status == nil {
			status = map[string]any{}
		}
		status["session_monitor_disabled"] = disabled
		return storage.WriteJSON(repository.Paths, path, status)
	})
}

// handleAPICodexManagerActivateBest turns the existing recommendation into an
// explicit action. The gateway still owns automatic request routing; this only
// changes the user's pinned/default account in the Manager store.
func handleAPICodexManagerActivateBest(w http.ResponseWriter, r *http.Request) {
	selection := codexManagerRecommendation()
	if selection.Best == nil || !selection.Best.Recommendable || strings.TrimSpace(selection.Best.Account) == "" {
		http.Error(w, "No account with available quota is ready; refresh limits or sign in again", http.StatusConflict)
		return
	}
	repository := account.Repository{Paths: codexManagerPaths()}
	if err := switchCodexManagerLiveAccount(r.Context(), repository, selection.Best.Account); err != nil {
		http.Error(w, "Could not activate recommended account: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"account": selection.Best.Account, "recommendation": selection.Best, "accounts": redactCodexManagerAccounts(repository)})
}

func handleAPICodexManagerHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("account"))
	if _, err := storage.SanitizeAccountName(name); err != nil {
		http.Error(w, "account is required", http.StatusBadRequest)
		return
	}
	limit := 250
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			http.Error(w, "invalid history limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	since, err := codexManagerHistorySince(r.URL.Query().Get("from"), r.URL.Query().Get("range"))
	if err != nil {
		http.Error(w, "invalid history range", http.StatusBadRequest)
		return
	}
	before, err := codexManagerHistoryTimestamp(r.URL.Query().Get("before"))
	if err != nil {
		http.Error(w, "invalid history cursor", http.StatusBadRequest)
		return
	}
	store := history.Store{Paths: codexManagerPaths()}
	if window := strings.TrimSpace(r.URL.Query().Get("window")); window != "" {
		if !before.IsZero() {
			http.Error(w, "history cursor is not supported for a chart series", http.StatusBadRequest)
			return
		}
		series, err := store.SeriesIn(name, window, since, limit, r.URL.Query().Get("timezone"))
		if err != nil {
			http.Error(w, "Could not load history series", http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"series": series})
		return
	}
	// Ask for one sentinel sample so callers can page backwards without ever
	// receiving an unbounded JSONL file.
	rows, err := store.ReadPage(name, since, before, limit+1)
	if err != nil {
		http.Error(w, "Could not load history", http.StatusInternalServerError)
		return
	}
	response := map[string]any{"items": rows}
	if len(rows) > limit {
		rows = rows[1:]
		response["items"] = rows
		response["nextBefore"] = rows[0].At.UTC().Format(time.RFC3339Nano)
	}
	writeJSON(w, response)
}

func codexManagerHistoryTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func codexManagerHistorySince(from, rangeName string) (time.Time, error) {
	if from = strings.TrimSpace(from); from != "" {
		return codexManagerHistoryTimestamp(from)
	}
	switch strings.TrimSpace(strings.ToLower(rangeName)) {
	case "", "all":
		return time.Time{}, nil
	case "7d":
		return time.Now().UTC().Add(-7 * 24 * time.Hour), nil
	case "30d":
		return time.Now().UTC().Add(-30 * 24 * time.Hour), nil
	case "90d":
		return time.Now().UTC().Add(-90 * 24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported range")
	}
}

func codexManagerRecommendation() recommendation.Selection {
	paths := codexManagerPaths()
	manager := codexmanager.New(paths)
	selection, err := manager.Recommendation(time.Now().UTC())
	if err != nil {
		return recommendation.Selection{}
	}
	return selection
}

func codexManagerSnapshot() map[string]any {
	settings, _ := state.LoadSettings()
	paths := codexManagerPaths()
	repository := account.Repository{Paths: paths}
	accounts := redactCodexManagerAccounts(repository)
	return map[string]any{
		"enabled":          settings.CodexBackend.Enabled,
		"mode":             settings.CodexBackend.Mode,
		"managerBaseUrl":   settings.CodexBackend.ManagerBaseURL,
		"autoSwitchPolicy": settings.CodexBackend.AutoSwitchPolicy,
		"gateway":          codexManagerStatus(),
		"gatewayKey":       map[string]bool{"configured": secretsConfigured(codexManagerGatewaySecretName)},
		"diagnostics":      codexManagerDiagnostics(),
		"accounts":         accounts,
	}
}

func redactCodexManagerAccounts(repository account.Repository) []account.Account {
	names, err := repository.List()
	if err != nil {
		return []account.Account{}
	}
	active, _ := repository.Active()
	result := make([]account.Account, 0, len(names))
	for _, name := range names {
		raw, err := repository.Read(name)
		if err != nil {
			continue
		}
		metadata := auth.Metadata(raw)
		plan := account.Plan(metadata["plan"])
		if plan != account.PlanFree && plan != account.PlanPlus {
			plan = account.PlanUnknown
		}
		state, checkedAt, refreshedAt, statusMessage, rateLimits, sessionMonitor, chromeProfile := codexManagerAccountStatus(repository.Paths, name)
		result = append(result, account.Account{
			Name:           name,
			Email:          metadata["email"],
			AccountID:      metadata["account_id"],
			Plan:           plan,
			State:          state,
			TokenExpiresAt: auth.AccessExpiry(raw),
			LastCheckedAt:  checkedAt,
			LastRefreshAt:  refreshedAt,
			StatusMessage:  statusMessage,
			RateLimits:     rateLimits,
			SessionMonitor: sessionMonitor,
			ChromeProfile:  chromeProfile,
			Active:         name == active,
		})
	}
	return result
}

// codexManagerAccountStatus reads only the safe local status projection. It
// contains no token material and lets the UI distinguish a ready account from
// one that needs a fresh device login after an explicit limit check.
func codexManagerAccountStatus(paths storage.Paths, name string) (account.State, *time.Time, *time.Time, string, *limits.Snapshot, *account.SessionMonitor, *account.ChromeProfile) {
	path, err := paths.Status(name)
	if err != nil {
		return account.StateReady, nil, nil, "", nil, nil, nil
	}
	var status struct {
		State                  string                  `json:"state"`
		Message                string                  `json:"message"`
		CheckedAt              time.Time               `json:"checked_at"`
		RateLimits             *limits.Snapshot        `json:"rate_limits"`
		SessionMonitor         *account.SessionMonitor `json:"session_monitor"`
		SessionMonitorDisabled bool                    `json:"session_monitor_disabled"`
		ChromeProfile          *account.ChromeProfile  `json:"chrome_profile"`
	}
	if err := storage.ReadJSON(path, &status); err != nil {
		return account.StateReady, nil, nil, "", nil, nil, nil
	}
	state := account.State(strings.TrimSpace(status.State))
	if state != account.StateReady && state != account.StateNeedsLogin && state != account.StateError && state != account.StateStale {
		state = account.StateReady
	}
	if status.SessionMonitor == nil && status.SessionMonitorDisabled {
		status.SessionMonitor = &account.SessionMonitor{RevocationDisabled: true}
	} else if status.SessionMonitor != nil && status.SessionMonitorDisabled {
		status.SessionMonitor.RevocationDisabled = true
	}
	if status.CheckedAt.IsZero() {
		return state, nil, nil, strings.TrimSpace(status.Message), status.RateLimits, status.SessionMonitor, status.ChromeProfile
	}
	checkedAt := status.CheckedAt.UTC()
	var refreshedAt *time.Time
	if strings.HasPrefix(strings.TrimSpace(status.Message), "refreshed") {
		refreshedAt = &checkedAt
	}
	return state, &checkedAt, refreshedAt, strings.TrimSpace(status.Message), status.RateLimits, status.SessionMonitor, status.ChromeProfile
}

func secretsConfigured(name string) bool {
	// Kept separate so the API surface only ever sees a boolean.
	return secrets.Configured(name)
}
