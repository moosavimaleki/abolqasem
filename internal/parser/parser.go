package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/render"
	"abolqasem/internal/state"
)

var ErrTranscriptUnavailable = errors.New("transcript is unavailable")

var codexInternalMemoryCitationPattern = regexp.MustCompile(`(?is)(?:\r?\n)*<oai-mem-citation>\s*.*?</oai-mem-citation>\s*$`)

type Message struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Role      string     `json:"role"`
	Kind      string     `json:"kind"`
	Text      string     `json:"text"`
	HTML      string     `json:"html"`
	Direction string     `json:"direction"`
	Index     int        `json:"index"`
	CreatedAt *time.Time `json:"created_at"`
}

type SearchableMessage struct {
	ID        string
	SessionID string
	Role      string
	Kind      string
	Text      string
	Index     int
	CreatedAt *time.Time
	Source    string
	Fields    map[string]any
}

type ParseResult struct {
	SessionID     string    `json:"session_id"`
	Items         []Message `json:"items"`
	HasMoreBefore bool      `json:"has_more_before"`
	HasMoreAfter  bool      `json:"has_more_after"`
	OldestCursor  string    `json:"oldest_cursor"`
	NewestCursor  string    `json:"newest_cursor"`
	Status        string    `json:"status,omitempty"`
}

type ParseOptions struct {
	Limit  int
	Before string
	After  string
}

type SessionSummary struct {
	FirstPreview         string
	LastPreview          string
	MessageCountEstimate int
}

type SearchOptions struct {
	Query        string
	Limit        int
	SnippetRunes int
}

type SearchResult struct {
	SessionID string        `json:"session_id"`
	Matches   []SearchMatch `json:"matches"`
}

type SearchMatch struct {
	MessageID string     `json:"message_id"`
	Role      string     `json:"role"`
	Index     int        `json:"index"`
	Snippet   string     `json:"snippet"`
	CreatedAt *time.Time `json:"created_at"`
}

type CacheStats struct {
	FullEntries       int `json:"full_entries"`
	SummaryEntries    int `json:"summary_entries"`
	EstimatedBytes    int `json:"estimated_bytes"`
	MaxBytes          int `json:"max_bytes"`
	MaxEntries        int `json:"max_entries"`
	SummaryMaxEntries int `json:"summary_max_entries"`
}

type cacheEntry struct {
	mtime          time.Time
	size           int64
	messages       []Message
	searchTexts    []string
	lastAccess     time.Time
	estimatedBytes int
}

type summaryCacheEntry struct {
	mtime      time.Time
	size       int64
	summary    SessionSummary
	lastAccess time.Time
}

var (
	cacheMu      sync.Mutex
	cache        = map[string]cacheEntry{}
	summaryCache = map[string]summaryCacheEntry{}
	cacheBytes   int
)

const (
	maxStructuredJSONBytes = 64 * 1024 * 1024

	parserCacheTTL      = 10 * time.Minute
	parserCacheMaxItems = 16
	parserCacheMaxBytes = 128 * 1024 * 1024

	summaryCacheTTL      = 30 * time.Minute
	summaryCacheMaxItems = 512
)

func Stats() CacheStats {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return CacheStats{
		FullEntries:       len(cache),
		SummaryEntries:    len(summaryCache),
		EstimatedBytes:    cacheBytes,
		MaxBytes:          parserCacheMaxBytes,
		MaxEntries:        parserCacheMaxItems,
		SummaryMaxEntries: summaryCacheMaxItems,
	}
}

// ClearCache releases only derived in-memory parser data. Session transcripts
// remain untouched and will be parsed again on demand.
func ClearCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = map[string]cacheEntry{}
	summaryCache = map[string]summaryCacheEntry{}
	cacheBytes = 0
}

func ParseMessages(agent, sessionID, transcriptPath string, opts ParseOptions) (*ParseResult, error) {
	if strings.TrimSpace(transcriptPath) == "" {
		return &ParseResult{
			SessionID: sessionID,
			Items:     []Message{},
			Status:    "metadata_only",
		}, ErrTranscriptUnavailable
	}
	if opts.Limit <= 0 {
		opts.Limit = 30
	}

	messages, err := loadMessages(agent, sessionID, transcriptPath)
	if err != nil {
		return nil, err
	}

	result := &ParseResult{
		SessionID: sessionID,
		Items:     []Message{},
	}
	if len(messages) == 0 {
		return result, nil
	}

	start, end := sliceRange(messages, opts)
	if start < 0 {
		start = 0
	}
	if end > len(messages) {
		end = len(messages)
	}
	if start > end {
		start = end
	}

	result.Items = append(result.Items, messages[start:end]...)
	result.HasMoreBefore = start > 0
	result.HasMoreAfter = end < len(messages)
	if len(result.Items) > 0 {
		result.OldestCursor = strconv.Itoa(result.Items[0].Index)
		result.NewestCursor = strconv.Itoa(result.Items[len(result.Items)-1].Index)
	}
	return result, nil
}

func GetSessionSummary(agent, sessionID, transcriptPath string) (SessionSummary, error) {
	if strings.TrimSpace(transcriptPath) == "" {
		return SessionSummary{}, ErrTranscriptUnavailable
	}
	info, err := os.Stat(transcriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionSummary{}, ErrTranscriptUnavailable
		}
		return SessionSummary{}, err
	}
	cacheKey := agent + "|" + filepath.Clean(transcriptPath)
	if summary, ok := getParserSummaryCacheEntry(cacheKey, info); ok {
		return summary, nil
	}
	summary := SessionSummary{}
	firstAnyPreview := ""
	lastPreview := ""
	err = StreamSearchableMessages(agent, sessionID, transcriptPath, func(message SearchableMessage) bool {
		summary.MessageCountEstimate++
		if strings.TrimSpace(message.Text) == "" {
			return true
		}
		isBootstrap := state.IsAgentBootstrapPrompt(message.Text)
		if firstAnyPreview == "" && !isBootstrap {
			firstAnyPreview = inlinePreview(message.Text)
		}
		if strings.EqualFold(strings.TrimSpace(message.Role), "user") && !isBootstrap {
			if summary.FirstPreview == "" {
				summary.FirstPreview = inlinePreview(message.Text)
			}
		}
		lastPreview = message.Text
		return true
	})
	if err != nil {
		return SessionSummary{}, err
	}
	if summary.FirstPreview == "" {
		summary.FirstPreview = firstAnyPreview
	}
	summary.LastPreview = trimPreview(lastPreview)
	setParserSummaryCacheEntry(cacheKey, info, summary)
	return summary, nil
}

func SearchMessages(agent, sessionID, transcriptPath string, opts SearchOptions) (SearchResult, error) {
	result := SearchResult{
		SessionID: sessionID,
		Matches:   []SearchMatch{},
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return result, nil
	}
	if strings.TrimSpace(transcriptPath) == "" {
		return result, ErrTranscriptUnavailable
	}
	if opts.Limit <= 0 {
		opts.Limit = 3
	}
	if opts.SnippetRunes <= 0 {
		opts.SnippetRunes = 180
	}

	queryLower := strings.ToLower(query)
	err := StreamSearchableMessages(agent, sessionID, transcriptPath, func(message SearchableMessage) bool {
		if !strings.Contains(strings.ToLower(message.Text), queryLower) {
			return true
		}
		result.Matches = append(result.Matches, SearchMatch{
			MessageID: message.ID,
			Role:      message.Role,
			Index:     message.Index,
			Snippet:   searchSnippet(message.Text, queryLower, opts.SnippetRunes),
			CreatedAt: message.CreatedAt,
		})
		return len(result.Matches) < opts.Limit
	})
	if err != nil {
		if errors.Is(err, ErrTranscriptUnavailable) {
			return result, ErrTranscriptUnavailable
		}
		return result, err
	}
	return result, nil
}

func StreamSearchableMessages(agent, sessionID, transcriptPath string, visit func(SearchableMessage) bool) error {
	if strings.TrimSpace(transcriptPath) == "" {
		return ErrTranscriptUnavailable
	}
	info, err := os.Stat(transcriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrTranscriptUnavailable
		}
		return err
	}

	file, err := os.Open(transcriptPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	lineIndex := 0
	emitted := 0
	var previous *SearchableMessage
	codexCommandItems := make(map[string]struct{})
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineIndex++
			raw := map[string]any{}
			if decodeErr := json.Unmarshal(bytesTrimSpace(line), &raw); decodeErr == nil {
				if msg := extractSearchableMessage(agent, raw, sessionID, lineIndex); msg != nil {
					if !normalizeCodexCommandMessage(agent, msg, codexCommandItems) {
						continue
					}
					if shouldDropSearchableDuplicate(agent, previous, msg) {
						continue
					}
					accepted := *msg
					previous = &accepted
					emitted++
					if !visit(accepted) {
						return nil
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
	}
	if emitted == 0 && shouldTryStructuredJSON(transcriptPath, info.Size()) {
		return streamStructuredJSONMessages(agent, sessionID, transcriptPath, visit)
	}
	return nil
}

func searchSnippet(text, queryLower string, maxRunes int) string {
	textRunes := []rune(strings.TrimSpace(text))
	if len(textRunes) <= maxRunes {
		return string(textRunes)
	}

	index := indexRunes([]rune(strings.ToLower(string(textRunes))), []rune(queryLower))
	if index < 0 {
		index = 0
	}
	start := index - (maxRunes / 3)
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(textRunes) {
		end = len(textRunes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}

	prefix := ""
	if start > 0 {
		prefix = "..."
	}
	suffix := ""
	if end < len(textRunes) {
		suffix = "..."
	}
	return prefix + string(textRunes[start:end]) + suffix
}

func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		matched := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func loadMessages(agent, sessionID, transcriptPath string) ([]Message, error) {
	info, err := os.Stat(transcriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTranscriptUnavailable
		}
		return nil, err
	}

	cacheKey := agent + "|" + filepath.Clean(transcriptPath)
	if entry, ok := getParserCacheEntry(cacheKey, info); ok {
		return entry.messages, nil
	}

	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	messages := make([]Message, 0, 64)
	lineIndex := 0
	var previous *SearchableMessage
	codexCommandItems := make(map[string]struct{})
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineIndex++
			raw := map[string]any{}
			if decodeErr := json.Unmarshal(bytesTrimSpace(line), &raw); decodeErr == nil {
				if msg := extractSearchableMessage(agent, raw, sessionID, lineIndex); msg != nil {
					if !normalizeCodexCommandMessage(agent, msg, codexCommandItems) {
						continue
					}
					if shouldDropSearchableDuplicate(agent, previous, msg) {
						continue
					}
					accepted := *msg
					previous = &accepted
					messages = append(messages, *searchableToMessage(accepted))
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	if len(messages) == 0 && shouldTryStructuredJSON(transcriptPath, info.Size()) {
		if structured, err := loadStructuredJSONMessages(agent, sessionID, transcriptPath); err == nil {
			messages = structured
		}
	}

	setParserCacheEntry(cacheKey, cacheEntry{
		mtime:    info.ModTime(),
		size:     info.Size(),
		messages: messages,
	})
	return messages, nil
}

func loadSearchableMessages(agent, sessionID, transcriptPath string) ([]Message, []string, error) {
	info, err := os.Stat(transcriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrTranscriptUnavailable
		}
		return nil, nil, err
	}

	cacheKey := agent + "|" + filepath.Clean(transcriptPath)
	if entry, ok := getParserCacheEntry(cacheKey, info); ok {
		if len(entry.searchTexts) != len(entry.messages) {
			entry.searchTexts = buildSearchTexts(entry.messages)
			setParserCacheEntry(cacheKey, entry)
		}
		return entry.messages, entry.searchTexts, nil
	}

	messages, err := loadMessages(agent, sessionID, transcriptPath)
	if err != nil {
		return nil, nil, err
	}
	searchTexts := buildSearchTexts(messages)

	entry, _ := getParserCacheEntry(cacheKey, info)
	entry.searchTexts = searchTexts
	setParserCacheEntry(cacheKey, entry)
	return messages, searchTexts, nil
}

func getParserCacheEntry(cacheKey string, info os.FileInfo) (cacheEntry, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	entry, ok := cache[cacheKey]
	if !ok || entry.size != info.Size() || !entry.mtime.Equal(info.ModTime()) {
		if ok {
			removeParserCacheEntryLocked(cacheKey)
		}
		return cacheEntry{}, false
	}
	now := time.Now()
	if now.Sub(entry.lastAccess) > parserCacheTTL {
		removeParserCacheEntryLocked(cacheKey)
		return cacheEntry{}, false
	}
	entry.lastAccess = now
	cache[cacheKey] = entry
	return entry, true
}

func setParserCacheEntry(cacheKey string, entry cacheEntry) {
	if cacheKey == "" || len(entry.messages) == 0 {
		return
	}
	entry.lastAccess = time.Now()
	entry.estimatedBytes = estimateParserCacheEntryBytes(entry)
	if entry.estimatedBytes > parserCacheMaxBytes/2 {
		return
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if _, ok := cache[cacheKey]; ok {
		removeParserCacheEntryLocked(cacheKey)
	}
	cache[cacheKey] = entry
	cacheBytes += entry.estimatedBytes
	evictParserCacheLocked(time.Now())
}

func removeParserCacheEntryLocked(cacheKey string) {
	if entry, ok := cache[cacheKey]; ok {
		cacheBytes -= entry.estimatedBytes
		if cacheBytes < 0 {
			cacheBytes = 0
		}
		delete(cache, cacheKey)
	}
}

func evictParserCacheLocked(now time.Time) {
	for key, entry := range cache {
		if now.Sub(entry.lastAccess) > parserCacheTTL {
			removeParserCacheEntryLocked(key)
		}
	}
	for len(cache) > parserCacheMaxItems || cacheBytes > parserCacheMaxBytes {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range cache {
			if oldestKey == "" || entry.lastAccess.Before(oldest) {
				oldestKey = key
				oldest = entry.lastAccess
			}
		}
		if oldestKey == "" {
			return
		}
		removeParserCacheEntryLocked(oldestKey)
	}
}

func estimateParserCacheEntryBytes(entry cacheEntry) int {
	total := 256 + len(entry.messages)*256 + len(entry.searchTexts)*64
	for _, message := range entry.messages {
		total += len(message.ID) + len(message.SessionID) + len(message.Role) + len(message.Kind)
		total += len(message.Text) + len(message.HTML) + len(message.Direction)
	}
	for _, text := range entry.searchTexts {
		total += len(text)
	}
	return total
}

func getParserSummaryCacheEntry(cacheKey string, info os.FileInfo) (SessionSummary, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	entry, ok := summaryCache[cacheKey]
	if !ok || entry.size != info.Size() || !entry.mtime.Equal(info.ModTime()) {
		if ok {
			delete(summaryCache, cacheKey)
		}
		return SessionSummary{}, false
	}
	now := time.Now()
	if now.Sub(entry.lastAccess) > summaryCacheTTL {
		delete(summaryCache, cacheKey)
		return SessionSummary{}, false
	}
	entry.lastAccess = now
	summaryCache[cacheKey] = entry
	return entry.summary, true
}

func setParserSummaryCacheEntry(cacheKey string, info os.FileInfo, summary SessionSummary) {
	if cacheKey == "" {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()

	summaryCache[cacheKey] = summaryCacheEntry{
		mtime:      info.ModTime(),
		size:       info.Size(),
		summary:    summary,
		lastAccess: time.Now(),
	}
	evictParserSummaryCacheLocked(time.Now())
}

func evictParserSummaryCacheLocked(now time.Time) {
	for key, entry := range summaryCache {
		if now.Sub(entry.lastAccess) > summaryCacheTTL {
			delete(summaryCache, key)
		}
	}
	for len(summaryCache) > summaryCacheMaxItems {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range summaryCache {
			if oldestKey == "" || entry.lastAccess.Before(oldest) {
				oldestKey = key
				oldest = entry.lastAccess
			}
		}
		if oldestKey == "" {
			return
		}
		delete(summaryCache, oldestKey)
	}
}

func buildSearchTexts(messages []Message) []string {
	searchTexts := make([]string, len(messages))
	for index, message := range messages {
		searchTexts[index] = strings.ToLower(message.Text)
	}
	return searchTexts
}

func shouldTryStructuredJSON(transcriptPath string, size int64) bool {
	if size <= 0 || size > maxStructuredJSONBytes {
		return false
	}
	ext := strings.ToLower(filepath.Ext(transcriptPath))
	return ext == ".json"
}

func loadStructuredJSONMessages(agent, sessionID, transcriptPath string) ([]Message, error) {
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return nil, err
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	messages := make([]Message, 0, 64)
	collectStructuredMessages(agent, sessionID, raw, &messages, 0)
	return messages, nil
}

func streamStructuredJSONMessages(agent, sessionID, transcriptPath string, visit func(SearchableMessage) bool) error {
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return err
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	keepGoing := true
	nextIndex := 0
	collectStructuredSearchableMessages(agent, sessionID, raw, 0, &nextIndex, &keepGoing, visit)
	return nil
}

func collectStructuredMessages(agent, sessionID string, value any, messages *[]Message, depth int) {
	if depth > 8 {
		return
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectStructuredMessages(agent, sessionID, item, messages, depth+1)
		}
	case map[string]any:
		if msg := extractMessage(agent, typed, sessionID, len(*messages)+1); msg != nil {
			*messages = append(*messages, *msg)
			return
		}
		for _, key := range []string{"messages", "history", "turns", "contents", "conversation", "entries", "items", "curatedHistory"} {
			if child, ok := typed[key]; ok {
				collectStructuredMessages(agent, sessionID, child, messages, depth+1)
			}
		}
	}
}

func collectStructuredSearchableMessages(agent, sessionID string, value any, depth int, nextIndex *int, keepGoing *bool, visit func(SearchableMessage) bool) {
	if depth > 8 || !*keepGoing {
		return
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectStructuredSearchableMessages(agent, sessionID, item, depth+1, nextIndex, keepGoing, visit)
			if !*keepGoing {
				return
			}
		}
	case map[string]any:
		if msg := extractSearchableMessage(agent, typed, sessionID, 0); msg != nil {
			(*nextIndex)++
			msg.Index = *nextIndex
			msg.ID = fmt.Sprintf("evt_%s_%d", sessionID, msg.Index)
			*keepGoing = visit(*msg)
			return
		}
		for _, key := range []string{"messages", "history", "turns", "contents", "conversation", "entries", "items", "curatedHistory"} {
			if child, ok := typed[key]; ok {
				collectStructuredSearchableMessages(agent, sessionID, child, depth+1, nextIndex, keepGoing, visit)
				if !*keepGoing {
					return
				}
			}
		}
	}
}

func sliceRange(messages []Message, opts ParseOptions) (int, int) {
	end := len(messages)
	if opts.Before != "" {
		index := findMessageIndex(messages, opts.Before)
		if index >= 0 {
			end = index
		}
	}

	start := end - opts.Limit
	if start < 0 {
		start = 0
	}

	if opts.After != "" {
		index := findMessageIndex(messages, opts.After)
		if index >= 0 {
			start = index + 1
			end = start + opts.Limit
			if end > len(messages) {
				end = len(messages)
			}
		}
	}
	return start, end
}

func findMessageIndex(messages []Message, cursor string) int {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return -1
	}
	if idx, err := strconv.Atoi(cursor); err == nil {
		for i, msg := range messages {
			if msg.Index == idx {
				return i
			}
		}
	}
	for i, msg := range messages {
		if msg.ID == cursor {
			return i
		}
	}
	return -1
}

func extractMessage(agent string, raw map[string]any, sessionID string, index int) *Message {
	msg := extractSearchableMessage(agent, raw, sessionID, index)
	if msg == nil {
		return nil
	}
	return searchableToMessage(*msg)
}

func extractSearchableMessage(agent string, raw map[string]any, sessionID string, index int) *SearchableMessage {
	switch strings.ToLower(agent) {
	case "codex":
		return extractCodexMessage(raw, sessionID, index)
	case "claude":
		return extractClaudeMessage(raw, sessionID, index)
	default:
		return extractGenericMessage(raw, sessionID, index)
	}
}

func extractCodexMessage(raw map[string]any, sessionID string, index int) *SearchableMessage {
	payload := asMap(raw["payload"])
	recordType := stringValue(raw["type"])
	eventType := stringValue(payload["type"])
	if eventType == "" {
		eventType = recordType
	}
	source := codexMessageSource(recordType, eventType)

	// Codex stores collaboration traffic in the session event stream as a
	// response_item/agent_message. It is addressed from one agent to another,
	// rather than to the user, and must never be rendered as chat content.
	if isCodexInterAgentMessage(recordType, eventType, payload) {
		return nil
	}

	switch eventType {
	case "thread_settings_applied":
		settings := asMap(payload["thread_settings"])
		model := firstNonEmpty(stringValue(settings["model"]), stringValue(payload["model"]))
		effort := firstNonEmpty(stringValue(settings["reasoning_effort"]), stringValue(settings["reasoningEffort"]), stringValue(payload["reasoning_effort"]))
		if model == "" && effort == "" {
			return nil
		}
		summary := strings.TrimSpace(strings.Join([]string{model, effort}, " · "))
		msg := newCodexSearchableMessage(sessionID, index, "system", "model_change", summary, extractTimestamp(raw, payload), source)
		if msg != nil {
			msg.Fields = map[string]any{"model": model, "reasoningEffort": effort}
		}
		return msg
	case "item_completed":
		item := asMap(payload["item"])
		if !strings.EqualFold(strings.TrimSpace(stringValue(item["type"])), "plan") {
			return nil
		}
		plan := strings.TrimSpace(stringValue(item["text"]))
		if plan == "" {
			return nil
		}
		turnID := codexTurnID(payload)
		if turnID == "" {
			turnID = strings.TrimSuffix(strings.TrimSpace(stringValue(item["id"])), "-plan")
		}
		msg := newCodexSearchableMessage(sessionID, index, "assistant", "proposed_plan", plan, extractTimestamp(raw, payload), source)
		if msg != nil {
			msg.Fields = map[string]any{"turnId": turnID, "plan": plan}
		}
		return msg
	case "task_complete":
		// A failed Codex turn is recorded as task_complete with the actual
		// provider error nested under error.message.  Keep that error in the
		// transcript: otherwise the user only sees their prompt and an idle
		// composer, with no indication that reauthentication (or another
		// recovery action) is required.
		errorPayload := asMap(payload["error"])
		errorMessage := strings.TrimSpace(stringValue(errorPayload["message"]))
		if errorMessage == "" {
			return nil
		}
		msg := newCodexSearchableMessage(sessionID, index, "system", "result", errorMessage, extractTimestamp(raw, payload), source)
		if msg != nil {
			msg.Fields = map[string]any{
				"subtype":    "error",
				"isError":    true,
				"durationMs": payload["duration_ms"],
			}
		}
		return msg
	case "patch_apply_end":
		changes := extractCodexFileChanges(payload["changes"])
		if len(changes) == 0 {
			return nil
		}
		status := firstNonEmpty(stringValue(payload["status"]), "completed")
		if success, ok := payload["success"].(bool); ok && !success {
			status = "failed"
		}
		itemID := firstNonEmpty(stringValue(payload["call_id"]), stringValue(payload["id"]))
		msg := newCodexSearchableMessage(sessionID, index, "tool", "file_change", fmt.Sprintf("%d files changed", len(changes)), extractTimestamp(raw, payload), source)
		if msg != nil {
			msg.Fields = map[string]any{"itemId": itemID, "status": status, "changes": changes}
		}
		return msg
	case "custom_tool_call":
		input := stringValue(payload["input"])
		if explanation, plan, ok := extractCodexPlanUpdate(input); ok {
			turnID := codexTurnID(payload)
			if turnID == "" {
				turnID = firstNonEmpty(stringValue(payload["call_id"]), stringValue(payload["id"]))
			}
			msg := newCodexSearchableMessage(sessionID, index, "tool", "turn_plan", "Plan", extractTimestamp(raw, payload), source)
			if msg != nil {
				msg.Fields = map[string]any{"turnId": turnID, "explanation": explanation, "plan": plan}
			}
			return msg
		}
		toolName := strings.TrimSpace(stringValue(payload["name"]))
		if changes := extractCodexPatchToolChanges(toolName, input); len(changes) > 0 {
			itemID := firstNonEmpty(stringValue(payload["call_id"]), stringValue(payload["id"]))
			msg := newCodexSearchableMessage(sessionID, index, "tool", "file_change", fmt.Sprintf("%d files changed", len(changes)), extractTimestamp(raw, payload), source)
			if msg != nil {
				// apply_patch is recorded as a custom tool call in newer Codex
				// transcripts. Unlike patch_apply_end it has no separate change
				// event, so render it as completed as soon as the patch is recorded.
				msg.Fields = map[string]any{"itemId": itemID, "status": "completed", "changes": changes}
			}
			return msg
		}
		if !strings.EqualFold(toolName, "exec") {
			return nil
		}
		command, cwd, ok := extractCodexExecCommand(input)
		if !ok {
			return nil
		}
		itemID := firstNonEmpty(stringValue(payload["call_id"]), stringValue(payload["id"]))
		msg := newCodexSearchableMessage(sessionID, index, "tool", "command_execution", command, extractTimestamp(raw, payload), source)
		if msg != nil {
			msg.Fields = map[string]any{"itemId": itemID, "command": command, "cwd": cwd, "status": "inProgress"}
		}
		return msg
	case "custom_tool_call_output":
		output := firstNonEmpty(flattenText(payload["output"]), flattenText(payload["content"]))
		msg := newCodexSearchableMessage(sessionID, index, "tool", "custom_tool_call_output", output, extractTimestamp(raw, payload), source)
		if msg != nil {
			status, exitCode, hasExitCode := codexCommandCompletion(output)
			msg.Fields = map[string]any{
				"itemId":           firstNonEmpty(stringValue(payload["call_id"]), stringValue(payload["id"])),
				"status":           status,
				"aggregatedOutput": output,
			}
			if hasExitCode {
				msg.Fields["exitCode"] = exitCode
			}
		}
		return msg
	case "user_message":
		text := firstNonEmpty(
			codexText(payload["message"]),
			codexText(payload["text"]),
			codexContentText(payload["content"]),
			codexText(raw["message"]),
		)
		return newCodexSearchableMessage(sessionID, index, "user", "message", text, extractTimestamp(raw, payload), source)
	case "agent_message":
		text := firstNonEmpty(
			codexText(payload["message"]),
			codexText(payload["text"]),
			codexContentText(payload["content"]),
		)
		if plan, ok := extractCodexProposedPlan(text); ok {
			return newCodexProposedPlanMessage(sessionID, index, payload, plan, extractTimestamp(raw, payload), source)
		}
		return newCodexSearchableMessage(sessionID, index, "assistant", "message", text, extractTimestamp(raw, payload), source)
	case "tool_call", "command_output", "tool_result":
		return newCodexSearchableMessage(sessionID, index, "tool", "tool", firstNonEmpty(
			flattenText(payload["output"]),
			flattenText(payload["message"]),
			flattenText(raw["output"]),
			flattenText(raw["message"]),
		), extractTimestamp(raw, payload), source)
	}

	if eventType != "message" {
		return nil
	}
	role := normalizeRole(firstNonEmpty(stringValue(payload["role"]), stringValue(raw["role"])))
	if role != "user" && role != "assistant" {
		return nil
	}
	text := firstNonEmpty(
		codexContentText(payload["content"]),
		codexText(payload["message"]),
		codexText(payload["text"]),
	)
	if role == "assistant" {
		if plan, ok := extractCodexProposedPlan(text); ok {
			return newCodexProposedPlanMessage(sessionID, index, payload, plan, extractTimestamp(raw, payload), source)
		}
	}
	return newCodexSearchableMessage(sessionID, index, role, "message", text, extractTimestamp(raw, payload), source)
}

func extractCodexProposedPlan(text string) (string, bool) {
	const openTag = "<proposed_plan>"
	const closeTag = "</proposed_plan>"
	trimmed := strings.TrimSpace(text)
	start := strings.Index(trimmed, openTag)
	if start < 0 {
		return "", false
	}
	planStart := start + len(openTag)
	endOffset := strings.Index(trimmed[planStart:], closeTag)
	if endOffset < 0 {
		return "", false
	}
	plan := strings.TrimSpace(trimmed[planStart : planStart+endOffset])
	return plan, plan != ""
}

func newCodexProposedPlanMessage(sessionID string, index int, payload map[string]any, plan string, createdAt *time.Time, source string) *SearchableMessage {
	turnID := codexTurnID(payload)
	if turnID == "" {
		turnID = firstNonEmpty(stringValue(payload["id"]), fmt.Sprintf("plan-%d", index))
	}
	msg := newCodexSearchableMessage(sessionID, index, "assistant", "proposed_plan", plan, createdAt, source)
	if msg != nil {
		msg.Fields = map[string]any{"turnId": turnID, "plan": plan}
	}
	return msg
}

func extractCodexFileChanges(value any) []map[string]any {
	changesByPath := asMap(value)
	if len(changesByPath) == 0 {
		return nil
	}
	paths := make([]string, 0, len(changesByPath))
	for path := range changesByPath {
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	changes := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		details := asMap(changesByPath[path])
		change := map[string]any{
			"path": strings.TrimSpace(path),
			"kind": firstNonEmpty(stringValue(details["type"]), stringValue(details["kind"]), "update"),
			"diff": firstNonEmpty(stringValue(details["unified_diff"]), stringValue(details["diff"])),
		}
		if movedToPath := firstNonEmpty(stringValue(details["move_path"]), stringValue(details["movedToPath"])); movedToPath != "" {
			change["movedToPath"] = movedToPath
		}
		changes = append(changes, change)
	}
	return changes
}

// extractCodexPatchToolChanges turns the textual input of the apply_patch
// custom tool into the native file-change shape consumed by the transcript UI.
// New Codex sessions record apply_patch directly rather than emitting the older
// patch_apply_end event, so without this the actual write is invisible.
func extractCodexPatchToolChanges(toolName, input string) []map[string]any {
	if !strings.Contains(strings.ToLower(strings.TrimSpace(toolName)), "patch") || !strings.Contains(input, "*** ") {
		return nil
	}

	var changes []map[string]any
	var current map[string]any
	var diffLines []string
	flush := func() {
		if current == nil {
			return
		}
		current["diff"] = strings.Join(diffLines, "\n")
		changes = append(changes, current)
		current = nil
		diffLines = nil
	}

	for _, line := range strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n") {
		var kind, path string
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			kind, path = "update", strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
		case strings.HasPrefix(line, "*** Add File: "):
			kind, path = "add", strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
		case strings.HasPrefix(line, "*** Delete File: "):
			kind, path = "delete", strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
		case line == "*** End Patch":
			flush()
			continue
		}
		if path != "" {
			flush()
			current = map[string]any{"path": path, "kind": kind}
			continue
		}
		if current != nil && line != "*** Begin Patch" {
			diffLines = append(diffLines, line)
		}
	}
	flush()
	return changes
}

func codexTurnID(payload map[string]any) string {
	if turnID := firstNonEmpty(stringValue(payload["turn_id"]), stringValue(payload["turnId"])); turnID != "" {
		return turnID
	}
	metadata := asMap(payload["internal_chat_message_metadata_passthrough"])
	return firstNonEmpty(stringValue(metadata["turn_id"]), stringValue(metadata["turnId"]))
}

func extractCodexPlanUpdate(input string) (string, []map[string]string, bool) {
	start := strings.Index(input, "tools.update_plan(")
	if start < 0 {
		return "", nil, false
	}
	objectStart := strings.Index(input[start:], "{")
	if objectStart < 0 {
		return "", nil, false
	}
	objectStart += start
	object, ok := extractBalancedJSONObject(input[objectStart:])
	if !ok {
		return "", nil, false
	}
	explanation := extractCodexPlanString(object, "explanation")
	// update_plan is emitted by Codex as JavaScript. Depending on the client
	// version, its object keys are either bare (`step:`) or JSON-quoted
	// (`"step":`). Accept both forms; real session files commonly use the
	// latter, which previously made the whole plan silently disappear.
	stepMatches := regexp.MustCompile(`(?s)(?:^|[,{]\s*)["']?step["']?\s*:\s*("(?:\\.|[^"\\])*")\s*,\s*["']?status["']?\s*:\s*("(?:\\.|[^"\\])*")`).FindAllStringSubmatch(object, -1)
	plan := make([]map[string]string, 0, len(stepMatches))
	for _, match := range stepMatches {
		if len(match) != 3 {
			continue
		}
		var step, status string
		if json.Unmarshal([]byte(match[1]), &step) != nil || json.Unmarshal([]byte(match[2]), &status) != nil || strings.TrimSpace(step) == "" {
			continue
		}
		plan = append(plan, map[string]string{"step": strings.TrimSpace(step), "status": normalizeCodexPlanStatus(status)})
	}
	return explanation, plan, len(plan) > 0
}

func normalizeCodexPlanStatus(status string) string {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(status), "-", "_")) {
	case "in_progress", "inprogress":
		return "inProgress"
	case "completed", "complete", "done":
		return "completed"
	default:
		return "pending"
	}
}

func extractBalancedJSONObject(value string) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for index, char := range value {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return value[:index+1], true
			}
		}
	}
	return "", false
}

func extractCodexPlanString(value string, name string) string {
	pattern := regexp.MustCompile(`(?s)(?:^|[,{]\s*)` + regexp.QuoteMeta(name) + `\s*:\s*("(?:\\.|[^"\\])*")`)
	match := pattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return ""
	}
	var result string
	if json.Unmarshal([]byte(match[1]), &result) != nil {
		return ""
	}
	return strings.TrimSpace(result)
}

func normalizeCodexCommandMessage(agent string, message *SearchableMessage, commandItems map[string]struct{}) bool {
	if !strings.EqualFold(strings.TrimSpace(agent), "codex") || message == nil {
		return true
	}
	itemID := strings.TrimSpace(stringValue(message.Fields["itemId"]))
	switch message.Kind {
	case "command_execution":
		if itemID == "" {
			return false
		}
		commandItems[itemID] = struct{}{}
		return true
	case "custom_tool_call_output":
		if _, ok := commandItems[itemID]; !ok {
			return false
		}
		message.Kind = "command_execution"
		return true
	default:
		return true
	}
}

var codexExecCommandFieldPattern = regexp.MustCompile(`(?:^|[,\{]\s*)["']?cmd["']?\s*:\s*("(?:\\.|[^"\\])*")`)
var codexExecWorkdirFieldPattern = regexp.MustCompile(`(?:^|[,\{]\s*)["']?workdir["']?\s*:\s*("(?:\\.|[^"\\])*")`)
var codexExitCodePattern = regexp.MustCompile(`(?i)(?:exited with code|exit(?:ed)? code\s*[:=]?)\s*(-?[0-9]+)`)

func extractCodexExecCommand(input string) (string, string, bool) {
	const marker = "tools.exec_command("
	commands := make([]string, 0, 1)
	workdirs := make([]string, 0, 1)
	for remaining := input; ; {
		markerIndex := strings.Index(remaining, marker)
		if markerIndex < 0 {
			break
		}
		remaining = remaining[markerIndex+len(marker):]
		segment := remaining
		if nextIndex := strings.Index(segment, marker); nextIndex >= 0 {
			segment = segment[:nextIndex]
		}
		command := extractCodexJSONStringField(segment, codexExecCommandFieldPattern)
		if strings.TrimSpace(command) != "" {
			commands = append(commands, command)
			workdirs = append(workdirs, extractCodexJSONStringField(segment, codexExecWorkdirFieldPattern))
		}
	}
	if len(commands) == 0 {
		return "", "", false
	}
	cwd := workdirs[0]
	for _, workdir := range workdirs[1:] {
		if workdir != cwd {
			cwd = ""
			break
		}
	}
	return strings.Join(commands, "\n"), cwd, true
}

func extractCodexJSONStringField(input string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(input)
	if len(match) != 2 {
		return ""
	}
	var value string
	if err := json.Unmarshal([]byte(match[1]), &value); err != nil {
		return ""
	}
	return value
}

func codexCommandCompletion(output string) (string, int, bool) {
	if match := codexExitCodePattern.FindStringSubmatch(output); len(match) == 2 {
		if exitCode, err := strconv.Atoi(match[1]); err == nil {
			if exitCode == 0 {
				return "completed", exitCode, true
			}
			return "failed", exitCode, true
		}
	}
	if strings.Contains(output, "Script completed") {
		return "completed", 0, true
	}
	return "completed", 0, false
}

func isCodexInterAgentMessage(recordType, eventType string, payload map[string]any) bool {
	if recordType != "response_item" || eventType != "agent_message" {
		return false
	}
	return strings.TrimSpace(stringValue(payload["author"])) != "" ||
		strings.TrimSpace(stringValue(payload["recipient"])) != ""
}

func codexText(value any) string {
	if text, ok := value.(string); ok {
		if isEmbeddedDataURL(text) {
			return ""
		}
		return text
	}
	return codexContentText(value)
}

func codexContentText(value any) string {
	switch typed := value.(type) {
	case string:
		if isEmbeddedDataURL(typed) {
			return ""
		}
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, value := range typed {
			if text := strings.TrimSpace(codexContentText(value)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]any:
		switch stringValue(typed["type"]) {
		case "input_text", "output_text", "text":
			text := codexText(typed["text"])
			if isCodexImageBoundaryText(text) {
				return ""
			}
			return text
		}
	}
	return ""
}

func isCodexImageBoundaryText(value string) bool {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "</image>") {
		return true
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "<image ") && strings.HasSuffix(lower, ">")
}

func isEmbeddedDataURL(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(text)), "data:")
}

func codexMessageSource(recordType string, eventType string) string {
	recordType = strings.TrimSpace(recordType)
	eventType = strings.TrimSpace(eventType)
	if recordType == "" {
		return ""
	}
	if eventType == "" || eventType == recordType {
		return "codex:" + recordType
	}
	return "codex:" + recordType + ":" + eventType
}

func newCodexSearchableMessage(sessionID string, index int, role, kind, text string, createdAt *time.Time, source string) *SearchableMessage {
	if strings.EqualFold(strings.TrimSpace(role), "assistant") {
		text = stripCodexInternalMemoryCitation(text)
	}
	msg := newSearchableMessage(sessionID, index, role, kind, text, createdAt)
	if msg != nil {
		msg.Source = source
	}
	return msg
}

func stripCodexInternalMemoryCitation(text string) string {
	return strings.TrimSpace(codexInternalMemoryCitationPattern.ReplaceAllString(text, ""))
}

func shouldDropSearchableDuplicate(agent string, previous *SearchableMessage, current *SearchableMessage) bool {
	if !strings.EqualFold(strings.TrimSpace(agent), "codex") || previous == nil || current == nil {
		return false
	}
	if previous.Role != current.Role || previous.Kind != current.Kind {
		return false
	}
	if strings.TrimSpace(previous.Text) == "" || strings.TrimSpace(previous.Text) != strings.TrimSpace(current.Text) {
		return false
	}
	if !isCodexResponseEventPair(previous.Source, current.Source) {
		return false
	}
	return transcriptTimesMatch(previous.CreatedAt, current.CreatedAt)
}

func isCodexResponseEventPair(left string, right string) bool {
	leftIsResponse := strings.HasPrefix(left, "codex:response_item:message")
	rightIsResponse := strings.HasPrefix(right, "codex:response_item:message")
	leftIsEvent := strings.HasPrefix(left, "codex:event_msg:")
	rightIsEvent := strings.HasPrefix(right, "codex:event_msg:")
	return (leftIsResponse && rightIsEvent) || (leftIsEvent && rightIsResponse)
}

func transcriptTimesMatch(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return true
	}
	delta := left.Sub(*right)
	if delta < 0 {
		delta = -delta
	}
	return delta <= 2*time.Second
}

func extractClaudeMessage(raw map[string]any, sessionID string, index int) *SearchableMessage {
	message := asMap(raw["message"])
	role := firstNonEmpty(stringValue(message["role"]), stringValue(raw["role"]))
	kind := "message"
	if role == "" {
		role = "assistant"
	}

	text := flattenText(message["content"])
	if text == "" {
		text = flattenText(raw["content"])
	}
	if text == "" {
		text = flattenText(raw["message"])
	}
	if text == "" {
		return nil
	}

	if strings.Contains(strings.ToLower(text), "tool_use") || strings.Contains(strings.ToLower(text), "tool_result") {
		role = "tool"
		kind = "tool"
	}
	return newSearchableMessage(sessionID, index, normalizeRole(role), kind, text, extractTimestamp(raw, message))
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "model":
		return "assistant"
	default:
		return role
	}
}

func extractGenericMessage(raw map[string]any, sessionID string, index int) *SearchableMessage {
	role := firstNonEmpty(stringValue(raw["role"]), stringValue(raw["event_type"]), "assistant")
	kind := "message"
	if role == "command_output" || role == "tool_call" || role == "tool" {
		role = "tool"
		kind = "tool"
	}
	text := firstNonEmpty(
		flattenText(raw["content"]),
		flattenText(raw["text"]),
		flattenText(raw["output"]),
		flattenText(raw["message"]),
		flattenText(raw["user"]),
		flattenText(raw["assistant"]),
	)
	if text == "" {
		return nil
	}
	return newSearchableMessage(sessionID, index, normalizeRole(role), kind, text, extractTimestamp(raw))
}

func newSearchableMessage(sessionID string, index int, role, kind, text string, createdAt *time.Time) *SearchableMessage {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return &SearchableMessage{
		ID:        fmt.Sprintf("evt_%s_%d", sessionID, index),
		SessionID: sessionID,
		Role:      role,
		Kind:      kind,
		Text:      text,
		Index:     index,
		CreatedAt: createdAt,
	}
}

func searchableToMessage(message SearchableMessage) *Message {
	return &Message{
		ID:        message.ID,
		SessionID: message.SessionID,
		Role:      message.Role,
		Kind:      message.Kind,
		Text:      message.Text,
		HTML:      render.MarkdownToHTML(message.Text),
		Direction: DetectDirection(message.Text),
		Index:     message.Index,
		CreatedAt: message.CreatedAt,
	}
}

func extractTimestamp(values ...map[string]any) *time.Time {
	for _, value := range values {
		for _, key := range []string{"created_at", "timestamp", "time"} {
			if raw := stringValue(value[key]); raw != "" {
				if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
					return &parsed
				}
			}
		}
	}
	return nil
}

func flattenText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := flattenText(item); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]any:
		if text := firstNonEmpty(
			flattenText(typed["text"]),
			flattenText(typed["message"]),
			flattenText(typed["content"]),
			flattenText(typed["parts"]),
			flattenText(typed["output"]),
			flattenText(typed["input"]),
		); text != "" {
			return text
		}
		if name := stringValue(typed["name"]); name != "" {
			args := flattenText(typed["arguments"])
			if args != "" {
				return name + "\n" + args
			}
			return name
		}
		if typed["type"] != nil {
			if body, err := json.MarshalIndent(typed, "", "  "); err == nil {
				return string(body)
			}
		}
	default:
		if body, err := json.Marshal(typed); err == nil {
			return string(body)
		}
	}
	return ""
}

func asMap(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return map[string]any{}
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func trimPreview(value string) string {
	value = inlinePreview(value)
	if len(value) > 160 {
		return strings.TrimSpace(value[:160]) + "..."
	}
	return value
}

func inlinePreview(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
}

func bytesTrimSpace(value []byte) []byte {
	return bytes.TrimSpace(value)
}
