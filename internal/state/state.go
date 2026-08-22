package state

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// HookEvent is the normalized payload received from a local hook.
type HookEvent struct {
	Agent          string `json:"agent"`
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name,omitempty"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	ProjectName    string `json:"project_name,omitempty"`
	PromptPreview  string `json:"prompt_preview,omitempty"`
	LastPreview    string `json:"last_preview,omitempty"`
	Model          string `json:"model,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	MetadataOnly   bool   `json:"metadata_only,omitempty"`
	InvalidReason  string `json:"invalid_reason,omitempty"`
}

// SessionMeta holds metadata for a single session.
type SessionMeta struct {
	Key                  string    `json:"key"`
	Agent                string    `json:"agent"`
	SessionID            string    `json:"session_id"`
	SessionName          string    `json:"session_name,omitempty"`
	TranscriptPath       string    `json:"transcript_path"`
	Cwd                  string    `json:"cwd"`
	ProjectName          string    `json:"project_name"`
	Model                string    `json:"model,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
	FirstPreview         string    `json:"first_preview,omitempty"`
	LastPreview          string    `json:"last_preview"`
	MessageCountEstimate int       `json:"message_count_estimate"`
	MetadataOnly         bool      `json:"metadata_only"`
	InvalidReason        string    `json:"invalid_reason,omitempty"`
}

// AppState holds the global state of the application.
type AppState struct {
	Sessions          map[string]SessionMeta `json:"sessions"`
	UnreadSessionKeys map[string]bool        `json:"unread_session_keys,omitempty"`
	LatestSessionKey  string                 `json:"latest_session_key"`
	LatestSessionID   string                 `json:"latest_session_id,omitempty"`
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
	event.HookEventName = strings.TrimSpace(event.HookEventName)
	event.TranscriptPath = normalizePath(event.TranscriptPath)
	event.ProjectName = strings.TrimSpace(event.ProjectName)
	event.PromptPreview = sanitizePreview(event.PromptPreview)
	event.LastPreview = sanitizePreview(event.LastPreview)
	event.Model = strings.TrimSpace(event.Model)
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
	if appState.Sessions == nil {
		appState.Sessions = map[string]SessionMeta{}
	}
	if appState.UnreadSessionKeys == nil {
		appState.UnreadSessionKeys = map[string]bool{}
	}
	event = NormalizeAndValidateEvent(event)
	event.SessionID = canonicalSessionIDForEvent(appState, event)
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
	if event.Model != "" && !staleEvent {
		meta.Model = event.Model
	}
	meta.UpdatedAt = updatedAt
	if !staleEvent || (meta.MetadataOnly && event.TranscriptPath != "" && !event.MetadataOnly) {
		meta.MetadataOnly = event.MetadataOnly
		meta.InvalidReason = event.InvalidReason
	}
	if event.TranscriptPath != "" && (!staleEvent || meta.TranscriptPath == "") {
		meta.TranscriptPath = event.TranscriptPath
	}
	if event.LastPreview != "" {
		meta.LastPreview = event.LastPreview
	}
	if event.PromptPreview != "" {
		if meta.FirstPreview == "" {
			meta.FirstPreview = event.PromptPreview
		}
		if meta.LastPreview == "" {
			meta.LastPreview = event.PromptPreview
		}
		if meta.MessageCountEstimate == 0 {
			meta.MessageCountEstimate = 1
		}
	}

	appState.Sessions[key] = meta
	if shouldMarkLatest(appState, meta) {
		appState.LatestSessionKey = key
		appState.LatestSessionID = event.SessionID
	}
	if shouldMarkSessionUnread(existing, ok, meta, staleEvent) {
		appState.UnreadSessionKeys[key] = true
	}
	return meta
}

func canonicalSessionIDForEvent(appState *AppState, event HookEvent) string {
	if appState == nil || appState.Sessions == nil || event.TranscriptPath == "" {
		return event.SessionID
	}

	pathKey := transcriptPathIndexKey(event.Agent, event.TranscriptPath)
	var matched []SessionMeta
	for _, meta := range appState.Sessions {
		if transcriptPathIndexKey(meta.Agent, meta.TranscriptPath) == pathKey {
			matched = append(matched, meta)
		}
	}
	if len(matched) == 0 {
		return event.SessionID
	}

	canonical := matched[0]
	for _, meta := range matched[1:] {
		canonical = mergeCanonicalSessionMeta(canonical, meta)
	}

	merged := mergeCanonicalSessionMeta(canonical, SessionMeta{
		Agent:          event.Agent,
		SessionID:      event.SessionID,
		TranscriptPath: event.TranscriptPath,
		Cwd:            event.Cwd,
		ProjectName:    event.ProjectName,
	})

	canonicalKey := SessionKey(event.Agent, merged.SessionID)
	merged.Key = canonicalKey
	unread := false
	for _, meta := range matched {
		if appState.UnreadSessionKeys != nil && appState.UnreadSessionKeys[meta.Key] {
			unread = true
			delete(appState.UnreadSessionKeys, meta.Key)
		}
		delete(appState.Sessions, meta.Key)
	}
	appState.Sessions[canonicalKey] = merged
	if unread {
		if appState.UnreadSessionKeys == nil {
			appState.UnreadSessionKeys = map[string]bool{}
		}
		appState.UnreadSessionKeys[canonicalKey] = true
	}
	if appState.LatestSessionKey != "" {
		for _, meta := range matched {
			if appState.LatestSessionKey == meta.Key {
				appState.LatestSessionKey = canonicalKey
				break
			}
		}
	}
	return merged.SessionID
}

func shouldMarkSessionUnread(existing SessionMeta, existed bool, next SessionMeta, staleEvent bool) bool {
	if staleEvent {
		return false
	}
	if !existed {
		return false
	}
	if next.UpdatedAt.After(existing.UpdatedAt) {
		return true
	}
	return next.LastPreview != existing.LastPreview
}

func MarkSessionRead(appState *AppState, sessionKey string) bool {
	if appState == nil || len(appState.UnreadSessionKeys) == 0 {
		return false
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || !appState.UnreadSessionKeys[sessionKey] {
		return false
	}
	delete(appState.UnreadSessionKeys, sessionKey)
	return true
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

func mergeCanonicalSessionMeta(current, candidate SessionMeta) SessionMeta {
	preferred, other := preferredCanonicalSession(current, candidate)
	merged := preferred
	merged.Key = SessionKey(merged.Agent, merged.SessionID)
	if other.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = other.UpdatedAt
	}
	if merged.SessionName == "" {
		merged.SessionName = other.SessionName
	}
	if merged.TranscriptPath == "" {
		merged.TranscriptPath = other.TranscriptPath
	}
	if merged.Cwd == "" {
		merged.Cwd = other.Cwd
	}
	if merged.ProjectName == "" || merged.ProjectName == "unknown" {
		merged.ProjectName = other.ProjectName
	}
	if merged.FirstPreview == "" {
		merged.FirstPreview = other.FirstPreview
	}
	if merged.LastPreview == "" {
		merged.LastPreview = other.LastPreview
	}
	if merged.MessageCountEstimate == 0 {
		merged.MessageCountEstimate = other.MessageCountEstimate
	}
	if merged.InvalidReason == "" {
		merged.InvalidReason = other.InvalidReason
	}
	if merged.MetadataOnly && !other.MetadataOnly {
		merged.MetadataOnly = false
	}
	return merged
}

func preferredCanonicalSession(a, b SessionMeta) (SessionMeta, SessionMeta) {
	aScore := sessionIDQuality(a.Agent, a.SessionID)
	bScore := sessionIDQuality(b.Agent, b.SessionID)
	if bScore > aScore {
		return b, a
	}
	if aScore > bScore {
		return a, b
	}
	if b.UpdatedAt.After(a.UpdatedAt) {
		return b, a
	}
	return a, b
}

func sessionIDQuality(agent, sessionID string) int {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return -1
	}
	if strings.EqualFold(strings.TrimSpace(agent), "codex") && strings.HasPrefix(strings.ToLower(sessionID), "rollout-") {
		return 0
	}
	return 1
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

func ResolveSessionName(meta SessionMeta) string {
	if strings.TrimSpace(meta.SessionName) != "" {
		return SanitizeSessionName(meta.SessionName)
	}
	return deriveSessionName(meta.FirstPreview, meta.LastPreview, meta.SessionID)
}

func SanitizeSessionName(value string) string {
	value = normalizeSessionTitle(value)
	runes := []rune(value)
	if len(runes) > 72 {
		value = strings.TrimSpace(string(runes[:72])) + "..."
	}
	if value == "" {
		return "نشست بدون نام"
	}
	return value
}

func deriveSessionName(firstPreview, lastPreview, sessionID string) string {
	for _, candidate := range []string{
		preferPersianTitle(firstPreview),
		normalizeSessionTitle(firstPreview),
		preferPersianTitle(lastPreview),
		normalizeSessionTitle(lastPreview),
		sessionID,
	} {
		if candidate != "" && !IsAgentBootstrapPrompt(candidate) {
			return SanitizeSessionName(candidate)
		}
	}
	return "نشست بدون نام"
}

func IsAgentBootstrapPrompt(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	normalized := strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(lower, "\r\n", "\n"), "\r", "\n")), " ")
	if strings.Contains(normalized, "agents.md instructions for ") {
		return true
	}
	if strings.Contains(normalized, "claude.md instructions for ") {
		return true
	}
	if strings.Contains(normalized, "<environment_context>") &&
		strings.Contains(normalized, "<cwd>") &&
		strings.Contains(normalized, "<shell>") {
		return true
	}
	if strings.Contains(normalized, "<instructions>") && strings.Contains(normalized, "</instructions>") {
		return true
	}
	return false
}

func preferPersianTitle(value string) string {
	normalized := normalizeSessionTitle(value)
	if normalized == "" {
		return ""
	}
	runes := []rune(normalized)
	start := -1
	for i, r := range runes {
		if unicode.In(r, unicode.Arabic) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	return normalizeSessionTitle(string(runes[start:]))
}

func normalizeSessionTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(strings.Split(value, "\n")[0])
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, " -–—_|:/\\,.!؟[](){}'\"`")
	return value
}
