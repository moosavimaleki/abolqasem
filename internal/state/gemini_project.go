package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var geminiProjectPathRE = regexp.MustCompile(`(?m)(?:^|[\s"'=:(])(/(?:home|Users|tmp|var)/[^\s"')]+)`)

func resolveGeminiTranscriptProbe(transcriptPath string) transcriptProbe {
	containerDir := geminiTranscriptContainerDir(transcriptPath)
	if containerDir == "" {
		return transcriptProbe{}
	}
	cwd := firstNonEmptyString(
		readGeminiProjectRootMarker(containerDir),
		resolveGeminiProjectRootFromRegistry(containerDir),
		resolveGeminiProjectRootFromLogs(containerDir),
	)
	cwd = normalizePath(cwd)
	if cwd == "" {
		return transcriptProbe{}
	}
	return transcriptProbe{Cwd: cwd, ProjectName: deriveProjectName(cwd, transcriptPath)}
}

func geminiTranscriptContainerDir(transcriptPath string) string {
	transcriptPath = normalizePath(transcriptPath)
	if transcriptPath == "" {
		return ""
	}
	dir := filepath.Dir(transcriptPath)
	if strings.EqualFold(filepath.Base(dir), "chats") {
		return filepath.Dir(dir)
	}
	return dir
}

func readGeminiProjectRootMarker(containerDir string) string {
	markerPath := filepath.Join(containerDir, ".project_root")
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func resolveGeminiProjectRootFromRegistry(containerDir string) string {
	registryPath := geminiProjectsRegistryPath()
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return ""
	}
	var payload struct {
		Projects map[string]string `json:"projects"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	if len(payload.Projects) == 0 {
		return ""
	}
	containerName := strings.TrimSpace(filepath.Base(containerDir))
	if containerName == "" {
		return ""
	}
	if root := geminiProjectRootBySlug(payload.Projects, containerName); root != "" {
		return root
	}
	if geminiLooksLikeLegacyHash(containerName) {
		if root := geminiProjectRootByLegacyHash(payload.Projects, containerName); root != "" {
			return root
		}
	}
	return ""
}

func geminiProjectsRegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(geminiConfigDir(home), "projects.json")
}

func geminiProjectRootBySlug(projects map[string]string, slug string) string {
	if len(projects) == 0 || strings.TrimSpace(slug) == "" {
		return ""
	}
	roots := make([]string, 0, 1)
	for projectPath, projectSlug := range projects {
		if strings.TrimSpace(projectSlug) == slug {
			roots = append(roots, normalizePath(projectPath))
		}
	}
	return chooseGeminiProjectRoot(roots)
}

func geminiProjectRootByLegacyHash(projects map[string]string, legacyHash string) string {
	if len(projects) == 0 || !geminiLooksLikeLegacyHash(legacyHash) {
		return ""
	}
	roots := make([]string, 0, 1)
	for projectPath := range projects {
		normalized := normalizePath(projectPath)
		if geminiLegacyProjectHash(normalized) == legacyHash {
			roots = append(roots, normalized)
		}
	}
	return chooseGeminiProjectRoot(roots)
}

func chooseGeminiProjectRoot(roots []string) string {
	if len(roots) == 0 {
		return ""
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if len(roots[i]) == len(roots[j]) {
			return roots[i] < roots[j]
		}
		return len(roots[i]) > len(roots[j])
	})
	for _, root := range roots {
		if root != "" {
			return root
		}
	}
	return ""
}

func geminiLooksLikeLegacyHash(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func geminiLegacyProjectHash(projectPath string) string {
	sum := sha256.Sum256([]byte(normalizePath(projectPath)))
	return hex.EncodeToString(sum[:])
}

func resolveGeminiProjectRootFromLogs(containerDir string) string {
	logPath := filepath.Join(containerDir, "logs.json")
	file, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 512*1024))
	if err != nil || len(data) == 0 {
		return ""
	}
	matches := geminiProjectPathRE.FindAllStringSubmatch(string(data), -1)
	candidates := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		candidate := normalizePath(strings.TrimSpace(match[1]))
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if info, err := os.Stat(candidate); err == nil {
			if info.IsDir() {
				candidates = append(candidates, candidate)
			} else {
				candidates = append(candidates, filepath.Dir(candidate))
			}
		}
	}
	for _, candidate := range candidates {
		if root := findGeminiProjectRoot(candidate); root != "" {
			return root
		}
	}
	return ""
}

func findGeminiProjectRoot(candidate string) string {
	candidate = normalizePath(candidate)
	if candidate == "" {
		return ""
	}
	current := candidate
	for {
		if info, err := os.Stat(filepath.Join(current, ".git")); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current || parent == "." || parent == string(filepath.Separator) {
			break
		}
		current = parent
	}
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}
