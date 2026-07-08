package server

import (
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
