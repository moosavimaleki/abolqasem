package state

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HookEvent is the normalized payload received from a local hook.
type HookEvent struct {
	Agent          string `json:"agent"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	ProjectName    string `json:"project_name,omitempty"`
	LastPreview    string `json:"last_preview,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	MetadataOnly   bool   `json:"metadata_only,omitempty"`
	InvalidReason  string `json:"invalid_reason,omitempty"`
}

// SessionMeta holds metadata for a single session.
type SessionMeta struct {
	Key                  string    `json:"key"`
	Agent                string    `json:"agent"`
	SessionID            string    `json:"session_id"`
	TranscriptPath       string    `json:"transcript_path"`
	Cwd                  string    `json:"cwd"`
	ProjectName          string    `json:"project_name"`
	UpdatedAt            time.Time `json:"updated_at"`
	LastPreview          string    `json:"last_preview"`
	MessageCountEstimate int       `json:"message_count_estimate"`
	MetadataOnly         bool      `json:"metadata_only"`
	InvalidReason        string    `json:"invalid_reason,omitempty"`
}

// AppState holds the global state of the application.
type AppState struct {
	Sessions         map[string]SessionMeta `json:"sessions"`
	LatestSessionKey string                 `json:"latest_session_key"`
	LatestSessionID  string                 `json:"latest_session_id,omitempty"`
}

func SessionKey(agent, sessionID string) string {
	agent = strings.TrimSpace(agent)
	sessionID = strings.TrimSpace(sessionID)
	if agent == "" {
		agent = "unknown"
	}
	return agent + ":" + sessionID
}

func NormalizeAndValidateEvent(event HookEvent) HookEvent {
	event.Agent = strings.TrimSpace(strings.ToLower(event.Agent))
	if event.Agent == "" {
		event.Agent = "unknown"
	}

	event.Cwd = strings.TrimSpace(event.Cwd)
	event.TranscriptPath = normalizePath(event.TranscriptPath)
	event.ProjectName = strings.TrimSpace(event.ProjectName)
	event.LastPreview = sanitizePreview(event.LastPreview)
	event.InvalidReason = strings.TrimSpace(event.InvalidReason)

	if event.ProjectName == "" {
		event.ProjectName = deriveProjectName(event.Cwd, event.TranscriptPath)
	}

	if event.SessionID == "" {
		event.SessionID = fallbackSessionID(event.Agent, event.TranscriptPath, event.Cwd)
	}

	if event.TranscriptPath == "" {
		event.MetadataOnly = true
		if event.InvalidReason == "" {
			event.InvalidReason = "transcript path is missing"
		}
	} else if _, err := os.Stat(event.TranscriptPath); err != nil {
		event.MetadataOnly = true
		if event.InvalidReason == "" {
			event.InvalidReason = "transcript is not readable"
		}
	}

	return event
}

func UpsertSession(appState *AppState, event HookEvent) SessionMeta {
	event = NormalizeAndValidateEvent(event)
	key := SessionKey(event.Agent, event.SessionID)

	existing, ok := appState.Sessions[key]
	if !ok {
		existing = SessionMeta{
			Key:       key,
			Agent:     event.Agent,
			SessionID: event.SessionID,
		}
	}

	updatedAt := time.Now()
	if event.UpdatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, event.UpdatedAt); err == nil {
			updatedAt = parsed
		}
	}
	staleEvent := ok && !existing.UpdatedAt.IsZero() && updatedAt.Before(existing.UpdatedAt)
	if staleEvent {
		updatedAt = existing.UpdatedAt
	}

	meta := existing
	meta.Key = key
	meta.Agent = event.Agent
	meta.SessionID = event.SessionID
	if event.Cwd != "" && !staleEvent {
		meta.Cwd = event.Cwd
	}
	if event.ProjectName != "" && event.ProjectName != "unknown" && !staleEvent {
		meta.ProjectName = event.ProjectName
	} else if meta.ProjectName == "" {
		meta.ProjectName = deriveProjectName(meta.Cwd, firstNonEmptyString(event.TranscriptPath, meta.TranscriptPath))
	}
	meta.UpdatedAt = updatedAt
	if !staleEvent || (meta.MetadataOnly && event.TranscriptPath != "" && !event.MetadataOnly) {
		meta.MetadataOnly = event.MetadataOnly
		meta.InvalidReason = event.InvalidReason
	}
	if event.TranscriptPath != "" {
		meta.TranscriptPath = event.TranscriptPath
	}
	if event.LastPreview != "" {
		meta.LastPreview = event.LastPreview
	}

	appState.Sessions[key] = meta
	if shouldMarkLatest(appState, meta) {
		appState.LatestSessionKey = key
		appState.LatestSessionID = event.SessionID
	}
	return meta
}

func shouldMarkLatest(appState *AppState, meta SessionMeta) bool {
	if appState.LatestSessionKey == "" {
		return true
	}
	latest, ok := appState.Sessions[appState.LatestSessionKey]
	if !ok || latest.UpdatedAt.IsZero() {
		return true
	}
	return !meta.UpdatedAt.Before(latest.UpdatedAt)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home
		}
	} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func deriveProjectName(cwd, transcriptPath string) string {
	if cwd != "" {
		name := filepath.Base(cwd)
		if name != "" && name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	if transcriptPath != "" {
		name := filepath.Base(filepath.Dir(transcriptPath))
		if name != "" && name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	return "unknown"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sanitizePreview(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if len(value) > 240 {
		return strings.TrimSpace(value[:240]) + "..."
	}
	return value
}

func fallbackSessionID(agent, transcriptPath, cwd string) string {
	if transcriptPath != "" {
		return filepath.Base(filepath.Dir(transcriptPath))
	}
	h := sha1.Sum([]byte(agent + "\n" + transcriptPath + "\n" + cwd))
	return "fallback-" + hex.EncodeToString(h[:6])
}
