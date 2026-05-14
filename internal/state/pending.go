package state

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

func GetPendingEventsFilePath() string {
	return filepath.Join(stateDir, "pending-events.jsonl")
}

func SavePendingEvent(event HookEvent) error {
	mu.Lock()
	defer mu.Unlock()

	event = NormalizeAndValidateEvent(event)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(GetPendingEventsFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	return nil
}

func ProcessPendingEvents(appState *AppState) error {
	path := GetPendingEventsFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // No pending events
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	modified := false

	for scanner.Scan() {
		var event HookEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue // skip invalid lines
		}
		UpsertSession(appState, event)
		modified = true
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if modified {
		if err := SaveState(appState); err != nil {
			return err
		}
	}

	// Clear the pending events file
	return os.Remove(path)
}
