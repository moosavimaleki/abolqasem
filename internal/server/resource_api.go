package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"abolqasem/internal/parser"
	"abolqasem/internal/workspace/events"
)

type resourceUsageResponse struct {
	Memory      runtimeMemStatsResponse      `json:"memory"`
	Caches      resourceCacheStatsResponse   `json:"caches"`
	Search      resourceSearchStatsResponse  `json:"search"`
	Storage     resourceStorageStatsResponse `json:"storage"`
	Checkpoints resourceCheckpointStats      `json:"checkpoints"`
}

type runtimeMemStatsResponse struct {
	Alloc      uint64 `json:"alloc"`
	TotalAlloc uint64 `json:"total_alloc"`
	Sys        uint64 `json:"sys"`
	HeapAlloc  uint64 `json:"heap_alloc"`
	HeapSys    uint64 `json:"heap_sys"`
	HeapIdle   uint64 `json:"heap_idle"`
	HeapInuse  uint64 `json:"heap_inuse"`
	NextGC     uint64 `json:"next_gc"`
	NumGC      uint32 `json:"num_gc"`
	Goroutines int    `json:"goroutines"`
}

type resourceCacheStatsResponse struct {
	Parser                parser.CacheStats                 `json:"parser"`
	LegacyImportedSession workspaceLegacyImportedCacheStats `json:"legacy_imported_session"`
}

type resourceSearchStatsResponse struct {
	SessionIndex sessionSearchIndexStats     `json:"session_index"`
	FileIndexes  projectFileSearchIndexStats `json:"file_indexes"`
}

type resourceStorageStatsResponse struct {
	WorkspaceBytes          int64                         `json:"workspace_bytes"`
	DataBytes               int64                         `json:"data_bytes"`
	EventStreams            map[string]int64              `json:"event_streams"`
	EventStreamArchives     map[string]int64              `json:"event_stream_archives"`
	EventStreamArchiveBytes int64                         `json:"event_stream_archive_bytes"`
	SearchBytes             int64                         `json:"search_bytes"`
	IndexBytes              int64                         `json:"index_bytes"`
	CheckpointBytes         int64                         `json:"checkpoint_bytes"`
	NativeTranscripts       resourceNativeTranscriptStats `json:"native_transcripts"`
}

type resourceNativeTranscriptStats struct {
	Count   int   `json:"count"`
	Missing int   `json:"missing"`
	Bytes   int64 `json:"bytes"`
}

type resourceCheckpointStats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

func handleAPIResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	checkpoints := checkpointStorageStats()
	writeJSON(w, resourceUsageResponse{
		Memory: runtimeMemStatsResponse{
			Alloc:      mem.Alloc,
			TotalAlloc: mem.TotalAlloc,
			Sys:        mem.Sys,
			HeapAlloc:  mem.HeapAlloc,
			HeapSys:    mem.HeapSys,
			HeapIdle:   mem.HeapIdle,
			HeapInuse:  mem.HeapInuse,
			NextGC:     mem.NextGC,
			NumGC:      mem.NumGC,
			Goroutines: runtime.NumGoroutine(),
		},
		Caches: resourceCacheStatsResponse{
			Parser:                parser.Stats(),
			LegacyImportedSession: workspaceLegacyImportedSessionCacheStats(),
		},
		Search: resourceSearchStatsResponse{
			SessionIndex: sessionSearchIndexStatsSnapshot(),
			FileIndexes:  projectFileSearchIndexStatsSnapshot(),
		},
		Storage:     resourceStorageStats(checkpoints),
		Checkpoints: checkpoints,
	})
}

func handleAPIResourceCompact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := workspaceStore()
	state, err := store.LoadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := store.Compact(state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func eventStreamSizes() map[string]int64 {
	out := map[string]int64{}
	for _, stream := range events.Streams() {
		info, err := os.Stat(filepath.Join(workspaceDataDir(), stream+".jsonl"))
		if err == nil {
			out[stream] = info.Size()
		} else {
			out[stream] = 0
		}
	}
	return out
}

func resourceStorageStats(checkpoints resourceCheckpointStats) resourceStorageStatsResponse {
	dataBytes := directorySize(workspaceDataDir())
	searchBytes := directorySize(filepath.Join(workspaceDataDir(), "search"))
	archives := eventStreamArchiveSizes()
	return resourceStorageStatsResponse{
		WorkspaceBytes:          dataBytes,
		DataBytes:               dataBytes,
		EventStreams:            eventStreamSizes(),
		EventStreamArchives:     archives,
		EventStreamArchiveBytes: sumInt64Map(archives),
		SearchBytes:             searchBytes,
		IndexBytes:              searchBytes,
		CheckpointBytes:         checkpoints.Bytes,
		NativeTranscripts:       nativeTranscriptStorageStats(),
	}
}

func eventStreamArchiveSizes() map[string]int64 {
	out := map[string]int64{}
	entries, err := os.ReadDir(workspaceDataDir())
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, stream := range events.Streams() {
			if !strings.HasPrefix(name, stream+".jsonl.archived-") {
				continue
			}
			info, err := entry.Info()
			if err == nil {
				out[stream] += info.Size()
			}
			break
		}
	}
	return out
}

func nativeTranscriptStorageStats() resourceNativeTranscriptStats {
	paths := knownNativeTranscriptPaths()
	stats := resourceNativeTranscriptStats{Count: len(paths)}
	for path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			stats.Missing++
			continue
		}
		stats.Bytes += info.Size()
	}
	return stats
}

func knownNativeTranscriptPaths() map[string]struct{} {
	paths := map[string]struct{}{}
	if appState, err := workspaceLoadLegacyState(); err == nil {
		for _, meta := range appState.Sessions {
			if path := strings.TrimSpace(meta.TranscriptPath); path != "" {
				paths[path] = struct{}{}
			}
		}
	}
	if workspaceState, err := workspaceStore().LoadStateLight(); err == nil {
		for _, chat := range workspaceState.ChatsByID {
			if path := strings.TrimSpace(chat.NativeTranscriptPath); path != "" {
				paths[path] = struct{}{}
			}
		}
	}
	return paths
}

func sumInt64Map(values map[string]int64) int64 {
	var total int64
	for _, value := range values {
		total += value
	}
	return total
}

func checkpointStorageStats() resourceCheckpointStats {
	entries, err := os.ReadDir(workspaceCheckpointsDir())
	if err != nil {
		return resourceCheckpointStats{}
	}
	stats := resourceCheckpointStats{Bytes: directorySize(workspaceCheckpointsDir())}
	for _, entry := range entries {
		if entry.IsDir() {
			stats.Count++
		}
	}
	return stats
}

func directorySize(root string) int64 {
	var total int64
	if root == "" {
		return 0
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
