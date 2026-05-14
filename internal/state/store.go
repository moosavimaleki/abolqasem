package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
			return &AppState{
				Sessions: make(map[string]SessionMeta),
			}, nil
		}
		return nil, err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Sessions == nil {
		state.Sessions = make(map[string]SessionMeta)
	}

	return &state, nil
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
