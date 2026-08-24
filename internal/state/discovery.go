package state

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxDiscoveryFiles      = 10000
	discoveryProbeMaxBytes = 256 * 1024
	discoveryProbeMaxLines = 80
)

type DiscoveryRoot struct {
	Agent string
	Path  string
}

type DiscoveryReport struct {
	Found              int
	Added              int
	Updated            int
	ChangedSessionKeys []string
}

type transcriptProbe struct {
	SessionID   string
	Cwd         string
	ProjectName string
}

func DiscoverSessions(appState *AppState) (DiscoveryReport, error) {
	return DiscoverSessionsInRoots(appState, defaultDiscoveryRoots())
}

func DiscoverSessionsInRoots(appState *AppState, roots []DiscoveryRoot) (DiscoveryReport, error) {
	if appState.Sessions == nil {
		appState.Sessions = make(map[string]SessionMeta)
	}

	pathIndex := transcriptPathIndex(appState)
	var report DiscoveryReport
	var walkErrors []error
	visitedRoots := map[string]bool{}

	for _, root := range roots {
		root.Agent = strings.TrimSpace(strings.ToLower(root.Agent))
		root.Path = normalizePath(root.Path)
		if root.Agent == "" || root.Path == "" || visitedRoots[root.Agent+"\x00"+root.Path] {
			continue
		}
		visitedRoots[root.Agent+"\x00"+root.Path] = true

		info, err := os.Stat(root.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				walkErrors = append(walkErrors, err)
			}
			continue
		}
		if !info.IsDir() {
			continue
		}

		filesSeen := 0
		err = filepath.WalkDir(root.Path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if shouldSkipDiscoveryDir(entry.Name()) && filepath.Clean(path) != filepath.Clean(root.Path) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isDiscoveryCandidate(root, path) {
				return nil
			}

			filesSeen++
			if filesSeen > maxDiscoveryFiles {
				return filepath.SkipAll
			}

			fileInfo, err := entry.Info()
			if err != nil || !fileInfo.Mode().IsRegular() {
				return nil
			}

			report.Found++
			before, existed, meta := upsertDiscoveredSession(appState, pathIndex, root, path, fileInfo.ModTime())
			if !existed {
				report.Added++
				report.ChangedSessionKeys = append(report.ChangedSessionKeys, meta.Key)
			} else if !sameSessionMeta(before, meta) {
				report.Updated++
				report.ChangedSessionKeys = append(report.ChangedSessionKeys, meta.Key)
			}
			pathIndex[transcriptPathIndexKey(meta.Agent, meta.TranscriptPath)] = meta.SessionID
			return nil
		})
		if err != nil && !errors.Is(err, filepath.SkipAll) {
			walkErrors = append(walkErrors, err)
		}
	}

	return report, errors.Join(walkErrors...)
}

func defaultDiscoveryRoots() []DiscoveryRoot {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	codexHome := firstNonEmptyString(os.Getenv("CODEX_HOME"), filepath.Join(home, ".codex"))
	claudeHome := firstNonEmptyString(os.Getenv("CLAUDE_CONFIG_DIR"), os.Getenv("CLAUDE_HOME"), filepath.Join(home, ".claude"))
	return []DiscoveryRoot{
		{Agent: "codex", Path: filepath.Join(codexHome, "sessions")},
		{Agent: "claude", Path: filepath.Join(claudeHome, "projects")},
	}
}

func upsertDiscoveredSession(appState *AppState, pathIndex map[string]string, root DiscoveryRoot, transcriptPath string, updatedAt time.Time) (SessionMeta, bool, SessionMeta) {
	if existing, ok := unchangedDiscoveredSession(appState, pathIndex, root, transcriptPath, updatedAt); ok {
		return existing, true, existing
	}
	probe := probeTranscript(transcriptPath)
	sessionID := firstNonEmptyString(
		pathIndex[transcriptPathIndexKey(root.Agent, transcriptPath)],
		probe.SessionID,
		discoveredCodexSessionID(root, transcriptPath),
		discoveredSessionID(root, transcriptPath),
	)
	event := HookEvent{
		Agent:          root.Agent,
		SessionID:      sessionID,
		TranscriptPath: transcriptPath,
		Cwd:            probe.Cwd,
		ProjectName:    probe.ProjectName,
		UpdatedAt:      updatedAt.Format(time.RFC3339),
	}
	key := SessionKey(root.Agent, sessionID)
	before, existed := appState.Sessions[key]
	meta := UpsertSession(appState, event)
	return before, existed, meta
}

func unchangedDiscoveredSession(appState *AppState, pathIndex map[string]string, root DiscoveryRoot, transcriptPath string, updatedAt time.Time) (SessionMeta, bool) {
	sessionID := pathIndex[transcriptPathIndexKey(root.Agent, transcriptPath)]
	if strings.TrimSpace(sessionID) == "" {
		return SessionMeta{}, false
	}
	meta, ok := appState.Sessions[SessionKey(root.Agent, sessionID)]
	if !ok {
		return SessionMeta{}, false
	}
	if strings.TrimSpace(meta.TranscriptPath) == "" || filepath.Clean(meta.TranscriptPath) != filepath.Clean(transcriptPath) {
		return SessionMeta{}, false
	}
	if !meta.UpdatedAt.Equal(updatedAt) {
		return SessionMeta{}, false
	}
	return meta, true
}

func discoveredCodexSessionID(root DiscoveryRoot, transcriptPath string) string {
	if root.Agent != "codex" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(transcriptPath), filepath.Ext(transcriptPath))
	parts := strings.Split(base, "-")
	if len(parts) < 6 || strings.ToLower(parts[0]) != "rollout" {
		return ""
	}
	return strings.Join(parts[len(parts)-5:], "-")
}

func transcriptPathIndex(appState *AppState) map[string]string {
	index := map[string]string{}
	for _, meta := range appState.Sessions {
		if meta.TranscriptPath == "" || meta.SessionID == "" {
			continue
		}
		index[transcriptPathIndexKey(meta.Agent, meta.TranscriptPath)] = meta.SessionID
	}
	return index
}

func transcriptPathIndexKey(agent, transcriptPath string) string {
	return strings.ToLower(strings.TrimSpace(agent)) + "\x00" + filepath.Clean(transcriptPath)
}

func isDiscoveryCandidate(root DiscoveryRoot, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	switch root.Agent {
	case "codex":
		return ext == ".jsonl" && strings.HasPrefix(base, "rollout-")
	case "claude":
		return ext == ".jsonl"
	default:
		return ext == ".jsonl"
	}
}

func shouldSkipDiscoveryDir(name string) bool {
	switch strings.ToLower(name) {
	case "node_modules", ".git", "cache", "caches", "generated_images":
		return true
	default:
		return false
	}
}

func discoveredSessionID(root DiscoveryRoot, transcriptPath string) string {
	base := strings.TrimSuffix(filepath.Base(transcriptPath), filepath.Ext(transcriptPath))
	base = strings.TrimSpace(base)
	if base == "" {
		base = "session"
	}
	if base != "session" && base != "transcript" && base != "chat" {
		return base
	}

	rel, err := filepath.Rel(root.Path, transcriptPath)
	if err != nil {
		rel = transcriptPath
	}
	sum := sha1.Sum([]byte(root.Agent + "\n" + filepath.ToSlash(rel)))
	return base + "-" + hex.EncodeToString(sum[:4])
}

func probeTranscript(path string) transcriptProbe {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		if probe, ok := probeStructuredJSON(path); ok {
			return probe
		}
	}
	return probeJSONLines(path)
}

func probeStructuredJSON(path string) (transcriptProbe, bool) {
	file, err := os.Open(path)
	if err != nil {
		return transcriptProbe{}, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, discoveryProbeMaxBytes))
	if err != nil || len(data) == 0 {
		return transcriptProbe{}, false
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return transcriptProbe{}, false
	}
	probe := transcriptProbe{
		SessionID:   findStringByKey(raw, sessionIDKeys, 0),
		Cwd:         findStringByKey(raw, cwdKeys, 0),
		ProjectName: findStringByKey(raw, projectNameKeys, 0),
	}
	return probe, probe.SessionID != "" || probe.Cwd != "" || probe.ProjectName != ""
}

func probeJSONLines(path string) transcriptProbe {
	file, err := os.Open(path)
	if err != nil {
		return transcriptProbe{}
	}
	defer file.Close()

	scanner := bufio.NewScanner(io.LimitReader(file, discoveryProbeMaxBytes))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	probe := transcriptProbe{}
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		if lineCount > discoveryProbeMaxLines {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		mergeProbe(&probe, raw)
		if probe.SessionID != "" && probe.Cwd != "" && probe.ProjectName != "" {
			break
		}
	}
	return probe
}

func mergeProbe(probe *transcriptProbe, raw any) {
	if probe.SessionID == "" {
		probe.SessionID = findStringByKey(raw, sessionIDKeys, 0)
	}
	if probe.Cwd == "" {
		probe.Cwd = findStringByKey(raw, cwdKeys, 0)
	}
	if probe.ProjectName == "" {
		probe.ProjectName = findStringByKey(raw, projectNameKeys, 0)
	}
}

func mergeTranscriptProbes(primary transcriptProbe, fallback transcriptProbe) transcriptProbe {
	if primary.SessionID == "" {
		primary.SessionID = fallback.SessionID
	}
	if primary.Cwd == "" {
		primary.Cwd = fallback.Cwd
	}
	if primary.ProjectName == "" {
		primary.ProjectName = fallback.ProjectName
	}
	return primary
}

var (
	sessionIDKeys = map[string]bool{
		"session_id": true,
		"sessionid":  true,
	}
	cwdKeys = map[string]bool{
		"cwd":                       true,
		"current_working_directory": true,
		"working_directory":         true,
		"workspace":                 true,
	}
	projectNameKeys = map[string]bool{
		"project_name": true,
		"projectname":  true,
	}
)

func findStringByKey(value any, keys map[string]bool, depth int) string {
	if depth > 8 {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if keys[strings.ToLower(key)] {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
		for _, item := range typed {
			if text := findStringByKey(item, keys, depth+1); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := findStringByKey(item, keys, depth+1); text != "" {
				return text
			}
		}
	}
	return ""
}

func sameSessionMeta(a, b SessionMeta) bool {
	return a.Key == b.Key &&
		a.Agent == b.Agent &&
		a.SessionID == b.SessionID &&
		a.TranscriptPath == b.TranscriptPath &&
		a.Cwd == b.Cwd &&
		a.ProjectName == b.ProjectName &&
		a.UpdatedAt.Equal(b.UpdatedAt) &&
		a.FirstPreview == b.FirstPreview &&
		a.LastPreview == b.LastPreview &&
		a.MessageCountEstimate == b.MessageCountEstimate &&
		a.MetadataOnly == b.MetadataOnly &&
		a.InvalidReason == b.InvalidReason
}
