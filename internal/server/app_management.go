package server

import (
	"ai-agent-manager/internal/adapters"
	"ai-agent-manager/internal/adapters/claude"
	"ai-agent-manager/internal/adapters/codex"
	"ai-agent-manager/internal/adapters/gemini"
	"ai-agent-manager/internal/appinfo"
	"ai-agent-manager/internal/buildinfo"
	"ai-agent-manager/internal/netproxy"
	"ai-agent-manager/internal/state"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	appReleaseAPIURL = "https://api.github.com/repos/" + appinfo.GitHubRepo + "/releases/latest"
)

var (
	appUpdateHTTPClient *http.Client
	executablePath      = os.Executable
	startDetached       = func(exe string, args ...string) error {
		return exec.Command(exe, args...).Start()
	}
	workspaceUpdateMu    sync.Mutex
	workspaceUpdateState map[string]any
)

type hookStatus struct {
	Agent            string `json:"agent"`
	UserInstalled    bool   `json:"user_installed"`
	ProjectInstalled bool   `json:"project_installed"`
	Error            string `json:"error,omitempty"`
}

type appManagementPatch struct {
	HookUpdates                     *bool   `json:"hookUpdates"`
	HookFollowMode                  *string `json:"hookFollowMode"`
	IgnoreHookNavigationWhileTyping *bool   `json:"ignoreHookNavigationWhileTyping"`
	FilesystemDiscovery             *bool   `json:"filesystemDiscovery"`
}

func workspaceManagementSnapshot() map[string]any {
	settings, _ := state.LoadSettings()
	settings = state.NormalizeSettings(settings)
	hooks := workspaceHookStatuses()
	return map[string]any{
		"hookNotifications": map[string]any{
			"enabled":                        settings.HookUpdates,
			"followMode":                     settings.HookFollowMode,
			"ignoreNavigationWhileTyping":    settings.IgnoreHookNavigationWhileTyping,
			"filesystemDiscovery":            settings.FilesystemDiscovery,
			"supportedModes":                 []string{state.HookFollowAuto, state.HookFollowNotice, state.HookFollowOff},
			"dangerousOperationsNeedConfirm": true,
		},
		"startup": map[string]any{
			"mode":             workspaceStartupMode(hooks),
			"serviceInstalled": workspaceServiceInstalled(),
			"platform":         runtime.GOOS,
		},
		"hooks":  hooks,
		"update": workspaceUpdateSnapshot(),
		"actions": map[string]any{
			"reloadSessions": map[string]any{"available": true, "requiresConfirmation": false},
			"restartServer":  map[string]any{"available": true, "requiresConfirmation": true},
			"installUpdate":  map[string]any{"available": true, "requiresConfirmation": true},
		},
	}
}

func applyWorkspaceManagementPatch(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		Patch appManagementPatch `json:"patch"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	settings, err := state.LoadSettings()
	if err != nil {
		return nil, err
	}
	if payload.Patch.HookUpdates != nil {
		settings.HookUpdates = *payload.Patch.HookUpdates
	}
	if payload.Patch.HookFollowMode != nil {
		settings.HookFollowMode = *payload.Patch.HookFollowMode
	}
	if payload.Patch.IgnoreHookNavigationWhileTyping != nil {
		settings.IgnoreHookNavigationWhileTyping = *payload.Patch.IgnoreHookNavigationWhileTyping
	}
	if payload.Patch.FilesystemDiscovery != nil {
		settings.FilesystemDiscovery = *payload.Patch.FilesystemDiscovery
	}
	if err := state.SaveSettings(settings); err != nil {
		return nil, err
	}
	return workspaceManagementSnapshot(), nil
}

func workspaceHookStatuses() []hookStatus {
	statuses := make([]hookStatus, 0, 3)
	for _, adapter := range []adapters.AgentAdapter{codex.New(), claude.New(), gemini.New()} {
		status := hookStatus{Agent: adapter.Name()}
		userInstalled, userErr := adapter.IsHookInstalled(adapters.ScopeUser)
		projectInstalled, projectErr := adapter.IsHookInstalled(adapters.ScopeProject)
		status.UserInstalled = userInstalled
		status.ProjectInstalled = projectInstalled
		if userErr != nil {
			status.Error = userErr.Error()
		} else if projectErr != nil {
			status.Error = projectErr.Error()
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func workspaceStartupMode(hooks []hookStatus) string {
	if workspaceServiceInstalled() {
		return "service"
	}
	for _, hook := range hooks {
		if hook.UserInstalled || hook.ProjectInstalled {
			return "hook"
		}
	}
	return "manual"
}

func workspaceServiceInstalled() bool {
	switch runtime.GOOS {
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", appinfo.Name+".service"))
		if err == nil {
			return true
		}
		_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", appinfo.LegacyName+".service"))
		return err == nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents", appinfo.LaunchAgentLabel+".plist"))
		if err == nil {
			return true
		}
		_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents", appinfo.LegacyLaunchAgentLabel+".plist"))
		return err == nil
	case "windows":
		return exec.Command("schtasks", "/Query", "/TN", appinfo.WindowsTaskName).Run() == nil ||
			exec.Command("schtasks", "/Query", "/TN", appinfo.LegacyWindowsTaskName).Run() == nil
	default:
		return false
	}
}

func workspaceUpdateSnapshot() map[string]any {
	workspaceUpdateMu.Lock()
	defer workspaceUpdateMu.Unlock()
	if workspaceUpdateState != nil {
		return cloneMap(workspaceUpdateState)
	}
	workspaceUpdateState = defaultWorkspaceUpdateSnapshot()
	return cloneMap(workspaceUpdateState)
}

func defaultWorkspaceUpdateSnapshot() map[string]any {
	return map[string]any{
		"currentVersion":    normalizedAppVersion(),
		"latestVersion":     nil,
		"status":            "idle",
		"updateAvailable":   false,
		"lastCheckedAt":     nil,
		"error":             nil,
		"installAction":     "restart",
		"reloadRequestedAt": nil,
	}
}

func workspaceCheckUpdate() map[string]any {
	snapshot := workspaceUpdateSnapshot()
	snapshot["status"] = "checking"
	snapshot["lastCheckedAt"] = time.Now().UnixMilli()

	req, err := http.NewRequest(http.MethodGet, appReleaseAPIURL, nil)
	if err != nil {
		return workspaceUpdateError(snapshot, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", appinfo.Name+"/"+normalizedAppVersion())

	resp, err := workspaceUpdateHTTPClient().Do(req)
	if err != nil {
		return workspaceUpdateError(snapshot, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return workspaceUpdateError(snapshot, fmt.Errorf("GitHub release check failed: %s", resp.Status))
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return workspaceUpdateError(snapshot, err)
	}
	latest := strings.TrimSpace(release.TagName)
	snapshot["latestVersion"] = latest
	available := updateVersionNewer(latest, normalizedAppVersion())
	snapshot["updateAvailable"] = available
	if available {
		snapshot["status"] = "available"
	} else {
		snapshot["status"] = "up_to_date"
	}
	return setWorkspaceUpdateSnapshot(snapshot)
}

func workspaceInstallUpdate() map[string]any {
	if err := scheduleServerCommand("update"); err != nil {
		snapshot := workspaceUpdateSnapshot()
		snapshot["status"] = "error"
		snapshot["error"] = err.Error()
		_ = setWorkspaceUpdateSnapshot(snapshot)
		return map[string]any{
			"ok":          false,
			"action":      "restart",
			"errorCode":   "install_failed",
			"userTitle":   "Update failed",
			"userMessage": err.Error(),
		}
	}
	snapshot := workspaceUpdateSnapshot()
	snapshot["status"] = "restart_pending"
	snapshot["updateAvailable"] = false
	snapshot["error"] = nil
	snapshot["reloadRequestedAt"] = time.Now().UnixMilli()
	_ = setWorkspaceUpdateSnapshot(snapshot)
	return map[string]any{
		"ok":          true,
		"action":      "restart",
		"errorCode":   nil,
		"userTitle":   nil,
		"userMessage": nil,
	}
}

func workspaceUpdateHTTPClient() *http.Client {
	if appUpdateHTTPClient != nil {
		return appUpdateHTTPClient
	}
	return netproxy.HTTPClient(20 * time.Second)
}

func scheduleServerRestart() error {
	return scheduleServerCommand("restart")
}

func scheduleServerCommand(arg string) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	go func() {
		time.Sleep(250 * time.Millisecond)
		_ = startDetached(exe, arg)
	}()
	return nil
}

func workspaceUpdateError(snapshot map[string]any, err error) map[string]any {
	snapshot["status"] = "error"
	snapshot["error"] = err.Error()
	snapshot["updateAvailable"] = false
	return setWorkspaceUpdateSnapshot(snapshot)
}

func setWorkspaceUpdateSnapshot(snapshot map[string]any) map[string]any {
	workspaceUpdateMu.Lock()
	defer workspaceUpdateMu.Unlock()
	workspaceUpdateState = cloneMap(snapshot)
	return cloneMap(workspaceUpdateState)
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func normalizedAppVersion() string {
	version := strings.TrimSpace(buildinfo.Version)
	if version == "" {
		return "dev"
	}
	return strings.TrimPrefix(version, "v")
}

func updateVersionNewer(latest string, current string) bool {
	latest = strings.TrimPrefix(strings.TrimSpace(latest), "v")
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	if latest == "" || current == "" || latest == current {
		return false
	}
	if isDevelopmentVersion(current) {
		return true
	}
	latestBase, latestPrerelease := splitVersionPrerelease(latest)
	currentBase, currentPrerelease := splitVersionPrerelease(current)
	switch compareDottedVersion(latestBase, currentBase) {
	case 1:
		return true
	case -1:
		return false
	default:
		return currentPrerelease != "" && latestPrerelease == ""
	}
}

func isDevelopmentVersion(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	return version == "dev" || strings.HasPrefix(version, "dev-")
}

func splitVersionPrerelease(version string) (base string, prerelease string) {
	base, prerelease, _ = strings.Cut(strings.TrimSpace(version), "-")
	return strings.TrimSpace(base), strings.TrimSpace(prerelease)
}

func compareDottedVersion(left string, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}
	for i := 0; i < maxParts; i++ {
		leftValue := versionPart(leftParts, i)
		rightValue := versionPart(rightParts, i)
		if leftValue > rightValue {
			return 1
		}
		if leftValue < rightValue {
			return -1
		}
	}
	return 0
}

func versionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(parts[index]))
	if err != nil {
		return 0
	}
	return value
}
