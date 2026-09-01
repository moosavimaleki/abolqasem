package server

import (
	"abolqasem/internal/appinfo"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/parser"
	"abolqasem/internal/state"
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
	TotalBytes              int64                         `json:"total_bytes"`
	CacheBytes              int64                         `json:"cache_bytes"`
	UploadBytes             int64                         `json:"upload_bytes"`
	WorkspaceBytes          int64                         `json:"workspace_bytes"`
	DataBytes               int64                         `json:"data_bytes"`
	EventStreams            map[string]int64              `json:"event_streams"`
	EventStreamArchives     map[string]int64              `json:"event_stream_archives"`
	EventStreamArchiveBytes int64                         `json:"event_stream_archive_bytes"`
	SearchBytes             int64                         `json:"search_bytes"`
	IndexBytes              int64                         `json:"index_bytes"`
	CheckpointBytes         int64                         `json:"checkpoint_bytes"`
	ArchiveBytes            int64                         `json:"archive_bytes"`
	ArchiveCount            int                           `json:"archive_count"`
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

const resourceUsageCacheTTL = 30 * time.Second

var resourceUsageCache = struct {
	sync.Mutex
	storage     resourceStorageStatsResponse
	checkpoints resourceCheckpointStats
	measuredAt  time.Time
	refreshing  bool
}{}

var resourceAutoCleanup = struct {
	sync.Mutex
	lastChecked time.Time
}{}

func handleAPIResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, currentResourceUsage(r.URL.Query().Get("fresh") == "1"))
}

func currentResourceUsage(forceStorageRefresh bool) resourceUsageResponse {
	maybeAutoCleanupResources()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	storage, checkpoints := cachedResourceStorageStats(forceStorageRefresh)
	return resourceUsageResponse{
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
		Storage:     storage,
		Checkpoints: checkpoints,
	}
}

// cachedResourceStorageStats keeps the expensive filesystem walks off normal
// page navigation. The sidebar asks for this endpoint on every page; a stale
// but recent disk number is preferable to blocking Chrome for seconds while
// walking thousands of transcripts and cache files.
func cachedResourceStorageStats(force bool) (resourceStorageStatsResponse, resourceCheckpointStats) {
	resourceUsageCache.Lock()
	fresh := !resourceUsageCache.measuredAt.IsZero() && time.Since(resourceUsageCache.measuredAt) < resourceUsageCacheTTL
	if fresh && !force {
		storage, checkpoints := resourceUsageCache.storage, resourceUsageCache.checkpoints
		resourceUsageCache.Unlock()
		return storage, checkpoints
	}
	if !force {
		storage, checkpoints := resourceUsageCache.storage, resourceUsageCache.checkpoints
		if !resourceUsageCache.refreshing {
			resourceUsageCache.refreshing = true
			go refreshResourceUsageCache()
		}
		resourceUsageCache.Unlock()
		return storage, checkpoints
	}
	resourceUsageCache.Unlock()
	storage, checkpoints := collectResourceStorageStats()
	resourceUsageCache.Lock()
	resourceUsageCache.storage = storage
	resourceUsageCache.checkpoints = checkpoints
	resourceUsageCache.measuredAt = time.Now()
	resourceUsageCache.refreshing = false
	resourceUsageCache.Unlock()
	return storage, checkpoints
}

func refreshResourceUsageCache() {
	storage, checkpoints := collectResourceStorageStats()
	resourceUsageCache.Lock()
	resourceUsageCache.storage = storage
	resourceUsageCache.checkpoints = checkpoints
	resourceUsageCache.measuredAt = time.Now()
	resourceUsageCache.refreshing = false
	resourceUsageCache.Unlock()
}

func collectResourceStorageStats() (resourceStorageStatsResponse, resourceCheckpointStats) {
	checkpoints := checkpointStorageStats()
	return resourceStorageStats(checkpoints), checkpoints
}

func invalidateResourceUsageCache() {
	resourceUsageCache.Lock()
	resourceUsageCache.measuredAt = time.Time{}
	resourceUsageCache.Unlock()
}

func maybeAutoCleanupResources() {
	settings, err := state.LoadSettings()
	if err != nil || !settings.DiskManagement.AutoCleanup {
		return
	}
	resourceAutoCleanup.Lock()
	if !resourceAutoCleanup.lastChecked.IsZero() && time.Since(resourceAutoCleanup.lastChecked) < resourceUsageCacheTTL {
		resourceAutoCleanup.Unlock()
		return
	}
	resourceAutoCleanup.lastChecked = time.Now()
	resourceAutoCleanup.Unlock()
	threshold := settings.DiskManagement.WarningThresholdBytes
	if threshold <= 0 {
		return
	}
	go func() {
		storage, _ := collectResourceStorageStats()
		if storage.TotalBytes <= threshold {
			return
		}
		// Automatic cleanup is intentionally limited to rebuildable/indexed data and
		// checkpoints. Durable event streams and user attachments are never removed.
		_, _ = clearResourceCache()
		_ = os.RemoveAll(workspaceCheckpointsDir())
		_ = os.MkdirAll(workspaceCheckpointsDir(), 0o755)
		invalidateResourceUsageCache()
	}()
}

func handleAPIResourceCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clearedBytes, err := clearResourceCache()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"status":        "ok",
		"cleared_bytes": clearedBytes,
		"resources":     currentResourceUsage(true),
	})
}

func clearResourceCache() (int64, error) {
	searchDir := filepath.Join(workspaceDataDir(), "search")
	clearedBytes := directorySize(searchDir)
	if err := os.RemoveAll(searchDir); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(searchDir, 0o755); err != nil {
		return 0, err
	}
	resetWorkspaceSearchCaches()
	parser.ClearCache()
	invalidateResourceUsageCache()
	return clearedBytes, nil
}

func handleAPIResourceCheckpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := workspaceCheckpointsDir()
	clearedBytes := directorySize(root)
	if err := os.RemoveAll(root); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invalidateResourceUsageCache()
	writeJSON(w, map[string]any{"status": "ok", "cleared_bytes": clearedBytes, "resources": currentResourceUsage(true)})
}

func handleAPIResourceArchives(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var clearedBytes int64
	for _, stream := range events.Streams() {
		matches, err := filepath.Glob(filepath.Join(workspaceDataDir(), stream+".jsonl.archived-*"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, path := range matches {
			clearedBytes += directorySize(path)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	invalidateResourceUsageCache()
	writeJSON(w, map[string]any{"status": "ok", "cleared_bytes": clearedBytes, "resources": currentResourceUsage(true)})
}

// handleAPIResourceAttachments removes only uploaded attachment payloads. It
// deliberately leaves transcript events and chat metadata intact; old
// attachment references will render as unavailable after this operation.
func handleAPIResourceAttachments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := resourceUploadRoot()
	clearedBytes := directorySize(root)
	if err := os.RemoveAll(root); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invalidateResourceUsageCache()
	writeJSON(w, map[string]any{"status": "ok", "cleared_bytes": clearedBytes, "resources": currentResourceUsage(true)})
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
	uploadBytes := directorySize(resourceUploadRoot())
	archives := eventStreamArchiveSizes()
	archiveBytes := sumInt64Map(archives)
	archiveCount := eventStreamArchiveCount()
	return resourceStorageStatsResponse{
		TotalBytes:              dataBytes + uploadBytes,
		CacheBytes:              searchBytes,
		UploadBytes:             uploadBytes,
		WorkspaceBytes:          dataBytes,
		DataBytes:               dataBytes,
		EventStreams:            eventStreamSizes(),
		EventStreamArchives:     archives,
		EventStreamArchiveBytes: archiveBytes,
		SearchBytes:             searchBytes,
		IndexBytes:              searchBytes,
		CheckpointBytes:         checkpoints.Bytes,
		ArchiveBytes:            archiveBytes,
		ArchiveCount:            archiveCount,
		NativeTranscripts:       nativeTranscriptStorageStats(),
	}
}

func eventStreamArchiveCount() int {
	count := 0
	entries, err := os.ReadDir(workspaceDataDir())
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		for _, stream := range events.Streams() {
			if strings.HasPrefix(entry.Name(), stream+".jsonl.archived-") {
				count++
				break
			}
		}
	}
	return count
}

func resourceUploadRoot() string {
	if strings.TrimSpace(resourceUploadRootOverride) != "" {
		return resourceUploadRootOverride
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, appinfo.Name, "uploads")
}

// resourceUploadRootOverride is only set by package tests so destructive
// cleanup tests never touch a developer's real attachment cache.
var resourceUploadRootOverride string

func resetWorkspaceSearchCaches() {
	sessionSearchIndex.Lock()
	sessionSearchIndex.indexPath = filepath.Join(workspaceDataDir(), "search", sessionSearchIndexPathPrefix)
	sessionSearchIndex.signature = ""
	sessionSearchIndex.indexedSessions = 0
	sessionSearchIndex.indexedDocs = 0
	sessionSearchIndex.Unlock()

	projectFileSearchIndexes.Lock()
	projectFileSearchIndexes.items = map[string]*projectFileSearchIndexState{}
	projectFileSearchIndexes.Unlock()
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
