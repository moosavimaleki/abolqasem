package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	defaultTerminalStateLimitBytes = 256 * 1024
	maxTerminalStateLimitBytes     = 4 * 1024 * 1024
)

type Snapshot struct {
	TerminalID      string `json:"terminalId"`
	Title           string `json:"title"`
	CWD             string `json:"cwd"`
	Shell           string `json:"shell"`
	Cols            int    `json:"cols"`
	Rows            int    `json:"rows"`
	Scrollback      int    `json:"scrollback"`
	SerializedState string `json:"serializedState"`
	Status          string `json:"status"`
	ExitCode        *int   `json:"exitCode"`
	Signal          *int   `json:"signal,omitempty"`
}

type Event struct {
	Type       string `json:"type"`
	TerminalID string `json:"terminalId"`
	Data       string `json:"data,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Signal     *int   `json:"signal,omitempty"`
}

type CreateRequest struct {
	ProjectID   string
	TerminalID  string
	CWD         string
	Mode        string
	ChatID      string
	TmuxSession string
	Command     string
	Cols        int
	Rows        int
	Scrollback  int
}

type Manager struct {
	onEvent func(Event)

	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	id         string
	title      string
	cwd        string
	shell      string
	cols       int
	rows       int
	scrollback int
	state      terminalState
	status     string
	exitCode   *int
	signal     *int
	process    process
	closeOnce  sync.Once
}

type process interface {
	io.ReadWriteCloser
	Resize(cols int, rows int) error
	Kill() error
	Wait() (int, *int)
	PID() int
}

func NewManager(onEvent func(Event)) *Manager {
	if onEvent == nil {
		onEvent = func(Event) {}
	}
	return &Manager{
		onEvent:  onEvent,
		sessions: map[string]*session{},
	}
}

func (m *Manager) Create(ctx context.Context, request CreateRequest) (Snapshot, error) {
	if strings.TrimSpace(request.TerminalID) == "" {
		return Snapshot{}, errors.New("terminalId is required")
	}
	cols, rows := normalizeSize(request.Cols, request.Rows)
	cwd := normalizeCWD(request.CWD)

	m.mu.Lock()
	if existing := m.sessions[request.TerminalID]; existing != nil {
		existing.cols = cols
		existing.rows = rows
		existing.scrollback = request.Scrollback
		existing.state.setLimit(terminalStateLimit(request.Scrollback))
		snapshot := existing.snapshotLocked()
		m.mu.Unlock()
		_ = existing.process.Resize(cols, rows)
		return snapshot, nil
	}
	m.mu.Unlock()

	shell := defaultShell()
	title := "Terminal"
	cmd := exec.CommandContext(ctx, shell)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	proc, err := startProcess(cmd, cols, rows)
	if err != nil {
		return Snapshot{}, err
	}
	item := &session{
		id:         request.TerminalID,
		title:      title,
		cwd:        cwd,
		shell:      filepath.Base(shell),
		cols:       cols,
		rows:       rows,
		scrollback: request.Scrollback,
		state:      newTerminalState(terminalStateLimit(request.Scrollback)),
		status:     "running",
		process:    proc,
	}

	m.mu.Lock()
	m.sessions[item.id] = item
	m.mu.Unlock()

	go m.readLoop(item)
	go m.waitLoop(item)

	m.mu.Lock()
	snapshot := item.snapshotLocked()
	m.mu.Unlock()
	return snapshot, nil
}

func (m *Manager) Snapshot(terminalID string) *Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := m.sessions[terminalID]
	if item == nil {
		return nil
	}
	snapshot := item.snapshotLocked()
	return &snapshot
}

func (m *Manager) Input(terminalID string, data string) error {
	m.mu.Lock()
	item := m.sessions[terminalID]
	m.mu.Unlock()
	if item == nil {
		return errors.New("terminal not found")
	}
	_, err := item.process.Write([]byte(data))
	return err
}

func (m *Manager) Resize(terminalID string, cols int, rows int) error {
	cols, rows = normalizeSize(cols, rows)
	m.mu.Lock()
	item := m.sessions[terminalID]
	if item != nil {
		item.cols = cols
		item.rows = rows
	}
	m.mu.Unlock()
	if item == nil {
		return errors.New("terminal not found")
	}
	return item.process.Resize(cols, rows)
}

func (m *Manager) Close(terminalID string) error {
	m.mu.Lock()
	item := m.sessions[terminalID]
	delete(m.sessions, terminalID)
	m.mu.Unlock()
	if item == nil {
		return nil
	}
	item.closeOnce.Do(func() {
		_ = item.process.Kill()
		_ = item.process.Close()
	})
	return nil
}

func (m *Manager) RootPIDsByCWD(cwd string) []int {
	cwd = normalizeCWD(cwd)
	cwd = filepath.Clean(cwd)
	m.mu.Lock()
	defer m.mu.Unlock()
	pids := make([]int, 0)
	for _, item := range m.sessions {
		if item == nil || item.status != "running" || item.process == nil {
			continue
		}
		if filepath.Clean(item.cwd) != cwd {
			continue
		}
		pid := item.process.PID()
		if pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

func (m *Manager) readLoop(item *session) {
	buffer := make([]byte, 4096)
	for {
		n, err := item.process.Read(buffer)
		if n > 0 {
			data := string(buffer[:n])
			m.mu.Lock()
			item.state.WriteString(data)
			m.mu.Unlock()
			m.onEvent(Event{Type: "terminal.output", TerminalID: item.id, Data: data})
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) waitLoop(item *session) {
	exitCode, signal := item.process.Wait()
	m.mu.Lock()
	if current := m.sessions[item.id]; current == item {
		item.status = "exited"
		item.exitCode = &exitCode
		item.signal = signal
	}
	m.mu.Unlock()
	m.onEvent(Event{Type: "terminal.exit", TerminalID: item.id, ExitCode: &exitCode, Signal: signal})
}

func (s *session) snapshotLocked() Snapshot {
	return Snapshot{
		TerminalID:      s.id,
		Title:           s.title,
		CWD:             s.cwd,
		Shell:           s.shell,
		Cols:            s.cols,
		Rows:            s.rows,
		Scrollback:      s.scrollback,
		SerializedState: s.state.String(),
		Status:          s.status,
		ExitCode:        s.exitCode,
		Signal:          s.signal,
	}
}

func normalizeSize(cols int, rows int) (int, int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return cols, rows
}

func normalizeCWD(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd != "" {
		if info, err := os.Stat(cwd); err == nil && info.IsDir() {
			return cwd
		}
	}
	current, err := os.Getwd()
	if err != nil {
		return os.TempDir()
	}
	return current
}

func defaultShell() string {
	if runtime.GOOS == "windows" {
		if shell := strings.TrimSpace(os.Getenv("COMSPEC")); shell != "" {
			return shell
		}
		return "cmd.exe"
	}
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "/bin/sh"
}

type terminalState struct {
	data  string
	limit int
}

func newTerminalState(limit int) terminalState {
	return terminalState{limit: limit}
}

func (s *terminalState) setLimit(limit int) {
	s.limit = limit
	s.trim()
}

func (s *terminalState) WriteString(value string) {
	s.data += value
	s.trim()
}

func (s *terminalState) String() string {
	return s.data
}

func (s *terminalState) trim() {
	if s.limit <= 0 {
		s.limit = defaultTerminalStateLimitBytes
	}
	if len(s.data) <= s.limit {
		return
	}
	s.data = s.data[len(s.data)-s.limit:]
}

func terminalStateLimit(scrollback int) int {
	if scrollback <= 0 {
		return defaultTerminalStateLimitBytes
	}
	limit := scrollback * 200
	if limit < defaultTerminalStateLimitBytes {
		return defaultTerminalStateLimitBytes
	}
	if limit > maxTerminalStateLimitBytes {
		return maxTerminalStateLimitBytes
	}
	return limit
}
