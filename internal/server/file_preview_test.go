package server

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildFilePreviewReturnsSmallWindow(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "service.py")
	body := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	preview, err := buildFilePreview([]string{root}, path, 6, filePreviewOptions{})
	if err != nil {
		t.Fatalf("buildFilePreview returned error: %v", err)
	}
	if preview.Path != path || preview.Line != 6 || preview.Language != "python" {
		t.Fatalf("unexpected preview metadata: %+v", preview)
	}
	if len(preview.Lines) != 11 {
		t.Fatalf("expected 11 lines, got %d", len(preview.Lines))
	}
	if !preview.Lines[5].Highlight || preview.Lines[5].Text != "line 6" {
		t.Fatalf("expected line 6 highlighted, got %+v", preview.Lines[5])
	}
}

func TestBuildFilePreviewReturnsFullFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "service.py")
	body := "line 1\nline 2\nline 3\nline 4\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	preview, err := buildFilePreview([]string{root}, path, 3, filePreviewOptions{Full: true})
	if err != nil {
		t.Fatalf("buildFilePreview returned error: %v", err)
	}
	if !preview.Full {
		t.Fatal("expected full preview")
	}
	if len(preview.Lines) != 4 {
		t.Fatalf("expected full file, got %d lines", len(preview.Lines))
	}
	if !preview.Lines[2].Highlight {
		t.Fatalf("expected target line highlighted, got %+v", preview.Lines[2])
	}
}

func TestBuildFilePreviewRendersMarkdownHTML(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "readme.md")
	body := "# Title\n\n```mermaid\nflowchart TD\n    A --> B\n```\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	preview, err := buildFilePreview([]string{root}, path, 0, filePreviewOptions{Full: true})
	if err != nil {
		t.Fatalf("buildFilePreview returned error: %v", err)
	}
	if preview.Language != "markdown" {
		t.Fatalf("expected markdown language, got %q", preview.Language)
	}
	if !strings.Contains(preview.HTML, "<h1") || !strings.Contains(preview.HTML, "language-mermaid") {
		t.Fatalf("expected rendered markdown html with mermaid class, got %s", preview.HTML)
	}
}

func TestBuildFilePreviewRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.py")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	_, err := buildFilePreview([]string{root}, outside, 1, filePreviewOptions{})
	if !errors.Is(err, errFilePreviewForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestBuildFilePreviewRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows runners")
	}

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.py")
	link := filepath.Join(root, "linked.py")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := buildFilePreview([]string{root}, link, 1, filePreviewOptions{})
	if !errors.Is(err, errFilePreviewForbidden) {
		t.Fatalf("expected forbidden symlink escape, got %v", err)
	}
}

func TestBuildFilePreviewRejectsSensitiveName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("TOKEN=secret\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := buildFilePreview([]string{root}, path, 1, filePreviewOptions{})
	if !errors.Is(err, errFilePreviewUnsupported) {
		t.Fatalf("expected unsupported sensitive file, got %v", err)
	}
}

func TestSafePreviewRootRejectsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("home unavailable: %v", err)
	}
	if _, ok := safePreviewRoot(home); ok {
		t.Fatalf("expected home root to be rejected: %s", home)
	}
}
