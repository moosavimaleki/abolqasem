package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"abolqasem/internal/workspace/events"
)

func TestResourceStorageStatsReportsArchivesNativeTranscriptsAndIndexes(t *testing.T) {
	withWorkspaceComposerStore(t)
	dataDir := workspaceDataDir()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, events.StreamMessages+".jsonl.archived-test"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "search", "sessions-v1"), 0o755); err != nil {
		t.Fatalf("mkdir search dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "search", "sessions-v1", "index.bin"), []byte("idx1"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	checkpointDir := filepath.Join(workspaceCheckpointsDir(), "checkpoint-1")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatalf("mkdir checkpoint dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(checkpointDir, "metadata.json"), []byte("check"), 0o644); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	nativePath := filepath.Join(t.TempDir(), "native.jsonl")
	if err := os.WriteFile(nativePath, []byte("native"), 0o644); err != nil {
		t.Fatalf("write native transcript: %v", err)
	}
	project, err := workspaceOpenProject(t.TempDir(), "Project")
	if err != nil {
		t.Fatalf("workspaceOpenProject returned error: %v", err)
	}
	chat, err := (&workspaceEventStore{store: workspaceStore()}).CreateChat(project.ID)
	if err != nil {
		t.Fatalf("CreateChat returned error: %v", err)
	}
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatRuntimeSet, time.Now().UnixMilli(), map[string]any{
		"chatId":               chat.ID,
		"nativeTranscriptPath": nativePath,
	}); err != nil {
		t.Fatalf("append runtime metadata: %v", err)
	}

	stats := resourceStorageStats(checkpointStorageStats())
	if stats.EventStreamArchives[events.StreamMessages] != int64(len("old")) {
		t.Fatalf("expected messages archive bytes, got %#v", stats.EventStreamArchives)
	}
	if stats.EventStreamArchiveBytes != int64(len("old")) {
		t.Fatalf("expected archive byte total, got %d", stats.EventStreamArchiveBytes)
	}
	if stats.IndexBytes != int64(len("idx1")) || stats.SearchBytes != int64(len("idx1")) {
		t.Fatalf("expected search/index bytes, got search=%d index=%d", stats.SearchBytes, stats.IndexBytes)
	}
	if stats.CheckpointBytes != int64(len("check")) {
		t.Fatalf("expected checkpoint bytes, got %d", stats.CheckpointBytes)
	}
	if stats.NativeTranscripts.Count != 1 || stats.NativeTranscripts.Bytes != int64(len("native")) || stats.NativeTranscripts.Missing != 0 {
		t.Fatalf("unexpected native transcript stats: %#v", stats.NativeTranscripts)
	}
}

func TestHandleAPIResourceCacheClearsOnlyDerivedSearchData(t *testing.T) {
	withWorkspaceComposerStore(t)
	dataDir := workspaceDataDir()
	searchFile := filepath.Join(dataDir, "search", "sessions-v1", "index.bin")
	messageFile := filepath.Join(dataDir, events.StreamMessages+".jsonl")
	checkpointFile := filepath.Join(workspaceCheckpointsDir(), "checkpoint-1", "checkpoint.json")
	for path, content := range map[string]string{
		searchFile:     "derived",
		messageFile:    "message",
		checkpointFile: "checkpoint",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/resources/cache", nil)
	recorder := httptest.NewRecorder()
	handleAPIResourceCache(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		ClearedBytes int64 `json:"cleared_bytes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ClearedBytes != int64(len("derived")) {
		t.Fatalf("expected cleared bytes %d, got %d", len("derived"), response.ClearedBytes)
	}
	if _, err := os.Stat(searchFile); !os.IsNotExist(err) {
		t.Fatalf("expected search index to be removed, stat err=%v", err)
	}
	for _, path := range []string{messageFile, checkpointFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected durable data %s to remain: %v", path, err)
		}
	}
}

func TestHandleAPIResourceCheckpointsAndArchivesClearOnlySelectedData(t *testing.T) {
	withWorkspaceComposerStore(t)
	checkpointFile := filepath.Join(workspaceCheckpointsDir(), "checkpoint-1", "checkpoint.json")
	archiveFile := filepath.Join(workspaceDataDir(), events.StreamMessages+".jsonl.archived-test")
	activeFile := filepath.Join(workspaceDataDir(), events.StreamMessages+".jsonl")
	for path, content := range map[string]string{checkpointFile: "checkpoint", archiveFile: "archive", activeFile: "active"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	for _, endpoint := range []struct {
		path    string
		handler http.HandlerFunc
		gone    string
		keep    string
	}{
		{"/api/resources/checkpoints", handleAPIResourceCheckpoints, checkpointFile, activeFile},
		{"/api/resources/archives", handleAPIResourceArchives, archiveFile, activeFile},
	} {
		recorder := httptest.NewRecorder()
		handler := endpoint.handler
		handler(recorder, httptest.NewRequest(http.MethodDelete, endpoint.path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", endpoint.path, recorder.Code, recorder.Body.String())
		}
		if _, err := os.Stat(endpoint.gone); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", endpoint.gone, err)
		}
		if _, err := os.Stat(endpoint.keep); err != nil {
			t.Fatalf("expected active stream to remain: %v", err)
		}
	}
}
