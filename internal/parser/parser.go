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
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-agent-manager/internal/render"
	"ai-agent-manager/internal/state"
)

var ErrTranscriptUnavailable = errors.New("transcript is unavailable")

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
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineIndex++
			raw := map[string]any{}
			if decodeErr := json.Unmarshal(bytesTrimSpace(line), &raw); decodeErr == nil {
				if msg := extractSearchableMessage(agent, raw, sessionID, lineIndex); msg != nil {
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
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineIndex++
			raw := map[string]any{}
			if decodeErr := json.Unmarshal(bytesTrimSpace(line), &raw); decodeErr == nil {
				if msg := extractSearchableMessage(agent, raw, sessionID, lineIndex); msg != nil {
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
	case "gemini":
		return extractGeminiMessage(raw, sessionID, index)
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

	switch eventType {
	case "user_message":
		return newCodexSearchableMessage(sessionID, index, "user", "message", firstNonEmpty(
			flattenText(payload["message"]),
			flattenText(payload["text"]),
			flattenText(raw["message"]),
		), extractTimestamp(raw, payload), source)
	case "agent_message":
		return newCodexSearchableMessage(sessionID, index, "assistant", "message", firstNonEmpty(
			flattenText(payload["message"]),
			flattenText(payload["text"]),
			flattenText(payload["content"]),
		), extractTimestamp(raw, payload), source)
	case "tool_call", "command_output", "tool_result":
		return newCodexSearchableMessage(sessionID, index, "tool", "tool", firstNonEmpty(
			flattenText(payload["output"]),
			flattenText(payload["message"]),
			flattenText(raw["output"]),
			flattenText(raw["message"]),
		), extractTimestamp(raw, payload), source)
	}

	if text := firstNonEmpty(flattenText(payload["message"]), flattenText(payload["content"]), flattenText(raw["message"]), flattenText(raw["text"]), flattenText(raw["content"])); text != "" {
		role := firstNonEmpty(stringValue(payload["role"]), stringValue(raw["role"]), "assistant")
		return newCodexSearchableMessage(sessionID, index, normalizeRole(role), "message", text, extractTimestamp(raw, payload), source)
	}
	return nil
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
	msg := newSearchableMessage(sessionID, index, role, kind, text, createdAt)
	if msg != nil {
		msg.Source = source
	}
	return msg
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

func extractGeminiMessage(raw map[string]any, sessionID string, index int) *SearchableMessage {
	role := normalizeRole(firstNonEmpty(stringValue(raw["role"]), stringValue(raw["speaker"])))
	kind := "message"
	text := firstNonEmpty(
		flattenText(raw["parts"]),
		flattenText(raw["content"]),
		flattenText(raw["response"]),
		flattenText(raw["prompt"]),
		flattenText(raw["text"]),
	)
	if text == "" {
		return nil
	}
	if role == "" {
		role = "assistant"
	}
	if role == "tool" {
		kind = "tool"
	}
	return newSearchableMessage(sessionID, index, role, kind, text, extractTimestamp(raw))
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
