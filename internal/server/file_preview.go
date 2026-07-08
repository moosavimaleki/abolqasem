package server

import (
	"abolqasem/internal/render"
	"abolqasem/internal/state"
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
	maxFilePreviewBytes     = 4 * 1024 * 1024
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
	Full      bool              `json:"full"`
	StartLine int               `json:"start_line"`
	EndLine   int               `json:"end_line"`
	Language  string            `json:"language"`
	HTML      string            `json:"html,omitempty"`
	Lines     []filePreviewLine `json:"lines"`
}

type filePreviewOptions struct {
	Full bool
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
	line := parsePositiveInt(r.URL.Query().Get("line"), 0)
	options := filePreviewOptions{Full: isTruthyQueryValue(r.URL.Query().Get("full"))}
	if requestedPath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	appState, err := state.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	roots := []string{}
	if sessionKey != "" {
		sessionMeta, ok := appState.Sessions[sessionKey]
		if !ok {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		roots = append(roots, sessionMeta.Cwd)
	}
	roots = append(roots, previewRootsFromState(appState)...)

	preview, err := buildFilePreview(roots, requestedPath, line, options)
	if err != nil {
		http.Error(w, err.Error(), filePreviewStatus(err))
		return
	}
	writeJSON(w, preview)
}

func buildFilePreview(rootPaths []string, requestedPath string, line int, options filePreviewOptions) (filePreviewResponse, error) {
	if line <= 0 && !options.Full {
		line = 1
	}

	targetPath, err := resolvePreviewPath(rootPaths, requestedPath)
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

	lines, err := readPreviewLines(targetPath, line, options)
	if err != nil {
		return filePreviewResponse{}, err
	}
	if len(lines) == 0 {
		return filePreviewResponse{}, errFilePreviewNotFound
	}

	language := languageForPath(targetPath)
	response := filePreviewResponse{
		Path:      targetPath,
		Line:      line,
		Full:      options.Full,
		StartLine: lines[0].Number,
		EndLine:   lines[len(lines)-1].Number,
		Language:  language,
		Lines:     lines,
	}
	if options.Full && language == "markdown" {
		response.HTML = render.MarkdownToHTML(joinPreviewLines(lines))
	}
	return response, nil
}

func resolvePreviewPath(rootPaths []string, requestedPath string) (string, error) {
	requestedPath = cleanRequestedPreviewPath(requestedPath)
	if requestedPath == "" || !filepath.IsAbs(requestedPath) {
		return "", errFilePreviewForbidden
	}
	targetEval, err := filepath.EvalSymlinks(requestedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errFilePreviewNotFound
		}
		return "", err
	}

	for _, rootPath := range rootPaths {
		rootEval, ok := safePreviewRoot(rootPath)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(rootEval, targetEval)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return targetEval, nil
		}
	}
	return "", errFilePreviewForbidden
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

func readPreviewLines(path string, targetLine int, options filePreviewOptions) ([]filePreviewLine, error) {
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

	start := 1
	end := 0
	if !options.Full {
		start = targetLine - filePreviewContextLines
		if start < 1 {
			start = 1
		}
		end = targetLine + filePreviewContextLines
	}

	lines := make([]filePreviewLine, 0, 256)
	number := 0
	for scanner.Scan() {
		number++
		if number < start {
			continue
		}
		if end > 0 && number > end {
			break
		}
		lines = append(lines, filePreviewLine{
			Number:    number,
			Text:      scanner.Text(),
			Highlight: targetLine > 0 && number == targetLine,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, errFilePreviewUnsupported
	}
	return lines, nil
}

func joinPreviewLines(lines []filePreviewLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, line.Text)
	}
	return strings.Join(parts, "\n")
}

func previewRootsFromState(appState *state.AppState) []string {
	roots := make([]string, 0, len(appState.Sessions))
	seen := map[string]bool{}
	for _, meta := range appState.Sessions {
		rootEval, ok := safePreviewRoot(meta.Cwd)
		if !ok || seen[rootEval] {
			continue
		}
		seen[rootEval] = true
		roots = append(roots, rootEval)
	}
	return roots
}

func safePreviewRoot(rootPath string) (string, bool) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return "", false
	}
	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return "", false
	}
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", false
	}
	if isBroadPreviewRoot(rootEval) {
		return "", false
	}
	return rootEval, true
}

func isBroadPreviewRoot(rootPath string) bool {
	clean := filepath.Clean(rootPath)
	if clean == string(filepath.Separator) || clean == "." {
		return true
	}
	if volume := filepath.VolumeName(clean); volume != "" && clean == volume+string(filepath.Separator) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil {
		homeEval, evalErr := filepath.EvalSymlinks(home)
		if evalErr == nil && filepath.Clean(homeEval) == clean {
			return true
		}
	}
	return false
}

func isTruthyQueryValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "full":
		return true
	default:
		return false
	}
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
