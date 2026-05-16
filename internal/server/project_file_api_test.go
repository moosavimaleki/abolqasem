package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectFileServingServesRegisteredProjectFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Title\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := registerProjectRoot("project-1", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects/project-1/files/README.md/content", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("expected markdown content type, got %q", response.Header().Get("Content-Type"))
	}
	if response.Body.String() != "# Title\n" {
		t.Fatalf("unexpected body: %q", response.Body.String())
	}
}

func TestProjectFileServingRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	parentSecret := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(parentSecret, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write secret failed: %v", err)
	}
	if err := registerProjectRoot("project-2", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects/project-2/files/../secret.txt/content", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for traversal, got %d", response.Code)
	}
}

func TestProjectFileServingRequiresRegisteredRoot(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/projects/unknown/files/README.md/content", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestSafeProjectFilePathRejectsEncodedTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := safeProjectFilePath(root, "%2e%2e/secret.txt"); err == nil {
		t.Fatal("expected encoded-looking traversal to be rejected after clean")
	}
}
