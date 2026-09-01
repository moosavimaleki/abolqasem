package server

import (
	"abolqasem/internal/providers/opencode"
	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var discoveryMu sync.Mutex
var openCodeDiscoveryMu sync.Mutex
var openCodeLastDiscovery time.Time

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
		broadcastGlobalEvent(SSEEvent{
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
	report, discoveryErr := state.DiscoverSessions(appState)
	openCodeReport, openCodeErr := discoverOpenCodeSessions(appState)
	report.Found += openCodeReport.Found
	report.Added += openCodeReport.Added
	report.Updated += openCodeReport.Updated
	report.ChangedSessionKeys = append(report.ChangedSessionKeys, openCodeReport.ChangedSessionKeys...)
	if report.Added == 0 && report.Updated == 0 {
		return report, errors.Join(discoveryErr, openCodeErr)
	}
	if err := state.SaveState(appState); err != nil {
		return report, err
	}
	workspaceSyncDiscoveredLegacySessions(report.ChangedSessionKeys)
	return report, errors.Join(discoveryErr, openCodeErr)
}

// discoverOpenCodeSessions imports native OpenCode sessions from its local
// read-only database. Exports are cached locally so the workspace can render
// them without starting an OpenCode helper process.
func discoverOpenCodeSessions(appState *state.AppState) (state.DiscoveryReport, error) {
	openCodeDiscoveryMu.Lock()
	if !openCodeLastDiscovery.IsZero() && time.Since(openCodeLastDiscovery) < 5*time.Minute {
		openCodeDiscoveryMu.Unlock()
		return state.DiscoveryReport{}, nil
	}
	openCodeLastDiscovery = time.Now()
	openCodeDiscoveryMu.Unlock()
	if providerexec.Executable("opencode") == "" {
		return state.DiscoveryReport{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	sessions, listErr := opencode.ListSessions(ctx)
	if listErr != nil {
		return state.DiscoveryReport{}, listErr
	}
	if appState.Sessions == nil {
		appState.Sessions = map[string]state.SessionMeta{}
	}
	var report state.DiscoveryReport
	var exportErrors []error
	for _, session := range sessions {
		if session.ID == "" || session.Directory == "" {
			continue
		}
		report.Found++
		updatedAt := opencode.SessionUpdatedAt(session).UTC()
		key := state.SessionKey("opencode", session.ID)
		before, existed := appState.Sessions[key]
		transcriptPath := before.TranscriptPath
		if !existed || !before.UpdatedAt.Equal(updatedAt) || transcriptPath == "" {
			path, exportErr := opencode.CacheExport(ctx, session.ID)
			if exportErr != nil {
				exportErrors = append(exportErrors, fmt.Errorf("%s: %w", session.ID, exportErr))
				continue
			}
			transcriptPath = path
		}
		meta := state.UpsertSession(appState, state.HookEvent{
			Agent:          "opencode",
			SessionID:      session.ID,
			TranscriptPath: transcriptPath,
			Cwd:            session.Directory,
			ProjectName:    filepath.Base(session.Directory),
			UpdatedAt:      updatedAt.Format(time.RFC3339),
		})
		if session.Title != "" {
			meta.SessionName = session.Title
			appState.Sessions[meta.Key] = meta
		}
		if !existed {
			report.Added++
			report.ChangedSessionKeys = append(report.ChangedSessionKeys, meta.Key)
		} else if !before.UpdatedAt.Equal(meta.UpdatedAt) || before.TranscriptPath != meta.TranscriptPath || before.SessionName != meta.SessionName {
			report.Updated++
			report.ChangedSessionKeys = append(report.ChangedSessionKeys, meta.Key)
		}
	}
	return report, errors.Join(exportErrors...)
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
