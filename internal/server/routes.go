package server

import (
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var webFS fs.FS

func SetWebFS(f fs.FS) {
	webFS = f
}

func setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", handleAPIState)
	mux.HandleFunc("/api/sessions", handleAPISessions)
	mux.HandleFunc("/api/search", handleAPISearch)
	mux.HandleFunc("/api/hook", handleAPIHook)
	mux.HandleFunc("/api/session/", handleAPISessionMessages)
	mux.HandleFunc("/api/file-preview", handleAPIFilePreview)
	mux.HandleFunc("/api/events", handleAPIEvents)

	rootFS := fs.FS(os.DirFS("web"))
	if webFS != nil {
		subFS, _ := fs.Sub(webFS, "web")
		rootFS = subFS
	}
	fileServer := http.FileServer(http.FS(rootFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isLocalFileRoute(r.URL.Path) {
			serveAppIndex(w, rootFS)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveAppIndex(w http.ResponseWriter, rootFS fs.FS) {
	data, err := fs.ReadFile(rootFS, "index.html")
	if err != nil {
		http.Error(w, "app index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func isLocalFileRoute(rawPath string) bool {
	if strings.HasPrefix(rawPath, "/api/") || rawPath == "/" {
		return false
	}
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		return false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	path = stripLineSuffix(path)
	path = cleanRequestedPreviewPath(path)
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	return looksLikeLocalFilesystemPath(path)
}

func stripLineSuffix(path string) string {
	if idx := strings.LastIndex(path, ":"); idx > 0 && idx < len(path)-1 {
		suffix := path[idx+1:]
		for _, ch := range suffix {
			if ch < '0' || ch > '9' {
				return path
			}
		}
		return path[:idx]
	}
	return path
}

func looksLikeLocalFilesystemPath(path string) bool {
	slashPath := filepath.ToSlash(path)
	return strings.HasPrefix(slashPath, "/home/") ||
		strings.HasPrefix(slashPath, "/Users/") ||
		strings.HasPrefix(slashPath, "/tmp/") ||
		strings.HasPrefix(slashPath, "/var/") ||
		(len(slashPath) >= 3 && slashPath[1] == ':' && slashPath[2] == '/')
}
