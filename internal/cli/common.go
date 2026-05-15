package cli

import (
	"ai-agent-manager/internal/platform"
	"ai-agent-manager/internal/state"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

const (
	autoPortStart = state.DefaultPort
	autoPortEnd   = state.DefaultPort + 99
)

func currentBaseURL() string {
	return state.LoadServerBaseURL()
}

func serverHealthy() bool {
	return serverHealthyAt(currentBaseURL())
}

func serverHealthyAt(baseURL string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/api/state")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var payload struct {
		App string `json:"app"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false
	}
	return payload.App == "ai-agent-manager"
}

func waitForServer(timeout time.Duration) bool {
	return waitForServerAt(currentBaseURL(), timeout)
}

func waitForServerAt(baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if serverHealthyAt(baseURL) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return serverHealthyAt(baseURL)
}

func ensureServerRunning(timeout time.Duration) error {
	_, err := ensureServerRunningInternal(timeout, false)
	return err
}

func ensureServerRunningForHook(timeout time.Duration) error {
	_, err := ensureServerRunningInternal(timeout, true)
	return err
}

func ensureServerRunningInternal(timeout time.Duration, openOnStart bool) (bool, error) {
	if baseURL, ok := discoverRunningServer(); ok {
		_ = state.SaveServerBaseURL(baseURL)
		return false, nil
	}

	var lastErr error
	for port := autoPortStart; port <= autoPortEnd; port++ {
		if !canListen(port) {
			continue
		}
		baseURL := state.DefaultBaseURL(port)
		if err := state.SaveServerBaseURL(baseURL); err != nil {
			return false, err
		}
		if err := startServerInBackground(port); err != nil {
			lastErr = err
			continue
		}
		if waitForServerAt(baseURL, timeout) {
			if openOnStart {
				_ = platform.OpenBrowser(baseURL)
			}
			return true, nil
		}
	}
	if lastErr != nil {
		return false, lastErr
	}
	return false, fmt.Errorf("server did not become healthy on ports %d-%d", autoPortStart, autoPortEnd)
}

func startServerInBackground(port int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "__server", "--port", strconv.Itoa(port))
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = detachedProcessAttributes()
	}
	err = cmd.Start()
	closeErr := devNull.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func discoverRunningServer() (string, bool) {
	if baseURL := currentBaseURL(); serverHealthyAt(baseURL) {
		return baseURL, true
	}
	for port := autoPortStart; port <= autoPortEnd; port++ {
		baseURL := state.DefaultBaseURL(port)
		if serverHealthyAt(baseURL) {
			return baseURL, true
		}
	}
	return "", false
}

func canListen(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func listenOnAvailablePort() (net.Listener, int, error) {
	for port := autoPortStart; port <= autoPortEnd; port++ {
		listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("no available port in range %d-%d", autoPortStart, autoPortEnd)
}

func configuredPort() int {
	baseURL := currentBaseURL()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return state.DefaultPort
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 {
		return state.DefaultPort
	}
	return port
}
