package eventstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/readmodels"
)

var ErrInvalidStream = errors.New("invalid event stream")

const (
	SnapshotFileName      = "snapshot.json"
	CompactionThreshold   = 2 * 1024 * 1024
	snapshotFileMode      = 0o644
	eventLogFileMode      = 0o644
	eventLogScannerBuffer = 8 * 1024 * 1024
)

type SnapshotFile struct {
	V              int                        `json:"v"`
	GeneratedAt    int64                      `json:"generatedAt"`
	Projects       []readmodels.ProjectRecord `json:"projects"`
	Chats          []readmodels.ChatRecord    `json:"chats"`
	QueuedMessages []QueuedMessageSet         `json:"queuedMessages,omitempty"`
}

type QueuedMessageSet struct {
	ChatID  string                         `json:"chatId"`
	Entries []readmodels.QueuedChatMessage `json:"entries"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) Append(stream string, event events.Event) error {
	if err := validateStream(stream); err != nil {
		return err
	}
	if event.V == 0 {
		event.V = events.Version
	}
	if event.Type == "" {
		return errors.New("event type is required")
	}
	if event.Timestamp == 0 {
		return errors.New("event timestamp is required")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.streamPath(stream), os.O_CREATE|os.O_WRONLY|os.O_APPEND, eventLogFileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (s *Store) Replay(stream string) ([]events.Event, error) {
	if err := validateStream(stream); err != nil {
		return nil, err
	}

	file, err := os.Open(s.streamPath(stream))
	if err != nil {
		if os.IsNotExist(err) {
			return []events.Event{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var result []events.Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event events.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", s.streamPath(stream), lineNumber, err)
		}
		result = append(result, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) LoadState() (readmodels.StoreState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadSnapshotLocked()
	if err != nil {
		return readmodels.StoreState{}, err
	}
	replayed, err := s.replayAllLocked()
	if err != nil {
		return readmodels.StoreState{}, err
	}
	for _, event := range replayed {
		state = readmodels.Apply(state, event)
	}
	return state, nil
}

func (s *Store) Compact(state readmodels.StoreState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}

	snapshot := SnapshotFile{
		V:              events.Version,
		GeneratedAt:    time.Now().UnixMilli(),
		Projects:       activeProjects(state),
		Chats:          activeChats(state),
		QueuedMessages: queuedMessages(state),
	}
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	tmpPath := s.snapshotPath() + ".tmp"
	if err := os.WriteFile(tmpPath, payload, snapshotFileMode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.snapshotPath()); err != nil {
		return err
	}

	for _, stream := range events.Streams() {
		if err := os.WriteFile(s.streamPath(stream), nil, eventLogFileMode); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ShouldCompact() (bool, error) {
	var total int64
	for _, stream := range events.Streams() {
		info, err := os.Stat(s.streamPath(stream))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, err
		}
		total += info.Size()
	}
	return total >= CompactionThreshold, nil
}

func (s *Store) streamPath(stream string) string {
	return filepath.Join(s.dir, stream+".jsonl")
}

func (s *Store) snapshotPath() string {
	return filepath.Join(s.dir, SnapshotFileName)
}

func (s *Store) loadSnapshotLocked() (readmodels.StoreState, error) {
	state := readmodels.EmptyState()
	data, err := os.ReadFile(s.snapshotPath())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return readmodels.StoreState{}, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return state, nil
	}

	var snapshot SnapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return readmodels.StoreState{}, err
	}
	if snapshot.V != events.Version {
		return readmodels.StoreState{}, fmt.Errorf("unsupported snapshot version: %d", snapshot.V)
	}

	for _, project := range snapshot.Projects {
		state.ProjectsByID[project.ID] = project
		if project.LocalPath != "" && project.DeletedAt == 0 {
			state.ProjectIDsByPath[project.LocalPath] = project.ID
		}
	}
	for _, chat := range snapshot.Chats {
		state.ChatsByID[chat.ID] = chat
	}
	for _, queuedSet := range snapshot.QueuedMessages {
		state.QueuedMessagesByChatID[queuedSet.ChatID] = append([]readmodels.QueuedChatMessage(nil), queuedSet.Entries...)
	}
	return state, nil
}

func (s *Store) replayAllLocked() ([]events.Event, error) {
	type replayEvent struct {
		event       events.Event
		sourceIndex int
		lineIndex   int
	}

	var replayed []replayEvent
	for sourceIndex, stream := range events.Streams() {
		streamEvents, err := s.replayStreamLocked(stream)
		if err != nil {
			return nil, err
		}
		for lineIndex, event := range streamEvents {
			replayed = append(replayed, replayEvent{
				event:       event,
				sourceIndex: sourceIndex,
				lineIndex:   lineIndex,
			})
		}
	}

	sort.SliceStable(replayed, func(i, j int) bool {
		left := replayed[i]
		right := replayed[j]
		if left.event.Timestamp != right.event.Timestamp {
			return left.event.Timestamp < right.event.Timestamp
		}
		if eventPriority(left.event.Type) != eventPriority(right.event.Type) {
			return eventPriority(left.event.Type) < eventPriority(right.event.Type)
		}
		if left.sourceIndex != right.sourceIndex {
			return left.sourceIndex < right.sourceIndex
		}
		return left.lineIndex < right.lineIndex
	})

	result := make([]events.Event, 0, len(replayed))
	for _, entry := range replayed {
		result = append(result, entry.event)
	}
	return result, nil
}

func (s *Store) replayStreamLocked(stream string) ([]events.Event, error) {
	file, err := os.Open(s.streamPath(stream))
	if err != nil {
		if os.IsNotExist(err) {
			return []events.Event{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var result []events.Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), eventLogScannerBuffer)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event events.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", s.streamPath(stream), lineNumber, err)
		}
		result = append(result, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func validateStream(stream string) error {
	switch stream {
	case events.StreamProjects,
		events.StreamChats,
		events.StreamMessages,
		events.StreamQueuedMessages,
		events.StreamTurns:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidStream, stream)
	}
}

func activeProjects(state readmodels.StoreState) []readmodels.ProjectRecord {
	projects := make([]readmodels.ProjectRecord, 0, len(state.ProjectsByID))
	for _, project := range state.ProjectsByID {
		if project.DeletedAt != 0 {
			continue
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].UpdatedAt > projects[j].UpdatedAt
	})
	return projects
}

func activeChats(state readmodels.StoreState) []readmodels.ChatRecord {
	chats := make([]readmodels.ChatRecord, 0, len(state.ChatsByID))
	for _, chat := range state.ChatsByID {
		if chat.DeletedAt != 0 {
			continue
		}
		chats = append(chats, chat)
	}
	sort.Slice(chats, func(i, j int) bool {
		return chats[i].UpdatedAt > chats[j].UpdatedAt
	})
	return chats
}

func queuedMessages(state readmodels.StoreState) []QueuedMessageSet {
	sets := make([]QueuedMessageSet, 0, len(state.QueuedMessagesByChatID))
	for chatID, entries := range state.QueuedMessagesByChatID {
		if len(entries) == 0 {
			continue
		}
		sets = append(sets, QueuedMessageSet{
			ChatID:  chatID,
			Entries: append([]readmodels.QueuedChatMessage(nil), entries...),
		})
	}
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].ChatID < sets[j].ChatID
	})
	return sets
}

func eventPriority(eventType string) int {
	switch eventType {
	case events.TypeProjectOpened, events.TypeProjectSidebarRenamed, events.TypeProjectRemoved:
		return 0
	case events.TypeChatCreated:
		return 1
	case events.TypeChatRenamed, events.TypeChatProviderSet, events.TypeChatPlanModeSet:
		return 2
	case events.TypeMessageAppended:
		return 3
	case events.TypeQueuedMessageEnqueued, events.TypeQueuedMessageRemoved:
		return 4
	case events.TypeTurnStarted:
		return 5
	case events.TypeSessionTokenSet, events.TypePendingForkSessionTokenSet:
		return 6
	case events.TypeTurnCancelled:
		return 7
	case events.TypeTurnFinished, events.TypeTurnFailed:
		return 8
	case events.TypeChatReadStateSet:
		return 9
	case events.TypeChatDeleted, events.TypeChatArchived, events.TypeChatUnarchived:
		return 10
	default:
		return 100
	}
}
