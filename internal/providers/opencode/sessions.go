package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
)

type SessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
	Updated   int64  `json:"updated"`
	Created   int64  `json:"created"`
}

func ListSessions(ctx context.Context) ([]SessionSummary, error) {
	return listSessions(ctx, "")
}

// ListSessionsInDirectory asks OpenCode for sessions associated with one local
// project. The OpenCode CLI intentionally scopes `session list` by cwd.
func ListSessionsInDirectory(ctx context.Context, directory string) ([]SessionSummary, error) {
	return listSessions(ctx, strings.TrimSpace(directory))
}

func listSessions(ctx context.Context, directory string) ([]SessionSummary, error) {
	if sessions, err := listSessionsFromDatabase(); err == nil {
		if directory == "" {
			return sessions, nil
		}
		filtered := make([]SessionSummary, 0, len(sessions))
		for _, session := range sessions {
			if filepath.Clean(session.Directory) == filepath.Clean(directory) {
				filtered = append(filtered, session)
			}
		}
		return filtered, nil
	}
	var result []byte
	var err error
	if directory == "" {
		result, err = run(ctx, "session", "list", "--format", "json", "--max-count", "200")
	} else {
		result, err = runInDirectory(ctx, directory, "session", "list", "--format", "json", "--max-count", "200")
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(result))) == 0 {
		return []SessionSummary{}, nil
	}
	var sessions []SessionSummary
	if err := json.Unmarshal(jsonObjectBytes(result), &sessions); err != nil {
		return nil, fmt.Errorf("decode OpenCode session list: %w", err)
	}
	return sessions, nil
}

func ExportSession(ctx context.Context, sessionID string) ([]byte, error) {
	if data, err := exportSessionFromDatabase(sessionID); err == nil {
		return data, nil
	}
	result, err := run(ctx, "export", strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	data := jsonObjectBytes(result)
	if !json.Valid(data) {
		return nil, fmt.Errorf("OpenCode export for %s is not JSON", sessionID)
	}
	return data, nil
}

func CacheExport(ctx context.Context, sessionID string) (string, error) {
	data, err := ExportSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(state.GetStateDir(), "opencode", "sessions")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, strings.TrimSpace(sessionID)+".json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, path); err != nil {
		return "", err
	}
	return path, nil
}

func SessionUpdatedAt(session SessionSummary) time.Time {
	if session.Updated > 0 {
		return time.UnixMilli(session.Updated)
	}
	if session.Created > 0 {
		return time.UnixMilli(session.Created)
	}
	return time.Now()
}

func run(ctx context.Context, args ...string) ([]byte, error) {
	command := providerexec.ExecutableOrName("opencode")
	output, err := execCommandContext(ctx, command, args...)
	if err != nil {
		if json.Valid(jsonObjectBytes(output)) {
			return output, nil
		}
		return output, fmt.Errorf("opencode %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func runInDirectory(ctx context.Context, directory string, args ...string) ([]byte, error) {
	output, err := execCommandWithDirectory(ctx, directory, providerexec.ExecutableOrName("opencode"), args...)
	if err != nil {
		if json.Valid(jsonObjectBytes(output)) {
			return output, nil
		}
		return output, fmt.Errorf("opencode %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

var execCommandContext = func(ctx context.Context, command string, args ...string) ([]byte, error) {
	return execCommandContextDefault(ctx, command, args...)
}

func jsonObjectBytes(value []byte) []byte {
	trimmed := strings.TrimSpace(string(value))
	index := strings.IndexAny(trimmed, "[{")
	if index < 0 {
		return []byte(trimmed)
	}
	candidate := strings.TrimSpace(trimmed[index:])
	// The CLI may print a progress line after the JSON payload. Decode exactly
	// one top-level value so that harmless diagnostic suffixes do not make a
	// valid session list look corrupt.
	decoder := json.NewDecoder(strings.NewReader(candidate))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err == nil {
		return raw
	}
	return []byte(candidate)
}
