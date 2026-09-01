package server

import (
	"os"
	"path/filepath"
	"strings"

	"abolqasem/internal/codexmanager/storage"
	"abolqasem/internal/secrets"
)

// codexManagerDiagnostics is metadata-only. It makes local failures
// actionable without ever serializing tokens, cookies, paths, or secrets.
func codexManagerDiagnostics() map[string]any {
	storeReady, storeMessage := codexManagerDiagnosticPath(codexManagerPaths().Home, true)
	liveReady, liveMessage := codexManagerDiagnosticPath(codexManagerLiveAuthPath(), false)
	return map[string]any{
		"gateway":              codexManagerStatus(),
		"worker":               codexManagerMaintenanceWorkerStatus(),
		"sessionMonitor":       codexManagerSessionMonitorStatus(),
		"store":                map[string]any{"ready": storeReady, "message": storeMessage},
		"liveAuth":             map[string]any{"present": liveReady, "message": liveMessage},
		"gatewayKeyConfigured": secrets.Configured(codexManagerGatewaySecretName),
	}
}

func codexManagerDiagnosticPath(path string, directory bool) (bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if directory {
				if err := storage.EnsureDirs(codexManagerPaths()); err != nil {
					return false, "cannot create the local manager store"
				}
				return true, "local store is ready"
			}
			return false, "no live Codex login was found"
		}
		return false, "cannot inspect local manager files"
	}
	if directory && !info.IsDir() {
		return false, "manager store path is not a directory"
	}
	if !directory && info.IsDir() {
		return false, "live Codex credential path is not a file"
	}
	if info.Mode().Perm()&0o077 != 0 {
		return false, "local credential permissions are too open"
	}
	if strings.TrimSpace(filepath.Base(path)) == "" {
		return false, "invalid local path"
	}
	return true, "ready"
}
