package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/browser"
	"abolqasem/internal/state"
)

// handleAPICodexManagerBrowserCleanup always previews first unless the caller
// explicitly sets dryRun=false. The browser domain still protects the current
// device, and the server resolves both the account and profile locally.
func handleAPICodexManagerBrowserCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	profile, ok := codexManagerBrowserProfile(strings.TrimSpace(r.URL.Query().Get("profileId")))
	if !ok {
		http.Error(w, "Chrome profile was not found", http.StatusNotFound)
		return
	}
	var payload struct {
		Account string `json:"account"`
		DryRun  bool   `json:"dryRun"`
		Max     int    `json:"maxCodexSessions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&payload); err != nil || strings.TrimSpace(payload.Account) == "" {
		http.Error(w, "A managed account is required", http.StatusBadRequest)
		return
	}
	managed, ok := codexManagerAccountByName(strings.TrimSpace(payload.Account))
	if !ok {
		http.Error(w, "Managed account was not found", http.StatusNotFound)
		return
	}
	if payload.Max < 1 || payload.Max > 20 {
		payload.Max = defaultCodexManagerSessionLimit(managed.Plan)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cookies, err := browser.LoadChatGPTCookies(ctx, profile, nil)
	if err != nil {
		http.Error(w, "Could not read ChatGPT cookies; unlock the selected Chrome profile and retry", http.StatusBadGateway)
		return
	}
	client := codexManagerBrowserSessionClient(cookies)
	result := browser.Cleanup(ctx, managed, client, browser.CleanupPolicy{
		Enabled:        true,
		DryRun:         payload.DryRun,
		MaxCodexByPlan: map[account.Plan]int{managed.Plan: payload.Max},
	})
	if result.Error != "" {
		http.Error(w, "Could not inspect browser sessions: "+result.Error, http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"cleanup": result})
}

func codexManagerAccountByName(name string) (account.Account, bool) {
	for _, item := range redactCodexManagerAccounts(account.Repository{Paths: codexManagerPaths()}) {
		if item.Name == name {
			return item, true
		}
	}
	return account.Account{}, false
}

func defaultCodexManagerSessionLimit(plan account.Plan) int {
	// A managed Chrome profile should retain only its current Codex session.
	// Plan type does not increase the safe number of concurrent Codex logins.
	return 1
}

func handleAPICodexManagerBrowserDevices(w http.ResponseWriter, r *http.Request, revoke bool) {
	if (!revoke && r.Method != http.MethodGet) || (revoke && r.Method != http.MethodPost) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	profileID := strings.TrimSpace(r.URL.Query().Get("profileId"))
	profile, ok := codexManagerBrowserProfile(profileID)
	if !ok {
		http.Error(w, "Chrome profile was not found", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cookies, err := browser.LoadChatGPTCookies(ctx, profile, nil)
	if err != nil {
		http.Error(w, "Could not read ChatGPT cookies; unlock the selected Chrome profile and retry", http.StatusBadGateway)
		return
	}
	client := codexManagerBrowserSessionClient(cookies)
	if !revoke {
		devices, err := client.Devices(ctx)
		if err != nil {
			http.Error(w, "Could not load browser sessions: "+err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"devices": devices})
		return
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&payload); err != nil || strings.TrimSpace(payload.ID) == "" {
		http.Error(w, "A device ID is required", http.StatusBadRequest)
		return
	}
	devices, err := client.Devices(ctx)
	if err != nil {
		http.Error(w, "Could not verify browser session: "+err.Error(), http.StatusBadGateway)
		return
	}
	var target browser.Device
	for _, device := range devices {
		if device.ID == strings.TrimSpace(payload.ID) {
			target = device
			break
		}
	}
	if target.ID == "" {
		http.Error(w, "Browser session was not found", http.StatusNotFound)
		return
	}
	if err := client.Revoke(ctx, target); err != nil {
		http.Error(w, "Could not revoke browser session: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{"revoked": true, "id": strings.TrimSpace(payload.ID)})
}

func codexManagerBrowserProfile(id string) (browser.Profile, bool) {
	profiles, err := codexManagerBrowserProfiles()
	if err != nil {
		return browser.Profile{}, false
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return browser.Profile{}, false
}

func codexManagerBrowserSessionClient(cookies []browser.Cookie) browser.SessionClient {
	settings, err := state.LoadSettings()
	proxyURL := ""
	if err == nil {
		proxyURL = settings.CodexBackend.Maintenance.ProxyURL
	}
	return browser.SessionClient{Cookies: cookies, ProxyURL: proxyURL}
}
