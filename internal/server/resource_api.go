package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"ai-agent-manager/internal/parser"
	"ai-agent-manager/internal/workspace/events"
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
	WorkspaceBytes int64            `json:"workspace_bytes"`
	EventStreams   map[string]int64 `json:"event_streams"`
	SearchBytes    int64            `json:"search_bytes"`
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
		Storage: resourceStorageStatsResponse{
			WorkspaceBytes: directorySize(workspaceDataDir()),
			EventStreams:   eventStreamSizes(),
			SearchBytes:    directorySize(filepath.Join(workspaceDataDir(), "search")),
		},
		Checkpoints: checkpointStorageStats(),
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
