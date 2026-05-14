package server

import (
	"ai-agent-manager/internal/state"
	"bufio"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	filePreviewContextLines = 8
	maxFilePreviewBytes     = 2 * 1024 * 1024
	maxFilePreviewLineBytes = 1024 * 1024
)

var (
	errFilePreviewForbidden   = errors.New("file preview is not allowed for this path")
	errFilePreviewNotFound    = errors.New("file preview target was not found")
	errFilePreviewTooLarge    = errors.New("file preview target is too large")
	errFilePreviewUnsupported = errors.New("file preview target is not a supported text/code file")
)

type filePreviewResponse struct {
	Path      string            `json:"path"`
	Line      int               `json:"line"`
	StartLine int               `json:"start_line"`
	EndLine   int               `json:"end_line"`
	Language  string            `json:"language"`
	Lines     []filePreviewLine `json:"lines"`
}

type filePreviewLine struct {
	Number    int    `json:"number"`
	Text      string `json:"text"`
	Highlight bool   `json:"highlight"`
}

func handleAPIFilePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionKey := strings.TrimSpace(r.URL.Query().Get("session_key"))
	requestedPath := strings.TrimSpace(r.URL.Query().Get("path"))
	line := parsePositiveInt(r.URL.Query().Get("line"), 1)
	if sessionKey == "" || requestedPath == "" {
		http.Error(w, "session_key and path are required", http.StatusBadRequest)
		return
	}

	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sessionMeta, ok := appState.Sessions[sessionKey]
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	preview, err := buildFilePreview(sessionMeta.Cwd, requestedPath, line)
	if err != nil {
		http.Error(w, err.Error(), filePreviewStatus(err))
		return
	}
	writeJSON(w, preview)
}

func buildFilePreview(rootPath, requestedPath string, line int) (filePreviewResponse, error) {
	if line <= 0 {
		line = 1
	}

	targetPath, err := resolvePreviewPath(rootPath, requestedPath)
	if err != nil {
		return filePreviewResponse{}, err
	}
	if !isSupportedPreviewFile(targetPath) {
		return filePreviewResponse{}, errFilePreviewUnsupported
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return filePreviewResponse{}, errFilePreviewNotFound
		}
		return filePreviewResponse{}, err
	}
	if !info.Mode().IsRegular() {
		return filePreviewResponse{}, errFilePreviewUnsupported
	}
	if info.Size() > maxFilePreviewBytes {
		return filePreviewResponse{}, errFilePreviewTooLarge
	}

	lines, err := readPreviewLines(targetPath, line)
	if err != nil {
		return filePreviewResponse{}, err
	}
	if len(lines) == 0 {
		return filePreviewResponse{}, errFilePreviewNotFound
	}

	return filePreviewResponse{
		Path:      targetPath,
		Line:      line,
		StartLine: lines[0].Number,
		EndLine:   lines[len(lines)-1].Number,
		Language:  languageForPath(targetPath),
		Lines:     lines,
	}, nil
}

func resolvePreviewPath(rootPath, requestedPath string) (string, error) {
	rootPath = strings.TrimSpace(rootPath)
	requestedPath = cleanRequestedPreviewPath(requestedPath)
	if rootPath == "" || requestedPath == "" || !filepath.IsAbs(requestedPath) {
		return "", errFilePreviewForbidden
	}

	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return "", errFilePreviewForbidden
	}
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", errFilePreviewForbidden
	}
	targetEval, err := filepath.EvalSymlinks(requestedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errFilePreviewNotFound
		}
		return "", err
	}

	rel, err := filepath.Rel(rootEval, targetEval)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errFilePreviewForbidden
	}
	return targetEval, nil
}

func cleanRequestedPreviewPath(path string) string {
	path = strings.TrimSpace(path)
	if runtime.GOOS == "windows" && len(path) >= 4 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func readPreviewLines(path string, targetLine int) ([]filePreviewLine, error) {
	start := targetLine - filePreviewContextLines
	if start < 1 {
		start = 1
	}
	end := targetLine + filePreviewContextLines

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errFilePreviewNotFound
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFilePreviewLineBytes)

	lines := make([]filePreviewLine, 0, (end-start)+1)
	number := 0
	for scanner.Scan() {
		number++
		if number < start {
			continue
		}
		if number > end {
			break
		}
		lines = append(lines, filePreviewLine{
			Number:    number,
			Text:      scanner.Text(),
			Highlight: number == targetLine,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, errFilePreviewUnsupported
	}
	return lines, nil
}

func filePreviewStatus(err error) int {
	switch {
	case errors.Is(err, errFilePreviewForbidden):
		return http.StatusForbidden
	case errors.Is(err, errFilePreviewNotFound):
		return http.StatusNotFound
	case errors.Is(err, errFilePreviewTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, errFilePreviewUnsupported):
		return http.StatusUnsupportedMediaType
	default:
		return http.StatusInternalServerError
	}
}

func isSupportedPreviewFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if isSensitivePreviewName(base) {
		return false
	}
	if supportedPreviewBasenames[base] {
		return true
	}
	return supportedPreviewExts[strings.ToLower(filepath.Ext(path))]
}

func isSensitivePreviewName(base string) bool {
	if strings.HasPrefix(base, ".env") {
		return true
	}
	switch base {
	case "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "known_hosts":
		return true
	default:
		return strings.HasSuffix(base, ".pem") ||
			strings.HasSuffix(base, ".key") ||
			strings.HasSuffix(base, ".p12") ||
			strings.HasSuffix(base, ".pfx")
	}
}

var supportedPreviewBasenames = map[string]bool{
	"dockerfile":        true,
	"gemfile":           true,
	"go.mod":            true,
	"go.sum":            true,
	"justfile":          true,
	"makefile":          true,
	"package-lock.json": true,
	"package.json":      true,
	"pnpm-lock.yaml":    true,
	"pyproject.toml":    true,
	"requirements.txt":  true,
	"taskfile.yml":      true,
	"taskfile.yaml":     true,
	"yarn.lock":         true,
}

var supportedPreviewExts = map[string]bool{
	".bash":  true,
	".c":     true,
	".cc":    true,
	".cfg":   true,
	".conf":  true,
	".cpp":   true,
	".cs":    true,
	".css":   true,
	".csv":   true,
	".go":    true,
	".h":     true,
	".hpp":   true,
	".html":  true,
	".java":  true,
	".js":    true,
	".json":  true,
	".jsx":   true,
	".kt":    true,
	".lua":   true,
	".md":    true,
	".php":   true,
	".proto": true,
	".py":    true,
	".rb":    true,
	".rs":    true,
	".scss":  true,
	".sh":    true,
	".sql":   true,
	".swift": true,
	".tf":    true,
	".toml":  true,
	".ts":    true,
	".tsx":   true,
	".txt":   true,
	".xml":   true,
	".yaml":  true,
	".yml":   true,
	".zsh":   true,
}

func languageForPath(path string) string {
	base := strings.ToLower(filepath.Base(path))
	if language, ok := basenameLanguages[base]; ok {
		return language
	}
	ext := strings.ToLower(filepath.Ext(path))
	if language, ok := extensionLanguages[ext]; ok {
		return language
	}
	return "plaintext"
}

var basenameLanguages = map[string]string{
	"dockerfile":   "dockerfile",
	"gemfile":      "ruby",
	"go.mod":       "go",
	"justfile":     "makefile",
	"makefile":     "makefile",
	"package.json": "json",
}

var extensionLanguages = map[string]string{
	".bash":  "bash",
	".c":     "c",
	".cc":    "cpp",
	".conf":  "ini",
	".cpp":   "cpp",
	".cs":    "csharp",
	".css":   "css",
	".go":    "go",
	".h":     "c",
	".hpp":   "cpp",
	".html":  "xml",
	".java":  "java",
	".js":    "javascript",
	".json":  "json",
	".jsx":   "javascript",
	".kt":    "kotlin",
	".lua":   "lua",
	".md":    "markdown",
	".php":   "php",
	".proto": "protobuf",
	".py":    "python",
	".rb":    "ruby",
	".rs":    "rust",
	".scss":  "scss",
	".sh":    "bash",
	".sql":   "sql",
	".swift": "swift",
	".tf":    "hcl",
	".toml":  "toml",
	".ts":    "typescript",
	".tsx":   "typescript",
	".xml":   "xml",
	".yaml":  "yaml",
	".yml":   "yaml",
	".zsh":   "bash",
}
