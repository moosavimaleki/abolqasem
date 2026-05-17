package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGeminiTranscriptProbeUsesProjectRootMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))

	projectRoot := filepath.Join(home, "work", "jax-xla")
	transcriptPath := filepath.Join(home, ".gemini", "tmp", "jax-xla", "chats", "session-2026-05-06.jsonl")
	writeDiscoveryFile(t, transcriptPath, `{"sessionId":"abc"}`+"\n")
	writeDiscoveryFile(t, filepath.Join(home, ".gemini", "tmp", "jax-xla", ".project_root"), projectRoot)

	probe := resolveGeminiTranscriptProbe(transcriptPath)
	if probe.Cwd != projectRoot {
		t.Fatalf("expected cwd %q, got %q", projectRoot, probe.Cwd)
	}
	if probe.ProjectName != "jax-xla" {
		t.Fatalf("expected project name jax-xla, got %q", probe.ProjectName)
	}
}

func TestResolveGeminiTranscriptProbeUsesProjectsRegistryForLegacyHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))

	projectRoot := filepath.Join(home, "Projects", "Revenue", "adwiser")
	legacyHash := geminiLegacyProjectHash(projectRoot)
	transcriptPath := filepath.Join(home, ".gemini", "tmp", legacyHash, "chats", "session-2025-12-29.json")
	writeDiscoveryFile(t, transcriptPath, `{"sessionId":"gem-1","projectHash":"`+legacyHash+`"}`)

	registryPath := filepath.Join(home, ".gemini", "projects.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("mkdir registry: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"projects": map[string]string{
			projectRoot: "adwiser",
		},
	})
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	probe := resolveGeminiTranscriptProbe(transcriptPath)
	if probe.Cwd != projectRoot {
		t.Fatalf("expected cwd %q, got %q", projectRoot, probe.Cwd)
	}
	if probe.ProjectName != "adwiser" {
		t.Fatalf("expected project name adwiser, got %q", probe.ProjectName)
	}
}

func TestDiscoverSessionsInRootsResolvesGeminiProjectFromStorageMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))

	geminiRoot := filepath.Join(home, ".gemini", "tmp")
	projectRoot := filepath.Join(home, "Projects", "Revenue", "adwiser")
	legacyHash := geminiLegacyProjectHash(projectRoot)
	geminiPath := writeDiscoveryFile(t,
		filepath.Join(geminiRoot, legacyHash, "chats", "session-2025-12-29T04-02-23b99565.json"),
		`{"sessionId":"gemini-session","projectHash":"`+legacyHash+`","messages":[{"type":"user","content":"hi"}]}`,
	)
	registryPath := filepath.Join(home, ".gemini", "projects.json")
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		t.Fatalf("mkdir registry: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"projects": map[string]string{
			projectRoot: "adwiser",
		},
	})
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	appState := newAppState()
	report, err := DiscoverSessionsInRoots(appState, []DiscoveryRoot{{Agent: "gemini", Path: geminiRoot}})
	if err != nil {
		t.Fatalf("DiscoverSessionsInRoots returned error: %v", err)
	}
	if report.Found != 1 || report.Added != 1 {
		t.Fatalf("unexpected discovery report: %+v", report)
	}
	meta, ok := appState.Sessions["gemini:gemini-session"]
	if !ok {
		t.Fatalf("expected gemini session in state")
	}
	if meta.Cwd != projectRoot {
		t.Fatalf("expected cwd %q, got %q", projectRoot, meta.Cwd)
	}
	if meta.ProjectName != "adwiser" {
		t.Fatalf("expected project name adwiser, got %q", meta.ProjectName)
	}
	if meta.TranscriptPath != geminiPath {
		t.Fatalf("expected transcript path %q, got %q", geminiPath, meta.TranscriptPath)
	}
}
