package state

import "time"

// HookEvent is the payload received from Codex Hook
type HookEvent struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

// SessionMeta holds metadata for a single session
type SessionMeta struct {
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path"`
	Cwd            string    `json:"cwd"`
	ProjectName    string    `json:"project_name"`
	UpdatedAt      time.Time `json:"updated_at"`
	LastPreview    string    `json:"last_preview"`
}

// AppState holds the global state of the application
type AppState struct {
	Sessions        map[string]SessionMeta `json:"sessions"`
	LatestSessionID string                 `json:"latest_session_id"`
}
