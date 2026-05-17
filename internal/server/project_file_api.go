package server

import (
	"ai-agent-manager/internal/workspace/legacyimport"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var projectRoots = struct {
	sync.RWMutex
	roots map[string]string
}{roots: map[string]string{}}

const (
	projectTreeDefaultLimit   = 400
	projectTreeMaxLimit       = 1000
	projectSearchDefaultLimit = 80
	projectSearchMaxLimit     = 200
	projectSearchMaxVisited   = 20000
	projectFileCacheTTL       = 10 * time.Second
	projectFileCacheMaxItems  = 256
)

type projectFileEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Size        int64  `json:"size,omitempty"`
	ModifiedAt  string `json:"modifiedAt,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Language    string `json:"language,omitempty"`
	HasChildren bool   `json:"hasChildren,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
}

type projectFileListResponse struct {
	ProjectID string             `json:"projectId"`
	Path      string             `json:"path"`
	Entries   []projectFileEntry `json:"entries"`
	Truncated bool               `json:"truncated"`
	Limit     int                `json:"limit"`
}

type projectFileListCacheEntry struct {
	expiresAt time.Time
	response  projectFileListResponse
}

var projectFileListCache = struct {
	sync.Mutex
	items map[string]projectFileListCacheEntry
}{items: map[string]projectFileListCacheEntry{}}

type fileProjectContextResponse struct {
	ProjectID    string `json:"projectId"`
	LocalPath    string `json:"localPath"`
	Title        string `json:"title"`
	RelativePath string `json:"relativePath"`
}

type fileProjectCandidate struct {
	projectID string
	localPath string
	title     string
	root      string
	priority  int
}

func projectFileCacheKey(kind string, root string, pathOrQuery string, limit int) string {
	value := strings.TrimSpace(pathOrQuery)
	if kind == "search" {
		value = strings.ToLower(value)
	}
	return kind + "\x00" + filepath.Clean(root) + "\x00" + value + "\x00" + strconv.Itoa(limit)
}

func getProjectFileListCache(key string) (projectFileListResponse, bool) {
	projectFileListCache.Lock()
	defer projectFileListCache.Unlock()
	entry, ok := projectFileListCache.items[key]
	if !ok {
		return projectFileListResponse{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(projectFileListCache.items, key)
		return projectFileListResponse{}, false
	}
	response := entry.response
	response.Entries = append([]projectFileEntry(nil), response.Entries...)
	return response, true
}

func setProjectFileListCache(key string, response projectFileListResponse) {
	response.Entries = append([]projectFileEntry(nil), response.Entries...)
	projectFileListCache.Lock()
	if len(projectFileListCache.items) >= projectFileCacheMaxItems {
		now := time.Now()
		for itemKey, item := range projectFileListCache.items {
			if now.After(item.expiresAt) || len(projectFileListCache.items) >= projectFileCacheMaxItems {
				delete(projectFileListCache.items, itemKey)
			}
		}
	}
	projectFileListCache.items[key] = projectFileListCacheEntry{
		expiresAt: time.Now().Add(projectFileCacheTTL),
		response:  response,
	}
	projectFileListCache.Unlock()
}

var projectExplorerIgnoredNames = map[string]bool{
	".abolqasem":    true,
	".cache":        true,
	".git":          true,
	".next":         true,
	".nuxt":         true,
	".parcel-cache": true,
	".turbo":        true,
	".vite":         true,
	"build":         true,
	"coverage":      true,
	"dist":          true,
	"node_modules":  true,
	"tmp":           true,
	"vendor":        true,
}

func registerProjectRoot(projectID string, root string) error {
	projectID = safeSegment(projectID)
	if projectID == "" {
		return errors.New("invalid project id")
	}
	rootEval, ok := safePreviewRoot(root)
	if !ok {
		return errors.New("project root is not readable")
	}
	projectRoots.Lock()
	projectRoots.roots[projectID] = rootEval
	projectRoots.Unlock()
	return nil
}

func projectRoot(projectID string) (string, bool) {
	projectRoots.RLock()
	defer projectRoots.RUnlock()
	root, ok := projectRoots.roots[safeSegment(projectID)]
	return root, ok
}

func handleAPIFileContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requestedPath := cleanRequestedPreviewPath(r.URL.Query().Get("path"))
	if requestedPath == "" || !filepath.IsAbs(requestedPath) {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	targetEval, err := filepath.EvalSymlinks(requestedPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	context, ok := projectContextForAbsolutePath(targetEval)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, context)
}

func handleAPIProjectFile(w http.ResponseWriter, r *http.Request, projectID string, rest string) {
	switch strings.Trim(rest, "/") {
	case "tree":
		handleAPIProjectFileTree(w, r, projectID)
		return
	case "search":
		handleAPIProjectFileSearch(w, r, projectID)
		return
	case "preview":
		handleAPIProjectFilePreview(w, r, projectID)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasSuffix(rest, "/content") {
		http.NotFound(w, r)
		return
	}
	rawPath := strings.TrimSuffix(rest, "/content")
	rawPath = strings.Trim(rawPath, "/")
	relativePath, err := url.PathUnescape(rawPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root, err := projectRootRequired(projectID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	absolutePath, err := safeProjectFilePath(root, relativePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(absolutePath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", projectFileMimeType(absolutePath))
	http.ServeFile(w, r, absolutePath)
}

func handleAPIProjectFileTree(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := projectRootRequired(projectID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	relativePath := cleanProjectRelativePath(r.URL.Query().Get("path"), true)
	absolutePath, err := safeProjectPath(root, relativePath, true)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(absolutePath)
	if err != nil || !info.IsDir() {
		http.NotFound(w, r)
		return
	}
	limit := clampInt(parsePositiveInt(r.URL.Query().Get("limit"), projectTreeDefaultLimit), 1, projectTreeMaxLimit)
	cacheKey := projectFileCacheKey("tree", root, relativePath, limit)
	if !isTruthyQueryValue(r.URL.Query().Get("refresh")) {
		if cached, ok := getProjectFileListCache(cacheKey); ok {
			writeJSON(w, cached)
			return
		}
	}
	entries, truncated, err := listProjectFileEntries(root, absolutePath, relativePath, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response := projectFileListResponse{
		ProjectID: safeSegment(projectID),
		Path:      relativePath,
		Entries:   entries,
		Truncated: truncated,
		Limit:     limit,
	}
	setProjectFileListCache(cacheKey, response)
	writeJSON(w, response)
}

func handleAPIProjectFileSearch(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := projectRootRequired(projectID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := clampInt(parsePositiveInt(r.URL.Query().Get("limit"), projectSearchDefaultLimit), 1, projectSearchMaxLimit)
	if query == "" {
		writeJSON(w, projectFileListResponse{
			ProjectID: safeSegment(projectID),
			Path:      "",
			Entries:   []projectFileEntry{},
			Limit:     limit,
		})
		return
	}
	refresh := isTruthyQueryValue(r.URL.Query().Get("refresh"))
	if refresh {
		invalidateProjectFileSearchIndex(root)
	}
	cacheKey := projectFileCacheKey("search", root, query, limit)
	if !refresh {
		if cached, ok := getProjectFileListCache(cacheKey); ok {
			writeJSON(w, cached)
			return
		}
	}
	entries, truncated, err := searchProjectFileEntries(r.Context(), root, query, limit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response := projectFileListResponse{
		ProjectID: safeSegment(projectID),
		Path:      "",
		Entries:   entries,
		Truncated: truncated,
		Limit:     limit,
	}
	setProjectFileListCache(cacheKey, response)
	writeJSON(w, response)
}

func handleAPIProjectFilePreview(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := projectRootRequired(projectID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	relativePath := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if relativePath == "" || relativePath == "." {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	absolutePath, err := safeProjectFilePath(root, relativePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	line := parsePositiveInt(r.URL.Query().Get("line"), 0)
	options := filePreviewOptions{Full: isTruthyQueryValue(r.URL.Query().Get("full"))}
	preview, err := buildFilePreview([]string{root}, absolutePath, line, options)
	if err != nil {
		http.Error(w, err.Error(), filePreviewStatus(err))
		return
	}
	writeJSON(w, preview)
}

func projectRootRequired(projectID string) (string, error) {
	if root, ok := projectRoot(projectID); ok {
		return root, nil
	}
	project, err := workspaceRuntimeProjectRequired(projectID)
	if err != nil || strings.TrimSpace(project.LocalPath) == "" {
		return "", errors.New("project not found")
	}
	return project.LocalPath, nil
}

func projectContextForAbsolutePath(targetPath string) (fileProjectContextResponse, bool) {
	candidates := []fileProjectCandidate{}
	projectRoots.RLock()
	for projectID, root := range projectRoots.roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidates = append(candidates, fileProjectCandidate{
			projectID: projectID,
			localPath: root,
			title:     filepath.Base(root),
			root:      root,
			priority:  0,
		})
	}
	projectRoots.RUnlock()
	if storeState, err := workspaceStore().LoadStateLight(); err == nil {
		for _, project := range storeState.ProjectsByID {
			if project.DeletedAt != 0 || strings.TrimSpace(project.LocalPath) == "" {
				continue
			}
			rootEval, ok := safePreviewRoot(project.LocalPath)
			if !ok {
				continue
			}
			candidates = append(candidates, fileProjectCandidate{
				projectID: project.ID,
				localPath: project.LocalPath,
				title:     project.Title,
				root:      rootEval,
				priority:  0,
			})
		}
	}

	if vcsRoot, ok := projectVCSRootForPath(targetPath); ok {
		candidates = append(candidates, fileProjectCandidateFromRoot(vcsRoot, 1))
	}

	if appState, err := workspaceLoadLegacyState(); err == nil {
		for _, meta := range appState.Sessions {
			if strings.TrimSpace(meta.Cwd) == "" {
				continue
			}
			rootEval, ok := safePreviewRoot(meta.Cwd)
			if !ok {
				continue
			}
			if vcsRoot, ok := projectVCSRootForPath(rootEval); ok && pathInsideRoot(vcsRoot, targetPath) {
				candidates = append(candidates, fileProjectCandidateFromRoot(vcsRoot, 1))
			}
			imported := legacyimport.ImportSession(meta, nil, legacyimport.ImportOptions{})
			candidates = append(candidates, fileProjectCandidate{
				projectID: imported.Project.ID,
				localPath: meta.Cwd,
				title:     imported.Project.Title,
				root:      rootEval,
				priority:  2,
			})
		}
	}

	var best *fileProjectCandidate
	var bestRelativePath string
	for _, item := range candidates {
		relativePath, err := filepath.Rel(item.root, targetPath)
		if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
			continue
		}
		if best == nil || betterFileProjectCandidate(item, *best) {
			itemCopy := item
			best = &itemCopy
			bestRelativePath = filepath.ToSlash(relativePath)
		}
	}
	if best == nil {
		return fileProjectContextResponse{}, false
	}
	if strings.HasPrefix(best.projectID, "file-project-") {
		_ = registerProjectRoot(best.projectID, best.root)
	}
	return fileProjectContextResponse{
		ProjectID:    best.projectID,
		LocalPath:    best.localPath,
		Title:        best.title,
		RelativePath: bestRelativePath,
	}, true
}

func betterFileProjectCandidate(candidate fileProjectCandidate, current fileProjectCandidate) bool {
	if candidate.priority != current.priority {
		return candidate.priority < current.priority
	}
	return len(candidate.root) > len(current.root)
}

func fileProjectCandidateFromRoot(root string, priority int) fileProjectCandidate {
	return fileProjectCandidate{
		projectID: "file-project-" + shortFileProjectHash(root),
		localPath: root,
		title:     filepath.Base(root),
		root:      root,
		priority:  priority,
	}
}

func shortFileProjectHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:16]
}

func projectVCSRootForPath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	for {
		if rootEval, ok := projectVCSRootAt(path); ok {
			return rootEval, true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", false
		}
		path = parent
	}
}

func projectVCSRootAt(path string) (string, bool) {
	gitPath := filepath.Join(path, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		return "", false
	}
	return safePreviewRoot(path)
}

func pathInsideRoot(root string, targetPath string) bool {
	relativePath, err := filepath.Rel(root, targetPath)
	return err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) && !filepath.IsAbs(relativePath)
}

func safeProjectFilePath(root string, relativePath string) (string, error) {
	return safeProjectPath(root, relativePath, false)
}

func safeProjectPath(root string, relativePath string, allowRoot bool) (string, error) {
	rootEval, ok := safePreviewRoot(root)
	if !ok {
		return "", errors.New("project root is not readable")
	}
	relativePath = cleanProjectRelativePath(relativePath, allowRoot)
	if relativePath == "" {
		if allowRoot {
			return rootEval, nil
		}
		return "", errors.New("path must stay inside project root")
	}
	nativeRelativePath := filepath.Clean(filepath.FromSlash(relativePath))
	if nativeRelativePath == "." || filepath.IsAbs(nativeRelativePath) || nativeRelativePath == ".." || strings.HasPrefix(nativeRelativePath, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay inside project root")
	}
	absolutePath := filepath.Join(rootEval, nativeRelativePath)
	targetEval, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootEval, targetEval)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("path must stay inside project root")
	}
	return targetEval, nil
}

func cleanProjectRelativePath(relativePath string, allowRoot bool) string {
	relativePath = strings.TrimSpace(relativePath)
	if unescaped, err := url.PathUnescape(relativePath); err == nil {
		relativePath = unescaped
	}
	relativePath = strings.TrimSpace(strings.ReplaceAll(relativePath, "\\", "/"))
	if relativePath == "" || relativePath == "." || relativePath == "/" {
		if allowRoot {
			return ""
		}
		return "."
	}
	relativePath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(relativePath)))
	if relativePath == "." && allowRoot {
		return ""
	}
	return relativePath
}

func listProjectFileEntries(root string, absoluteDir string, relativeDir string, limit int) ([]projectFileEntry, bool, error) {
	dirEntries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return nil, false, err
	}
	entries := make([]projectFileEntry, 0, minInt(len(dirEntries), limit))
	truncated := false
	for _, dirEntry := range dirEntries {
		childRelativePath := joinProjectRelativePath(relativeDir, dirEntry.Name())
		if shouldIgnoreProjectExplorerEntry(dirEntry.Name(), childRelativePath, dirEntry.IsDir()) {
			continue
		}
		entry, ok := projectFileEntryFromPath(root, childRelativePath)
		if !ok {
			continue
		}
		entries = append(entries, entry)
		if len(entries) >= limit {
			truncated = true
			break
		}
	}
	sortProjectFileEntries(entries)
	return entries, truncated, nil
}

func searchProjectFileEntries(ctx context.Context, root string, query string, limit int) ([]projectFileEntry, bool, error) {
	if entries, truncated, err := searchProjectFileEntriesIndexed(ctx, root, query, limit); err == nil {
		return entries, truncated, nil
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, false, err
	}

	rootEval, ok := safePreviewRoot(root)
	if !ok {
		return nil, false, errors.New("project root is not readable")
	}
	needle := strings.ToLower(query)
	entries := make([]projectFileEntry, 0, limit)
	truncated := false
	visited := 0
	stopWalk := errors.New("project file search stopped")
	err := filepath.WalkDir(rootEval, func(path string, dirEntry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if dirEntry != nil && dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		visited++
		if visited > projectSearchMaxVisited {
			truncated = true
			return stopWalk
		}
		relativePath, err := filepath.Rel(rootEval, path)
		if err != nil {
			return nil
		}
		relativePath = filepath.ToSlash(relativePath)
		if relativePath == "." {
			return nil
		}
		if shouldIgnoreProjectExplorerEntry(dirEntry.Name(), relativePath, dirEntry.IsDir()) {
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.Contains(strings.ToLower(relativePath), needle) {
			return nil
		}
		entry, ok := projectFileEntryFromPath(rootEval, relativePath)
		if !ok {
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entries = append(entries, entry)
		if len(entries) >= limit {
			truncated = true
			return stopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, stopWalk) {
		return nil, false, err
	}
	sortProjectFileEntries(entries)
	return entries, truncated, nil
}

func projectFileEntryFromPath(root string, relativePath string) (projectFileEntry, bool) {
	absolutePath, err := safeProjectPath(root, relativePath, false)
	if err != nil {
		return projectFileEntry{}, false
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return projectFileEntry{}, false
	}
	entryType := "file"
	if info.IsDir() {
		entryType = "directory"
	}
	entry := projectFileEntry{
		Name:       filepath.Base(relativePath),
		Path:       filepath.ToSlash(relativePath),
		Type:       entryType,
		ModifiedAt: formatProjectFileModifiedAt(info.ModTime()),
	}
	if info.IsDir() {
		entry.HasChildren = projectDirectoryHasVisibleChildren(root, absolutePath, relativePath)
	} else {
		entry.Size = info.Size()
		entry.MimeType = projectFileMimeType(absolutePath)
		entry.Language = languageForPath(absolutePath)
	}
	return entry, true
}

func projectDirectoryHasVisibleChildren(root string, absoluteDir string, relativeDir string) bool {
	entries, err := os.ReadDir(absoluteDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		childRelativePath := joinProjectRelativePath(relativeDir, entry.Name())
		if shouldIgnoreProjectExplorerEntry(entry.Name(), childRelativePath, entry.IsDir()) {
			continue
		}
		if _, err := safeProjectPath(root, childRelativePath, false); err == nil {
			return true
		}
	}
	return false
}

func shouldIgnoreProjectExplorerEntry(name string, relativePath string, isDir bool) bool {
	if !isDir {
		return false
	}
	if projectExplorerIgnoredNames[name] {
		return true
	}
	return projectExplorerIgnoredNames[filepath.Base(relativePath)]
}

func joinProjectRelativePath(parent string, name string) string {
	if parent == "" || parent == "." {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(filepath.Join(parent, name))
}

func sortProjectFileEntries(entries []projectFileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "directory"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

func formatProjectFileModifiedAt(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func projectFileMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".tsv":
		return "text/tab-separated-values; charset=utf-8"
	}
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		return mimeType
	}
	if isLikelyTextExt(ext) {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

func isLikelyTextExt(ext string) bool {
	switch ext {
	case ".txt", ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rb", ".rs", ".java", ".c", ".cpp", ".h", ".hpp", ".css", ".html", ".xml", ".yaml", ".yml", ".toml", ".sh", ".sql":
		return true
	default:
		return false
	}
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
