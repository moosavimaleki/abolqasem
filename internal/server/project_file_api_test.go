package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/legacyimport"
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

func TestProjectFileServingRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows runners")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	link := filepath.Join(root, "linked.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside failed: %v", err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create symlink failed: %v", err)
	}
	if err := registerProjectRoot("project-symlink", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects/project-symlink/files/linked.txt/content", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for symlink escape, got %d", response.Code)
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

func TestProjectFileServingResolvesLegacyProjectRoot(t *testing.T) {
	withWorkspaceComposerStore(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Legacy\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:legacy-files",
		Agent:          "codex",
		SessionID:      "legacy-files",
		TranscriptPath: filepath.Join(root, "rollout.jsonl"),
		Cwd:            root,
		ProjectName:    "Legacy Files",
		UpdatedAt:      time.Unix(1700000000, 0),
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}})
	projectID := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{}).Project.ID

	request := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/files/README.md/content", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 for legacy project file, got %d", response.Code)
	}
	if response.Body.String() != "# Legacy\n" {
		t.Fatalf("unexpected body: %q", response.Body.String())
	}
}

func TestSafeProjectFilePathRejectsEncodedTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := safeProjectFilePath(root, "%2e%2e/secret.txt"); err == nil {
		t.Fatal("expected encoded-looking traversal to be rejected after clean")
	}
}

func TestRegisterProjectRootRejectsBroadRoots(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("home unavailable: %v", err)
	}
	if err := registerProjectRoot("project-home", home); err == nil {
		t.Fatal("expected home directory root to be rejected")
	}
}

func TestProjectFileTreeListsVisibleEntriesAndSkipsGeneratedFolders(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "src"))
	mustMkdir(t, filepath.Join(root, "node_modules"))
	mustWriteFile(t, filepath.Join(root, "src", "main.go"), "package main\n")
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Readme\n")
	mustWriteFile(t, filepath.Join(root, "node_modules", "dep.js"), "module.exports = {}\n")
	if err := registerProjectRoot("project-tree", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects/project-tree/files/tree", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload projectFileListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	paths := projectFileEntryPaths(payload.Entries)
	if strings.Join(paths, ",") != "src,README.md" {
		t.Fatalf("unexpected tree paths: %#v", paths)
	}
	if payload.Entries[0].Type != "directory" || !payload.Entries[0].HasChildren {
		t.Fatalf("expected src directory with children, got %#v", payload.Entries[0])
	}
}

func TestProjectFileTreeRejectsTraversalAndSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows runners")
	}
	root := t.TempDir()
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "secret.txt"), "secret")
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatalf("create symlink failed: %v", err)
	}
	if err := registerProjectRoot("project-tree-secure", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	for _, target := range []string{
		"/api/projects/project-tree-secure/files/tree?path=../",
		"/api/projects/project-tree-secure/files/tree?path=outside",
	} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		handleAPIProjects(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", target, response.Code)
		}
	}
}

func TestProjectFileSearchFindsMatchingProjectPaths(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "internal", "server"))
	mustMkdir(t, filepath.Join(root, ".git"))
	mustWriteFile(t, filepath.Join(root, "internal", "server", "project_file_api.go"), "package server\n")
	mustWriteFile(t, filepath.Join(root, ".git", "config"), "[core]\n")
	if err := registerProjectRoot("project-search", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects/project-search/files/search?q=file", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload projectFileListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	paths := projectFileEntryPaths(payload.Entries)
	if len(paths) != 1 || paths[0] != "internal/server/project_file_api.go" {
		t.Fatalf("unexpected search paths: %#v", paths)
	}
}

func TestProjectFilePreviewUsesRelativePathInsideProject(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Project\n\nbody\n")
	if err := registerProjectRoot("project-preview", root); err != nil {
		t.Fatalf("registerProjectRoot failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects/project-preview/files/preview?path=README.md&full=1", nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload filePreviewResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.Language != "markdown" || !strings.Contains(payload.HTML, "<h1") {
		t.Fatalf("expected rendered markdown preview, got %#v", payload)
	}
}

func TestFileContextResolvesWorkspaceProjectForAbsolutePath(t *testing.T) {
	withWorkspaceComposerStore(t)

	root := t.TempDir()
	target := filepath.Join(root, "src", "main.go")
	mustWriteFile(t, target, "package main\n")
	project, err := workspaceOpenProject(root, "Context Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/file-context?path="+url.QueryEscape(target), nil)
	response := httptest.NewRecorder()
	handleAPIFileContext(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload fileProjectContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.ProjectID != project.ID || payload.RelativePath != "src/main.go" {
		t.Fatalf("unexpected file context: %#v", payload)
	}
}

func TestFileContextPrefersKnownProjectRootOverNestedLegacySession(t *testing.T) {
	withWorkspaceComposerStore(t)

	root := t.TempDir()
	target := filepath.Join(root, "scripts", "open_ui.py")
	mustWriteFile(t, target, "print('ok')\n")
	project, err := workspaceOpenProject(root, "Known Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject failed: %v", err)
	}
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{
		"codex:nested": {
			Key:            "codex:nested",
			Agent:          "codex",
			SessionID:      "nested",
			TranscriptPath: filepath.Join(root, "scripts", "rollout.jsonl"),
			Cwd:            filepath.Join(root, "scripts"),
			ProjectName:    "scripts",
			UpdatedAt:      time.Unix(1700000000, 0),
		},
	}})

	request := httptest.NewRequest(http.MethodGet, "/api/file-context?path="+url.QueryEscape(target), nil)
	response := httptest.NewRecorder()
	handleAPIFileContext(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload fileProjectContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if payload.ProjectID != project.ID || payload.LocalPath != root || payload.RelativePath != "scripts/open_ui.py" {
		t.Fatalf("expected known project root, got %#v", payload)
	}
}

func TestFileContextFallsBackToGitRootBeforeContainingFolder(t *testing.T) {
	withWorkspaceComposerStore(t)

	root := canonicalServerTestPath(t, t.TempDir())
	mustMkdir(t, filepath.Join(root, ".git"))
	target := filepath.Join(root, "scripts", "open_ui.py")
	mustWriteFile(t, target, "print('ok')\n")
	withLegacyState(t, &state.AppState{Sessions: map[string]state.SessionMeta{
		"codex:nested-git": {
			Key:            "codex:nested-git",
			Agent:          "codex",
			SessionID:      "nested-git",
			TranscriptPath: filepath.Join(root, "scripts", "rollout.jsonl"),
			Cwd:            filepath.Join(root, "scripts"),
			ProjectName:    "scripts",
			UpdatedAt:      time.Unix(1700000000, 0),
		},
	}})

	request := httptest.NewRequest(http.MethodGet, "/api/file-context?path="+url.QueryEscape(target), nil)
	response := httptest.NewRecorder()
	handleAPIFileContext(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload fileProjectContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !strings.HasPrefix(payload.ProjectID, "file-project-") || payload.LocalPath != root || payload.RelativePath != "scripts/open_ui.py" {
		t.Fatalf("expected git project root fallback, got %#v", payload)
	}

	treeRequest := httptest.NewRequest(http.MethodGet, "/api/projects/"+payload.ProjectID+"/files/tree", nil)
	treeResponse := httptest.NewRecorder()
	handleAPIProjects(treeResponse, treeRequest)
	if treeResponse.Code != http.StatusOK {
		t.Fatalf("expected registered fallback project tree, got %d: %s", treeResponse.Code, treeResponse.Body.String())
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
}

func canonicalServerTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks %s failed: %v", path, err)
	}
	return canonical
}

func projectFileEntryPaths(entries []projectFileEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return paths
}
