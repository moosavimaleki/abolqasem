package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	stateDir string
	mu       sync.Mutex
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("could not get home dir: %v", err))
	}
	stateDir = filepath.Join(home, ".cache", "ai-session-viewer")
	os.MkdirAll(stateDir, 0755)
}

func GetStateDir() string {
	return stateDir
}

func GetStateFilePath() string {
	return filepath.Join(stateDir, "state.json")
}

func LoadState() (*AppState, error) {
	mu.Lock()
	defer mu.Unlock()

	statePath := GetStateFilePath()
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return newAppState(), nil
		}
		return nil, err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		corruptPath := statePath + ".corrupt." + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(statePath, corruptPath); renameErr != nil {
			return nil, fmt.Errorf("invalid state json: %w (also failed to move corrupt file: %v)", err, renameErr)
		}
		return newAppState(), nil
	}

	return migrateState(&state), nil
}

func SaveState(state *AppState) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	statePath := GetStateFilePath()
	tempPath := statePath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, statePath)
}

func newAppState() *AppState {
	return &AppState{
		Sessions: make(map[string]SessionMeta),
	}
}

func migrateState(appState *AppState) *AppState {
	if appState == nil {
		return newAppState()
	}
	if appState.Sessions == nil {
		appState.Sessions = make(map[string]SessionMeta)
	}

	migrated := make(map[string]SessionMeta, len(appState.Sessions))
	for key, meta := range appState.Sessions {
		meta.Agent = stringsOr(meta.Agent, "unknown")
		meta.SessionID = stringsOr(meta.SessionID, key)
		if meta.Key == "" {
			meta.Key = key
		}
		newKey := meta.Key
		if !stringsContainsKey(newKey) {
			newKey = SessionKey(meta.Agent, meta.SessionID)
		}
		meta.Key = newKey
		if meta.ProjectName == "" {
			meta.ProjectName = deriveProjectName(meta.Cwd, meta.TranscriptPath)
		}
		if meta.TranscriptPath == "" {
			meta.MetadataOnly = true
			if meta.InvalidReason == "" {
				meta.InvalidReason = "transcript path is missing"
			}
		}
		migrated[newKey] = meta
	}
	appState.Sessions = migrated

	if appState.LatestSessionKey == "" && appState.LatestSessionID != "" {
		for key, meta := range appState.Sessions {
			if meta.SessionID == appState.LatestSessionID {
				appState.LatestSessionKey = key
				break
			}
		}
	}

	return appState
}

func stringsOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func stringsContainsKey(value string) bool {
	for _, ch := range value {
		if ch == ':' {
			return true
		}
	}
	return false
}
