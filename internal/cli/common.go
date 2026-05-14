package cli

import (
	"ai-agent-manager/internal/state"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func currentBaseURL() string {
	return state.LoadServerBaseURL()
}

func serverHealthy() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(currentBaseURL() + "/api/state")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
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
	cmd := exec.Command(exe, "server", "--port", strconv.Itoa(port))
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	return cmd.Start()
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
