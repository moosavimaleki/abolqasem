package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/codexmanager/sidecar"
	"abolqasem/internal/secrets"
	"abolqasem/internal/state"
)

var codexManagerRuntime = struct {
	sync.Mutex
	supervisor *sidecar.Supervisor
}{}

func activateCodexManager(ctx context.Context) (sidecar.Status, error) {
	settings, err := state.LoadSettings()
	if err != nil {
		return sidecar.Status{}, err
	}
	key, err := ensureCodexManagerGatewayKey()
	if err != nil {
		return sidecar.Status{}, err
	}
	config, err := codexManagerSidecarConfig(settings)
	if err != nil {
		return sidecar.Status{}, err
	}
	codexManagerRuntime.Lock()
	if codexManagerRuntime.supervisor == nil {
		codexManagerRuntime.supervisor = &sidecar.Supervisor{}
	}
	supervisor := codexManagerRuntime.supervisor
	codexManagerRuntime.Unlock()
	status, err := supervisor.Start(ctx, config, key)
	if err != nil {
		return status, err
	}
	settings.CodexBackend.Mode = state.CodexBackendManager
	settings.CodexBackend.Enabled = true
	if err := state.SaveSettings(settings); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = supervisor.Stop(stopCtx)
		return sidecar.Status{}, fmt.Errorf("could not save Codex Manager settings: %w", err)
	}
	return status, nil
}

func disableCodexManager() error {
	settings, err := state.LoadSettings()
	if err != nil {
		return err
	}
	// Do not stop the sidecar here. A manager-backed app-server already running
	// may still have an active turn; only future sessions use the native provider.
	settings.CodexBackend.Enabled = false
	settings.CodexBackend.Mode = state.CodexBackendNative
	return state.SaveSettings(settings)
}

func codexManagerStatus() sidecar.Status {
	codexManagerRuntime.Lock()
	supervisor := codexManagerRuntime.supervisor
	codexManagerRuntime.Unlock()
	if supervisor == nil {
		return sidecar.Status{State: sidecar.StateStopped}
	}
	return supervisor.Status()
}

func codexManagerSidecarConfig(settings state.AppSettings) (sidecar.Config, error) {
	endpoint, err := url.Parse(settings.CodexBackend.ManagerBaseURL)
	if err != nil || endpoint.Scheme != "http" || endpoint.Host == "" {
		return sidecar.Config{}, errors.New("Codex Manager base URL must be a local HTTP URL")
	}
	listen := endpoint.Host
	if !strings.Contains(listen, ":") {
		return sidecar.Config{}, errors.New("Codex Manager base URL must include a port")
	}
	return sidecar.Config{
		ListenAddress: listen,
		ManagerHome:   filepath.Join(state.GetStateDir(), "codex-manager"),
		ModelsCache:   filepath.Join(workspaceCodexRootDir(), "models_cache.json"),
		UpstreamBase:  "https://chatgpt.com/backend-api/codex",
		Proxy:         settings.ProviderProxy.HTTPProxy,
		APIKeyEnv:     "CODEX_MANAGER_GATEWAY_API_KEY",
	}, nil
}

func ensureCodexManagerGatewayKey() (string, error) {
	key, err := secrets.Get(codexManagerGatewaySecretName)
	if err == nil && key != "" {
		return key, nil
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate gateway key: %w", err)
	}
	key = base64.RawURLEncoding.EncodeToString(bytes)
	if err := secrets.Put(codexManagerGatewaySecretName, key); err != nil {
		return "", fmt.Errorf("store gateway key: %w", err)
	}
	return key, nil
}
