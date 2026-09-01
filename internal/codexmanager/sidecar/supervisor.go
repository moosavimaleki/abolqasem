package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

type Supervisor struct {
	Executable     string
	HTTPClient     *http.Client
	RestartBackoff time.Duration

	mu      sync.Mutex
	cmd     *exec.Cmd
	lock    *flock.Flock
	status  Status
	crashes int
}

func (s *Supervisor) Restart(ctx context.Context, config Config, apiKey string) (Status, error) {
	if err := s.Stop(ctx); err != nil {
		return s.Status(), err
	}
	backoff := s.RestartBackoff
	if backoff <= 0 {
		backoff = 250 * time.Millisecond
	}
	select {
	case <-ctx.Done():
		return s.Status(), ctx.Err()
	case <-time.After(backoff):
	}
	return s.Start(ctx, config, apiKey)
}

func (s *Supervisor) Start(ctx context.Context, config Config, apiKey string) (Status, error) {
	if err := validateConfig(config, apiKey); err != nil {
		return s.fail(config, err)
	}
	executable, err := resolveExecutable(s.Executable)
	if err != nil {
		return s.fail(config, err)
	}
	s.mu.Lock()
	if s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil {
		status := s.status
		s.mu.Unlock()
		return status, nil
	}
	s.status = Status{State: StateStarting, Listen: config.ListenAddress, StartedAt: time.Now().UTC(), CrashCount: s.crashes}
	s.mu.Unlock()
	lock, err := acquireLock(config.ManagerHome)
	if err != nil {
		return s.fail(config, err)
	}
	command := exec.Command(executable)
	command.Env = append(os.Environ(),
		"CODEX_MANAGER_GATEWAY_LISTEN="+config.ListenAddress,
		"CODEX_MANAGER_HOME="+config.ManagerHome,
		"CODEX_MANAGER_MODELS_CACHE="+config.ModelsCache,
		"CODEX_MANAGER_GATEWAY_UPSTREAM="+config.UpstreamBase,
		config.APIKeyEnv+"="+apiKey,
	)
	if config.Proxy != "" {
		command.Env = append(command.Env, "CODEX_MANAGER_PROXY="+config.Proxy)
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		_ = lock.Unlock()
		return s.fail(config, err)
	}
	s.mu.Lock()
	s.cmd = command
	s.lock = lock
	s.status.PID = command.Process.Pid
	s.mu.Unlock()
	go s.watch(command, lock)
	if _, err := s.waitReady(ctx, config.ListenAddress); err != nil {
		_ = s.Stop(context.Background())
		return s.fail(config, err)
	}
	s.mu.Lock()
	s.status.State = StateReady
	s.status.LastHealthy = time.Now().UTC()
	status := s.status
	s.mu.Unlock()
	return status, nil
}

func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	command := s.cmd
	if command != nil {
		s.status.State = StateStopped
		s.status.PID = 0
	}
	s.mu.Unlock()
	if command == nil || command.Process == nil || command.ProcessState != nil {
		s.markStopped(command)
		return nil
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		if !errors.Is(err, os.ErrProcessDone) {
			return err
		}
	}
	done := make(chan struct{})
	go func() {
		for {
			s.mu.Lock()
			active := s.cmd == command
			s.mu.Unlock()
			if !active {
				close(done)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	select {
	case <-ctx.Done():
		_ = command.Process.Kill()
		return ctx.Err()
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		return errors.New("sidecar did not stop gracefully")
	case <-done:
		s.markStopped(command)
		return nil
	}
}

func (s *Supervisor) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

func (s *Supervisor) waitReady(ctx context.Context, listen string) (Health, error) {
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Second}
	}
	endpoint := "http://" + listen + "/health"
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return Health{}, err
		}
		response, err := client.Do(req)
		if err == nil {
			var health struct {
				Status             string `json:"status"`
				Version            string `json:"version"`
				ConfiguredAccounts int    `json:"configuredAccounts"`
				Loopback           bool   `json:"loopback"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&health)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && health.Status == "ok" && health.Loopback {
				return Health{OK: true, Version: health.Version, ConfiguredAccounts: health.ConfiguredAccounts, Loopback: true}, nil
			}
		}
		select {
		case <-ctx.Done():
			return Health{}, ctx.Err()
		case <-deadline.C:
			return Health{}, errors.New("sidecar readiness timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *Supervisor) fail(config Config, err error) (Status, error) {
	s.mu.Lock()
	s.status = Status{State: StateCrashed, Listen: config.ListenAddress, LastError: redactError(err)}
	status := s.status
	s.mu.Unlock()
	return status, err
}

func (s *Supervisor) watch(command *exec.Cmd, lock *flock.Flock) {
	err := command.Wait()
	_ = lock.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != command {
		return
	}
	s.cmd = nil
	s.lock = nil
	s.status.PID = 0
	if s.status.State == StateStopped {
		return
	}
	s.status.State = StateCrashed
	s.crashes++
	s.status.CrashCount = s.crashes
	if err != nil {
		s.status.LastError = redactError(err)
	}
}

func (s *Supervisor) markStopped(command *exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if command != nil && s.cmd != command && s.cmd != nil {
		return
	}
	s.status.State = StateStopped
	s.status.PID = 0
}

func resolveExecutable(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if current, err := os.Executable(); err == nil {
		name := "codex-manager-gateway"
		if filepath.Ext(current) == ".exe" {
			name += ".exe"
		}
		adjacent := filepath.Join(filepath.Dir(current), name)
		if info, statErr := os.Stat(adjacent); statErr == nil && !info.IsDir() {
			return adjacent, nil
		}
	}
	executable, err := exec.LookPath("codex-manager-gateway")
	if err != nil {
		return "", errors.New("codex-manager sidecar binary was not found")
	}
	return executable, nil
}

func acquireLock(managerHome string) (*flock.Flock, error) {
	if err := os.MkdirAll(managerHome, 0o700); err != nil {
		return nil, fmt.Errorf("create sidecar state directory: %w", err)
	}
	lock := flock.New(filepath.Join(managerHome, "gateway.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock sidecar state: %w", err)
	}
	if !locked {
		return nil, errors.New("a codex-manager sidecar already owns this state directory")
	}
	return lock, nil
}

func validateConfig(config Config, apiKey string) error {
	address, err := net.ResolveTCPAddr("tcp", config.ListenAddress)
	if err != nil || address.IP == nil || !address.IP.IsLoopback() {
		return errors.New("sidecar listen address must be loopback")
	}
	upstream, err := url.Parse(config.UpstreamBase)
	if err != nil || upstream.Scheme != "https" {
		return errors.New("sidecar upstream must use HTTPS")
	}
	if strings.TrimSpace(config.ManagerHome) == "" || strings.TrimSpace(config.APIKeyEnv) == "" || strings.TrimSpace(apiKey) == "" {
		return errors.New("sidecar configuration is incomplete")
	}
	return nil
}

func redactError(err error) string {
	message := err.Error()
	if len(message) > 240 {
		message = message[:240]
	}
	return fmt.Sprintf("%s", message)
}
