package server

import (
	"ai-agent-manager/internal/state"
	"log"
	"strings"
	"sync"
	"time"
)

var discoveryMu sync.Mutex

func DiscoverSessionsOnce() {
	settings, err := state.LoadSettings()
	if err == nil && !settings.FilesystemDiscovery {
		return
	}
	report, err := runDiscovery()
	if err != nil {
		log.Printf("Warning: session discovery failed: %v", err)
	}
	if report.Added > 0 || report.Updated > 0 {
		EventBroker.Broadcast(SSEEvent{
			Source:    "discovery",
			EventKey:  "discovery:" + time.Now().Format(time.RFC3339Nano),
			UpdatedAt: time.Now().Format(time.RFC3339),
		})
		workspaceConnections.broadcast("")
	}
}

func StartDiscoveryLoop(interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			DiscoverSessionsOnce()
		}
	}()
}

func runDiscovery() (state.DiscoveryReport, error) {
	discoveryMu.Lock()
	defer discoveryMu.Unlock()

	appState, err := state.LoadState()
	if err != nil {
		return state.DiscoveryReport{}, err
	}
	report, err := state.DiscoverSessions(appState)
	if err != nil {
		return report, err
	}
	if report.Added == 0 && report.Updated == 0 {
		return report, nil
	}
	if err := state.SaveState(appState); err != nil {
		return report, err
	}
	workspaceSyncDiscoveredLegacySessions(report.ChangedSessionKeys)
	return report, nil
}

func workspaceSyncDiscoveredLegacySessions(sessionKeys []string) {
	if len(sessionKeys) == 0 {
		return
	}
	keys := make(map[string]struct{}, len(sessionKeys))
	for _, key := range sessionKeys {
		if key = strings.TrimSpace(key); key != "" {
			keys[key] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return
	}
	for _, meta := range workspaceLegacySessionMetas() {
		if _, ok := keys[meta.Key]; !ok {
			continue
		}
		_ = workspaceSyncMaterializedLegacyChat(meta)
	}
}
