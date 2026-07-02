package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-agent-manager/internal/parser"
	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/legacyimport"
	"ai-agent-manager/internal/workspace/readmodels"
	"ai-agent-manager/internal/workspace/transcript"

	"github.com/blugelabs/bluge"
)

const (
	sessionSearchIndexBatchSize    = 500
	sessionSearchIndexStoredRunes  = 2000
	sessionSearchIndexPathPrefix   = "sessions-v1"
	sessionSearchIndexMaxTopDocs   = 5000
	sessionSearchIndexMinTopDocs   = 500
	sessionSearchIndexDocsPerGroup = searchPerSessionLimit * 8
)

type sessionSearchIndexState struct {
	sync.Mutex
	indexPath       string
	signature       string
	indexedSessions int
	indexedDocs     int
}

var sessionSearchIndex = &sessionSearchIndexState{
	indexPath: filepath.Join(workspaceDataDir(), "search", sessionSearchIndexPathPrefix),
}

type sessionSearchIndexStats struct {
	Ready           bool   `json:"ready"`
	IndexedSessions int    `json:"indexed_sessions"`
	IndexedDocs     int    `json:"indexed_docs"`
	Bytes           int64  `json:"bytes"`
	Signature       string `json:"signature,omitempty"`
}

func sessionSearchIndexStatsSnapshot() sessionSearchIndexStats {
	sessionSearchIndex.Lock()
	stats := sessionSearchIndexStats{
		Ready:           sessionSearchIndex.signature != "",
		IndexedSessions: sessionSearchIndex.indexedSessions,
		IndexedDocs:     sessionSearchIndex.indexedDocs,
		Signature:       sessionSearchIndex.signature,
	}
	indexPath := sessionSearchIndex.indexPath
	sessionSearchIndex.Unlock()
	stats.Bytes = directorySize(indexPath)
	return stats
}

func searchSessionsWithIndex(ctx context.Context, appState *state.AppState, query string, offset int, limit int) ([]sessionSearchResult, int, int, int, error) {
	if appState == nil {
		return nil, 0, 0, 0, errors.New("state is required")
	}
	if err := sessionSearchIndex.ensure(ctx, appState); err != nil {
		return nil, 0, 0, 0, err
	}

	reader, err := bluge.OpenReader(bluge.DefaultConfig(sessionSearchIndex.indexPath))
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer reader.Close()

	topN := sessionSearchTopN(offset, limit)
	iterator, err := reader.Search(ctx, bluge.NewTopNSearch(topN, bluge.NewMatchQuery(query).SetField("body")))
	if err != nil {
		return nil, 0, 0, 0, err
	}

	queryLower := strings.ToLower(query)
	grouped := map[string]*sessionSearchResult{}
	order := []string{}
	for {
		match, err := iterator.Next()
		if err != nil {
			return nil, 0, 0, 0, err
		}
		if match == nil {
			break
		}
		fields := map[string]string{}
		if err := match.VisitStoredFields(func(field string, value []byte) bool {
			fields[field] = string(value)
			return true
		}); err != nil {
			return nil, 0, 0, 0, err
		}
		key := strings.TrimSpace(fields["key"])
		if key == "" {
			continue
		}
		item := grouped[key]
		if item == nil {
			next := sessionSearchResultFromFields(fields)
			grouped[key] = &next
			order = append(order, key)
			item = &next
		}
		if len(item.SearchMatches) >= searchPerSessionLimit {
			continue
		}
		matchIndex, _ := strconv.Atoi(fields["message_index"])
		createdAt := parseSearchIndexTimePtr(fields["message_created_at"])
		item.SearchMatches = append(item.SearchMatches, parser.SearchMatch{
			MessageID: fields["message_id"],
			Role:      fields["message_role"],
			Index:     matchIndex,
			Snippet:   serverSearchSnippet(fields["message_text"], queryLower, searchMaxSnippetRunes),
			CreatedAt: createdAt,
		})
		item.SearchMatchCount = len(item.SearchMatches)
	}

	items := make([]sessionSearchResult, 0, len(grouped))
	for _, key := range order {
		if item := grouped[key]; item != nil && len(item.SearchMatches) > 0 {
			items = append(items, *item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	total := len(items)
	nextOffset := 0
	if offset < len(items) {
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		items = items[offset:end]
		if end < total {
			nextOffset = end
		}
	} else {
		items = []sessionSearchResult{}
	}
	return items, nextOffset, total, sessionSearchIndex.indexedSessions, nil
}

func (index *sessionSearchIndexState) ensure(ctx context.Context, appState *state.AppState) error {
	signature := sessionSearchSignature(appState)
	index.Lock()
	defer index.Unlock()

	if index.indexPath != filepath.Join(workspaceDataDir(), "search", sessionSearchIndexPathPrefix) {
		index.indexPath = filepath.Join(workspaceDataDir(), "search", sessionSearchIndexPathPrefix)
		index.signature = ""
	}
	if signature != "" && signature == index.signature {
		if _, err := os.Stat(index.indexPath); err == nil {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return index.rebuild(ctx, appState, signature)
}

func (index *sessionSearchIndexState) rebuild(ctx context.Context, appState *state.AppState, signature string) error {
	if err := os.RemoveAll(index.indexPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(index.indexPath), 0o755); err != nil {
		return err
	}
	writer, err := bluge.OpenWriter(bluge.DefaultConfig(index.indexPath))
	if err != nil {
		return err
	}
	defer writer.Close()

	batch := bluge.NewBatch()
	pending := 0
	indexedDocs := 0
	indexedSessionKeys := map[string]struct{}{}
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
	addDoc := func(doc *bluge.Document, key string) error {
		batch.Update(doc.ID(), doc)
		pending++
		indexedDocs++
		indexedSessionKeys[key] = struct{}{}
		if pending >= sessionSearchIndexBatchSize {
			return flush()
		}
		return nil
	}

	storedWorkspaceChats := workspaceStoredChatSet()
	legacyMetas := make([]state.SessionMeta, 0, len(appState.Sessions))
	for _, meta := range appState.Sessions {
		legacyMetas = append(legacyMetas, meta)
	}
	sort.SliceStable(legacyMetas, func(i, j int) bool {
		return legacyMetas[i].UpdatedAt.After(legacyMetas[j].UpdatedAt)
	})
	for _, meta := range legacyMetas {
		if err := ctx.Err(); err != nil {
			return err
		}
		if meta.MetadataOnly || strings.TrimSpace(meta.TranscriptPath) == "" {
			continue
		}
		if _, ok := storedWorkspaceChats[legacyimport.ImportedChatID(meta)]; ok {
			continue
		}
		meta = enrichSessionMeta(meta)
		var addErr error
		streamErr := parser.StreamSearchableMessages(meta.Agent, meta.SessionID, meta.TranscriptPath, func(message parser.SearchableMessage) bool {
			if strings.TrimSpace(message.Text) == "" {
				return true
			}
			if addErr = addDoc(newLegacySessionSearchDocument(meta, message), meta.Key); addErr != nil {
				return false
			}
			return true
		})
		if addErr != nil {
			return addErr
		}
		if streamErr != nil {
			if errors.Is(streamErr, parser.ErrTranscriptUnavailable) {
				continue
			}
			return streamErr
		}
	}

	if err := indexWorkspaceSessions(ctx, addDoc); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	index.signature = signature
	index.indexedDocs = indexedDocs
	index.indexedSessions = len(indexedSessionKeys)
	return nil
}

func indexWorkspaceSessions(ctx context.Context, addDoc func(*bluge.Document, string) error) error {
	store := workspaceStore()
	storeState, err := store.LoadStateLight()
	if err != nil {
		return nil
	}
	for _, chat := range storeState.ChatsByID {
		if err := ctx.Err(); err != nil {
			return err
		}
		if chat.DeletedAt != 0 {
			continue
		}
		project, ok := storeState.ProjectsByID[chat.ProjectID]
		if !ok || project.DeletedAt != 0 {
			continue
		}
		if meta, ok := workspaceNativeTranscriptMetaForChatRecord(chat, project); ok {
			var addErr error
			index := 0
			streamErr := parser.StreamSearchableMessages(meta.Agent, meta.SessionID, meta.TranscriptPath, func(message parser.SearchableMessage) bool {
				if strings.TrimSpace(message.Text) == "" {
					return true
				}
				index++
				if addErr = addDoc(newWorkspaceNativeSessionSearchDocument(chat, project, meta, message, index), "workspace:"+chat.ID); addErr != nil {
					return false
				}
				return true
			})
			if addErr != nil {
				return addErr
			}
			if streamErr != nil && !errors.Is(streamErr, parser.ErrTranscriptUnavailable) {
				return streamErr
			}
			continue
		}
		if workspaceChatHasTmuxRuntime(chat) {
			continue
		}
		entries, err := store.ReplayTranscriptEntriesForChat(chat.ID, 0)
		if err != nil {
			continue
		}
		toolCallIDByToolID := map[string]string{}
		for index, entry := range entries {
			if transcript.Kind(entry) == transcript.KindToolCall {
				if toolID := workspaceEntryToolID(entry); toolID != "" {
					toolCallIDByToolID[chat.ID+"\x00"+toolID] = workspaceEntryString(entry, "_id")
				}
			}
			kind, role, text := workspaceEntrySearchText(entry)
			if strings.TrimSpace(text) == "" {
				continue
			}
			messageID := workspaceIndexedMessageID(chat.ID, entry, toolCallIDByToolID)
			doc := newWorkspaceSessionSearchDocument(chat, project, entry, kind, role, text, messageID, index+1)
			if err := addDoc(doc, "workspace:"+chat.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func newLegacySessionSearchDocument(meta state.SessionMeta, message parser.SearchableMessage) *bluge.Document {
	key := meta.Key
	chatID := legacyimport.ImportedChatID(meta)
	docID := "legacy:" + shortSessionSearchHash(key+"\x00"+message.ID+"\x00"+strconv.Itoa(message.Index))
	return bluge.NewDocument(docID).
		AddField(bluge.NewTextField("body", message.Text)).
		AddField(blugeStoredField("key", key)).
		AddField(blugeStoredField("chat_id", chatID)).
		AddField(blugeStoredField("agent", meta.Agent)).
		AddField(blugeStoredField("session_id", meta.SessionID)).
		AddField(blugeStoredField("session_name", state.ResolveSessionName(meta))).
		AddField(blugeStoredField("transcript_path", meta.TranscriptPath)).
		AddField(blugeStoredField("cwd", meta.Cwd)).
		AddField(blugeStoredField("project_name", meta.ProjectName)).
		AddField(blugeStoredField("model", meta.Model)).
		AddField(blugeStoredField("updated_at", formatSearchIndexTime(meta.UpdatedAt))).
		AddField(blugeStoredField("first_preview", meta.FirstPreview)).
		AddField(blugeStoredField("last_preview", meta.LastPreview)).
		AddField(blugeStoredField("message_count", strconv.Itoa(meta.MessageCountEstimate))).
		AddField(blugeStoredField("metadata_only", strconv.FormatBool(meta.MetadataOnly))).
		AddField(blugeStoredField("invalid_reason", meta.InvalidReason)).
		AddField(blugeStoredField("message_id", message.ID)).
		AddField(blugeStoredField("message_role", message.Role)).
		AddField(blugeStoredField("message_index", strconv.Itoa(message.Index))).
		AddField(blugeStoredField("message_text", trimSearchIndexStoredText(message.Text))).
		AddField(blugeStoredField("message_created_at", formatSearchIndexTimePtr(message.CreatedAt)))
}

func newWorkspaceSessionSearchDocument(chat readmodels.ChatRecord, project readmodels.ProjectRecord, entry readmodels.TranscriptEntry, kind string, role string, text string, messageID string, index int) *bluge.Document {
	agent := ""
	if chat.Provider != nil {
		agent = *chat.Provider
	}
	updatedAt := time.UnixMilli(max(chat.UpdatedAt, chat.LastMessageAt, chat.CreatedAt))
	createdAt := workspaceEntryCreatedAt(entry)
	key := "workspace:" + chat.ID
	docID := "workspace:" + shortSessionSearchHash(chat.ID+"\x00"+messageID+"\x00"+strconv.Itoa(index))
	return bluge.NewDocument(docID).
		AddField(bluge.NewTextField("body", text)).
		AddField(blugeStoredField("key", key)).
		AddField(blugeStoredField("chat_id", chat.ID)).
		AddField(blugeStoredField("agent", agent)).
		AddField(blugeStoredField("session_id", chat.ID)).
		AddField(blugeStoredField("session_name", chat.Title)).
		AddField(blugeStoredField("transcript_path", "")).
		AddField(blugeStoredField("cwd", project.LocalPath)).
		AddField(blugeStoredField("project_name", project.Title)).
		AddField(blugeStoredField("model", "")).
		AddField(blugeStoredField("updated_at", formatSearchIndexTime(updatedAt))).
		AddField(blugeStoredField("first_preview", "")).
		AddField(blugeStoredField("last_preview", trimSearchIndexStoredText(text))).
		AddField(blugeStoredField("message_count", "0")).
		AddField(blugeStoredField("metadata_only", "false")).
		AddField(blugeStoredField("invalid_reason", "")).
		AddField(blugeStoredField("message_id", messageID)).
		AddField(blugeStoredField("message_role", role)).
		AddField(blugeStoredField("message_index", strconv.Itoa(index))).
		AddField(blugeStoredField("message_kind", kind)).
		AddField(blugeStoredField("message_text", trimSearchIndexStoredText(text))).
		AddField(blugeStoredField("message_created_at", formatSearchIndexTimePtr(createdAt)))
}

func newWorkspaceNativeSessionSearchDocument(chat readmodels.ChatRecord, project readmodels.ProjectRecord, meta state.SessionMeta, message parser.SearchableMessage, index int) *bluge.Document {
	updatedAt := time.UnixMilli(max(chat.UpdatedAt, chat.LastMessageAt, chat.CreatedAt))
	key := "workspace:" + chat.ID
	messageID := workspaceSearchableCursor(message)
	docID := "workspace-native:" + shortSessionSearchHash(chat.ID+"\x00"+messageID+"\x00"+strconv.Itoa(index))
	return bluge.NewDocument(docID).
		AddField(bluge.NewTextField("body", message.Text)).
		AddField(blugeStoredField("key", key)).
		AddField(blugeStoredField("chat_id", chat.ID)).
		AddField(blugeStoredField("agent", meta.Agent)).
		AddField(blugeStoredField("session_id", meta.SessionID)).
		AddField(blugeStoredField("session_name", chat.Title)).
		AddField(blugeStoredField("transcript_path", meta.TranscriptPath)).
		AddField(blugeStoredField("cwd", project.LocalPath)).
		AddField(blugeStoredField("project_name", project.Title)).
		AddField(blugeStoredField("model", "")).
		AddField(blugeStoredField("updated_at", formatSearchIndexTime(updatedAt))).
		AddField(blugeStoredField("first_preview", "")).
		AddField(blugeStoredField("last_preview", trimSearchIndexStoredText(message.Text))).
		AddField(blugeStoredField("message_count", "0")).
		AddField(blugeStoredField("metadata_only", "false")).
		AddField(blugeStoredField("invalid_reason", "")).
		AddField(blugeStoredField("message_id", messageID)).
		AddField(blugeStoredField("message_role", message.Role)).
		AddField(blugeStoredField("message_index", strconv.Itoa(message.Index))).
		AddField(blugeStoredField("message_kind", message.Kind)).
		AddField(blugeStoredField("message_text", trimSearchIndexStoredText(message.Text))).
		AddField(blugeStoredField("message_created_at", formatSearchIndexTimePtr(message.CreatedAt)))
}

func sessionSearchResultFromFields(fields map[string]string) sessionSearchResult {
	messageCount, _ := strconv.Atoi(fields["message_count"])
	metadataOnly, _ := strconv.ParseBool(fields["metadata_only"])
	updatedAt := parseSearchIndexTime(fields["updated_at"])
	return sessionSearchResult{
		Key:                  fields["key"],
		ChatID:               fields["chat_id"],
		Agent:                fields["agent"],
		SessionID:            fields["session_id"],
		SessionName:          fields["session_name"],
		TranscriptPath:       fields["transcript_path"],
		Cwd:                  fields["cwd"],
		ProjectName:          fields["project_name"],
		Model:                fields["model"],
		UpdatedAt:            updatedAt,
		FirstPreview:         fields["first_preview"],
		LastPreview:          fields["last_preview"],
		MessageCountEstimate: messageCount,
		MetadataOnly:         metadataOnly,
		InvalidReason:        fields["invalid_reason"],
		SearchMatches:        []parser.SearchMatch{},
	}
}

func workspaceIndexedMessageID(chatID string, entry readmodels.TranscriptEntry, toolCallIDByToolID map[string]string) string {
	if transcript.Kind(entry) == transcript.KindToolResult {
		if toolID := workspaceEntryToolID(entry); toolID != "" {
			if callID := toolCallIDByToolID[chatID+"\x00"+toolID]; callID != "" {
				return callID
			}
		}
	}
	return firstNonEmptyString(
		workspaceEntryString(entry, "messageId"),
		workspaceEntryString(entry, "id"),
		workspaceEntryString(entry, "_id"),
	)
}

func blugeStoredField(name string, value string) *bluge.TermField {
	field := bluge.NewKeywordField(name, value)
	field.FieldOptions = bluge.Store
	return field
}

func sessionSearchSignature(appState *state.AppState) string {
	parts := make([]string, 0, len(appState.Sessions)+8)
	for _, meta := range appState.Sessions {
		parts = append(parts, strings.Join([]string{
			meta.Key,
			meta.Agent,
			meta.SessionID,
			meta.TranscriptPath,
			meta.Cwd,
			meta.ProjectName,
			meta.Model,
			meta.UpdatedAt.Format(time.RFC3339Nano),
			strconv.FormatBool(meta.MetadataOnly),
		}, "\x00"))
	}
	sort.Strings(parts)
	if storeState, err := workspaceStore().LoadStateLight(); err == nil {
		maxUpdated := int64(0)
		for _, chat := range storeState.ChatsByID {
			maxUpdated = max(maxUpdated, chat.UpdatedAt, chat.LastMessageAt, chat.CreatedAt, chat.DeletedAt)
			if path := strings.TrimSpace(chat.NativeTranscriptPath); path != "" {
				if info, err := os.Stat(path); err == nil {
					parts = append(parts, strings.Join([]string{
						"native",
						chat.ID,
						path,
						strconv.FormatInt(info.Size(), 10),
						info.ModTime().UTC().Format(time.RFC3339Nano),
					}, "\x00"))
				} else {
					parts = append(parts, strings.Join([]string{"native", chat.ID, path, "missing"}, "\x00"))
				}
			}
		}
		for _, project := range storeState.ProjectsByID {
			maxUpdated = max(maxUpdated, project.UpdatedAt, project.CreatedAt, project.DeletedAt)
		}
		parts = append(parts, fmt.Sprintf("workspace:%d:%d:%d", len(storeState.ChatsByID), len(storeState.ProjectsByID), maxUpdated))
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func sessionSearchTopN(offset int, limit int) int {
	topN := (offset + limit) * sessionSearchIndexDocsPerGroup
	if topN < sessionSearchIndexMinTopDocs {
		return sessionSearchIndexMinTopDocs
	}
	if topN > sessionSearchIndexMaxTopDocs {
		return sessionSearchIndexMaxTopDocs
	}
	return topN
}

func trimSearchIndexStoredText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= sessionSearchIndexStoredRunes {
		return string(runes)
	}
	return string(runes[:sessionSearchIndexStoredRunes])
}

func formatSearchIndexTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatSearchIndexTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatSearchIndexTime(*value)
}

func parseSearchIndexTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func parseSearchIndexTimePtr(value string) *time.Time {
	parsed := parseSearchIndexTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func shortSessionSearchHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:20]
}
