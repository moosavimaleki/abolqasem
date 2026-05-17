package server

import (
	"encoding/json"
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

func TestAppIndexRouteRewritesRelativeAssetsForDeepRoutes(t *testing.T) {
	previous := webFS
	webFS = fstest.MapFS{"web/index.html": {Data: []byte(`<script src="./assets/app.js"></script><link href="./assets/app.css">`)}}
	t.Cleanup(func() { webFS = previous })

	mux := http.NewServeMux()
	setupRoutes(mux)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/_/chat/chat-1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected app index status 200, got %d", response.Code)
	}
	if body := response.Body.String(); strings.Contains(body, `src="./`) || strings.Contains(body, `href="./`) {
		t.Fatalf("expected deep app route to use root-relative asset paths, got %q", body)
	}
}

func TestWorkspaceAuthDisabledEndpointsMatchAbolqasemShape(t *testing.T) {
	mux := http.NewServeMux()
	setupRoutes(mux)

	statusResponse := httptest.NewRecorder()
	mux.ServeHTTP(statusResponse, httptest.NewRequest(http.MethodGet, "/auth/status", nil))
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("expected auth status 200, got %d", statusResponse.Code)
	}
	var statusPayload map[string]bool
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &statusPayload); err != nil {
		t.Fatalf("json.Unmarshal status returned error: %v", err)
	}
	if statusPayload["enabled"] || !statusPayload["authenticated"] {
		t.Fatalf("unexpected auth status payload: %#v", statusPayload)
	}

	logoutResponse := httptest.NewRecorder()
	mux.ServeHTTP(logoutResponse, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("expected auth logout 200, got %d", logoutResponse.Code)
	}
	var logoutPayload map[string]bool
	if err := json.Unmarshal(logoutResponse.Body.Bytes(), &logoutPayload); err != nil {
		t.Fatalf("json.Unmarshal logout returned error: %v", err)
	}
	if !logoutPayload["ok"] {
		t.Fatalf("unexpected auth logout payload: %#v", logoutPayload)
	}

	wrongMethodResponse := httptest.NewRecorder()
	mux.ServeHTTP(wrongMethodResponse, httptest.NewRequest(http.MethodGet, "/auth/logout", nil))
	if wrongMethodResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected auth logout GET 405, got %d", wrongMethodResponse.Code)
	}
	if allow := wrongMethodResponse.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow POST, got %q", allow)
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
