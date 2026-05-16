package eventstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ai-agent-manager/internal/workspace/events"
)

var ErrInvalidStream = errors.New("invalid event stream")

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
	file, err := os.OpenFile(s.streamPath(stream), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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

func (s *Store) streamPath(stream string) string {
	return filepath.Join(s.dir, stream+".jsonl")
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
