package server

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var projectRoots = struct {
	sync.RWMutex
	roots map[string]string
}{roots: map[string]string{}}

func registerProjectRoot(projectID string, root string) error {
	projectID = safeSegment(projectID)
	if projectID == "" {
		return errors.New("invalid project id")
	}
	root = strings.TrimSpace(root)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return errors.New("project root is not readable")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	projectRoots.Lock()
	projectRoots.roots[projectID] = filepath.Clean(abs)
	projectRoots.Unlock()
	return nil
}

func projectRoot(projectID string) (string, bool) {
	projectRoots.RLock()
	defer projectRoots.RUnlock()
	root, ok := projectRoots.roots[safeSegment(projectID)]
	return root, ok
}

func handleAPIProjectFile(w http.ResponseWriter, r *http.Request, projectID string, rest string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(rest, "/content") {
		http.NotFound(w, r)
		return
	}
	rawPath := strings.TrimSuffix(rest, "/content")
	rawPath = strings.Trim(rawPath, "/")
	relativePath, err := url.PathUnescape(rawPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root, ok := projectRoot(projectID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	absolutePath, err := safeProjectFilePath(root, relativePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(absolutePath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", projectFileMimeType(absolutePath))
	http.ServeFile(w, r, absolutePath)
}

func safeProjectFilePath(root string, relativePath string) (string, error) {
	relativePath = strings.TrimSpace(relativePath)
	if unescaped, err := url.PathUnescape(relativePath); err == nil {
		relativePath = unescaped
	}
	relativePath = filepath.Clean(filepath.FromSlash(relativePath))
	if relativePath == "." || relativePath == "" || filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, "..") {
		return "", errors.New("path must stay inside project root")
	}
	root = filepath.Clean(root)
	absolutePath := filepath.Join(root, relativePath)
	rel, err := filepath.Rel(root, absolutePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("path must stay inside project root")
	}
	return absolutePath, nil
}

func projectFileMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".tsv":
		return "text/tab-separated-values; charset=utf-8"
	}
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}
	if isLikelyTextExt(ext) {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

func isLikelyTextExt(ext string) bool {
	switch ext {
	case ".txt", ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rb", ".rs", ".java", ".c", ".cpp", ".h", ".hpp", ".css", ".html", ".xml", ".yaml", ".yml", ".toml", ".sh", ".sql":
		return true
	default:
		return false
	}
}
