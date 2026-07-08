package cli

import (
	"abolqasem/internal/appinfo"
	"abolqasem/internal/state"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	autoPortStart = state.DefaultPort
	autoPortEnd   = state.DefaultPort + 99
)

type serverRuntimeInfo struct {
	App string `json:"app"`
	PID int    `json:"pid"`
}

func currentBaseURL() string {
	return state.LoadServerBaseURL()
}

func serverHealthy() bool {
	return serverHealthyAt(currentBaseURL())
}

func serverHealthyAt(baseURL string) bool {
	info, ok := serverRuntimeInfoAt(baseURL)
	return ok && isKnownAppName(info.App)
}

func serverRuntimeInfoAt(baseURL string) (serverRuntimeInfo, bool) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/api/state")
	if err != nil {
		return serverRuntimeInfo{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return serverRuntimeInfo{}, false
	}
	var payload serverRuntimeInfo
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return serverRuntimeInfo{}, false
	}
	return payload, isKnownAppName(payload.App)
}

func isKnownAppName(name string) bool {
	return name == appinfo.Name || name == appinfo.LegacyName
}

func waitForServer(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if serverHealthy() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return serverHealthy()
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

func ensureServiceRunning(timeout time.Duration) error {
	if !isServiceInstalled() {
		return fmt.Errorf("service is not installed; run %s install", appinfo.Name)
	}
	if serverHealthy() {
		return nil
	}
	if err := startService(); err != nil {
		return err
	}
	if !waitForServer(timeout) {
		return fmt.Errorf("service did not become healthy at %s", currentBaseURL())
	}
	return nil
}

func discoverRunningServer() (string, bool) {
	baseURL, _, ok := discoverRunningServerInfo()
	return baseURL, ok
}

func discoverRunningServerInfo() (string, serverRuntimeInfo, bool) {
	if baseURL := currentBaseURL(); serverHealthyAt(baseURL) {
		info, _ := serverRuntimeInfoAt(baseURL)
		return baseURL, info, true
	}
	for port := autoPortStart; port <= autoPortEnd; port++ {
		baseURL := state.DefaultBaseURL(port)
		if serverHealthyAt(baseURL) {
			info, _ := serverRuntimeInfoAt(baseURL)
			return baseURL, info, true
		}
	}
	return "", serverRuntimeInfo{}, false
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
