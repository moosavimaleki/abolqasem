package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLegacyViewerRouteServesEmbeddedViewer(t *testing.T) {
	previous := webFS
	webFS = fstest.MapFS{
		"web/index.html": {Data: []byte("<html>legacy viewer</html>")},
		"web/styles.css": {Data: []byte("body{}")},
	}
	t.Cleanup(func() { webFS = previous })

	mux := http.NewServeMux()
	setupRoutes(mux)

	indexResponse := httptest.NewRecorder()
	mux.ServeHTTP(indexResponse, httptest.NewRequest(http.MethodGet, "/legacy/", nil))
	if indexResponse.Code != http.StatusOK {
		t.Fatalf("expected legacy index status 200, got %d", indexResponse.Code)
	}
	if indexResponse.Body.String() != "<html>legacy viewer</html>" {
		t.Fatalf("unexpected legacy index body: %q", indexResponse.Body.String())
	}

	assetResponse := httptest.NewRecorder()
	mux.ServeHTTP(assetResponse, httptest.NewRequest(http.MethodGet, "/legacy/styles.css", nil))
	if assetResponse.Code != http.StatusOK {
		t.Fatalf("expected legacy asset status 200, got %d", assetResponse.Code)
	}
	if assetResponse.Body.String() != "body{}" {
		t.Fatalf("unexpected legacy asset body: %q", assetResponse.Body.String())
	}
}

func TestLegacyViewerRouteRedirectsToSlash(t *testing.T) {
	previous := webFS
	webFS = fstest.MapFS{"web/index.html": {Data: []byte("legacy")}}
	t.Cleanup(func() { webFS = previous })

	mux := http.NewServeMux()
	setupRoutes(mux)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/legacy", nil))
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected temporary redirect, got %d", response.Code)
	}
	if location := response.Header().Get("Location"); location != "/legacy/" {
		t.Fatalf("expected /legacy/ redirect, got %q", location)
	}
}

func TestEmbeddedViewerAssetsUseRelativePathsForLegacyRoute(t *testing.T) {
	legacyFS := webFSOrDisk()
	for _, path := range []string{"index.html", "styles.css", "styles/base.css", "styles/icons.css"} {
		data, err := fs.ReadFile(legacyFS, path)
		if err != nil {
			t.Fatalf("ReadFile %s returned error: %v", path, err)
		}
		content := string(data)
		if containsLegacyAbsoluteAssetPath(content) {
			t.Fatalf("legacy viewer asset paths must be relative under /legacy/ in %s", path)
		}
	}
}

func webFSOrDisk() fs.FS {
	if webFS != nil {
		subFS, err := fs.Sub(webFS, "web")
		if err == nil {
			return subFS
		}
	}
	return fs.FS(os.DirFS("../../web"))
}

func containsLegacyAbsoluteAssetPath(content string) bool {
	return containsAny(content, []string{`href="/`, `src="/`, `url("/`})
}

func containsAny(content string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}
