package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blugelabs/bluge"
)

const (
	projectFileSearchIndexTTL           = 5 * time.Minute
	projectFileSearchIndexBatchSize     = 256
	projectFileSearchIndexPathPrefix    = "files-v1"
	projectFileSearchMaxFileBytes       = 1024 * 1024
	projectFileSearchMaxContentBytes    = 256 * 1024
	projectFileSearchStoredSnippetRunes = 1200
)

type projectFileSearchIndexState struct {
	sync.Mutex
	root      string
	indexPath string
	builtAt   time.Time
	truncated bool
}

var projectFileSearchIndexes = struct {
	sync.Mutex
	items map[string]*projectFileSearchIndexState
}{items: map[string]*projectFileSearchIndexState{}}

type projectFileSearchIndexStats struct {
	Projects int   `json:"projects"`
	Built    int   `json:"built"`
	Bytes    int64 `json:"bytes"`
}

func projectFileSearchIndexStatsSnapshot() projectFileSearchIndexStats {
	projectFileSearchIndexes.Lock()
	indexes := make([]*projectFileSearchIndexState, 0, len(projectFileSearchIndexes.items))
	for _, item := range projectFileSearchIndexes.items {
		indexes = append(indexes, item)
	}
	projectFileSearchIndexes.Unlock()

	stats := projectFileSearchIndexStats{Projects: len(indexes)}
	for _, item := range indexes {
		item.Lock()
		built := !item.builtAt.IsZero()
		indexPath := item.indexPath
		item.Unlock()
		if built {
			stats.Built++
		}
		stats.Bytes += directorySize(indexPath)
	}
	return stats
}

func searchProjectFileEntriesIndexed(ctx context.Context, root string, query string, limit int) ([]projectFileEntry, bool, error) {
	rootEval, ok := safePreviewRoot(root)
	if !ok {
		return nil, false, errors.New("project root is not readable")
	}
	indexState := projectFileSearchIndexForRoot(rootEval)
	if err := indexState.ensure(ctx); err != nil {
		return nil, false, err
	}

	reader, err := bluge.OpenReader(bluge.DefaultConfig(indexState.indexPath))
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()

	queryRequest := bluge.NewTopNSearch(limit+1, bluge.NewMatchQuery(query).SetField("search"))
	iterator, err := reader.Search(ctx, queryRequest)
	if err != nil {
		return nil, false, err
	}

	entries := make([]projectFileEntry, 0, limit)
	truncated := indexState.truncated
	for {
		match, err := iterator.Next()
		if err != nil {
			return nil, false, err
		}
		if match == nil {
			break
		}
		relativePath := ""
		contentPreview := ""
		if err := match.VisitStoredFields(func(field string, value []byte) bool {
			if field == "path" {
				relativePath = string(value)
			}
			if field == "content_preview" {
				contentPreview = string(value)
			}
			return true
		}); err != nil {
			return nil, false, err
		}
		if relativePath == "" {
			continue
		}
		if len(entries) >= limit {
			truncated = true
			break
		}
		entry, ok := projectFileEntryFromPath(rootEval, relativePath)
		if !ok {
			continue
		}
		if contentPreview != "" && strings.Contains(strings.ToLower(contentPreview), strings.ToLower(query)) {
			entry.Snippet = serverSearchSnippet(contentPreview, strings.ToLower(query), searchMaxSnippetRunes)
		}
		entries = append(entries, entry)
	}
	sortProjectFileEntries(entries)
	return entries, truncated, nil
}

func projectFileSearchIndexForRoot(root string) *projectFileSearchIndexState {
	projectFileSearchIndexes.Lock()
	defer projectFileSearchIndexes.Unlock()

	if existing := projectFileSearchIndexes.items[root]; existing != nil {
		return existing
	}
	state := &projectFileSearchIndexState{
		root:      root,
		indexPath: filepath.Join(workspaceDataDir(), "search", projectFileSearchIndexPathPrefix, shortProjectFileSearchHash(root)),
	}
	projectFileSearchIndexes.items[root] = state
	return state
}

func invalidateProjectFileSearchIndex(root string) {
	rootEval, ok := safePreviewRoot(root)
	if !ok {
		return
	}
	indexState := projectFileSearchIndexForRoot(rootEval)
	indexState.Lock()
	indexState.builtAt = time.Time{}
	indexState.Unlock()
}

func (state *projectFileSearchIndexState) ensure(ctx context.Context) error {
	state.Lock()
	defer state.Unlock()

	if !state.builtAt.IsZero() && time.Since(state.builtAt) < projectFileSearchIndexTTL {
		if _, err := os.Stat(state.indexPath); err == nil {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return state.rebuild(ctx)
}

func (state *projectFileSearchIndexState) rebuild(ctx context.Context) error {
	if err := os.RemoveAll(state.indexPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(state.indexPath), 0o755); err != nil {
		return err
	}
	writer, err := bluge.OpenWriter(bluge.DefaultConfig(state.indexPath))
	if err != nil {
		return err
	}
	defer writer.Close()

	batch := bluge.NewBatch()
	pending := 0
	visited := 0
	truncated := false
	stopWalk := errors.New("project file index stopped")
	flush := func() error {
		if pending == 0 {
			return nil
		}
		if err := writer.Batch(batch); err != nil {
			return err
		}
		batch.Reset()
		pending = 0
		return nil
	}

	err = filepath.WalkDir(state.root, func(path string, dirEntry os.DirEntry, walkErr error) error {
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
		relativePath, err := filepath.Rel(state.root, path)
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

		doc := projectFileSearchDocument(state.root, path, relativePath, dirEntry)
		batch.Update(doc.ID(), doc)
		pending++
		if pending >= projectFileSearchIndexBatchSize {
			return flush()
		}
		return nil
	})
	if err != nil && !errors.Is(err, stopWalk) {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	state.builtAt = time.Now()
	state.truncated = truncated
	return nil
}

func projectFileSearchText(relativePath string, name string) string {
	relativePath = strings.TrimSpace(filepath.ToSlash(relativePath))
	name = strings.TrimSpace(name)
	normalized := strings.NewReplacer("/", " ", "_", " ", "-", " ", ".", " ").Replace(relativePath)
	if name == "" {
		return relativePath + "\n" + normalized
	}
	return relativePath + "\n" + name + "\n" + normalized
}

func projectFileSearchDocument(root string, absolutePath string, relativePath string, dirEntry os.DirEntry) *bluge.Document {
	body := projectFileSearchText(relativePath, dirEntry.Name())
	content := projectFileSearchContent(root, absolutePath, relativePath, dirEntry)
	if content != "" {
		body += "\n" + content
	}
	doc := bluge.NewDocument(projectFileSearchDocID(relativePath)).
		AddField(bluge.NewTextField("search", body)).
		AddField(blugeStoredField("path", relativePath))
	if content != "" {
		doc.AddField(blugeStoredField("content_preview", trimProjectFileSearchSnippet(content)))
	}
	return doc
}

func projectFileSearchContent(root string, absolutePath string, relativePath string, dirEntry os.DirEntry) string {
	if dirEntry.IsDir() || !projectFileSearchShouldIndexContent(relativePath, dirEntry) {
		return ""
	}
	if _, err := safeProjectFilePath(root, relativePath); err != nil {
		return ""
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		return ""
	}
	defer file.Close()
	buffer := make([]byte, projectFileSearchMaxContentBytes)
	n, err := file.Read(buffer)
	if err != nil && n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buffer[:n]))
}

func projectFileSearchShouldIndexContent(relativePath string, dirEntry os.DirEntry) bool {
	info, err := dirEntry.Info()
	if err != nil || !info.Mode().IsRegular() || info.Size() > projectFileSearchMaxFileBytes {
		return false
	}
	ext := strings.ToLower(filepath.Ext(relativePath))
	if isLikelyTextExt(ext) {
		return true
	}
	switch ext {
	case ".md", ".markdown", ".json", ".csv", ".tsv", ".log":
		return true
	default:
		return false
	}
}

func trimProjectFileSearchSnippet(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= projectFileSearchStoredSnippetRunes {
		return string(runes)
	}
	return string(runes[:projectFileSearchStoredSnippetRunes])
}

func projectFileSearchDocID(relativePath string) string {
	return filepath.ToSlash(strings.TrimSpace(relativePath))
}

func shortProjectFileSearchHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:16]
}
