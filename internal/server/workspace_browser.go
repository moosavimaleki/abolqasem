package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const localHTTPProbeTimeout = 650 * time.Millisecond

type localHTTPServerInfo struct {
	Title       string `json:"title"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Status      int    `json:"status"`
	OwnerPath   string `json:"ownerPath,omitempty"`
	ProcessName string `json:"processName,omitempty"`
	SameProject bool   `json:"sameProject,omitempty"`
}

type workspaceListeningPort struct {
	Port        int
	PID         int
	ProcessName string
	OwnerPath   string
}

var workspaceListListeningPorts = defaultWorkspaceListListeningPorts
var workspaceKillProcess = killProcess

func listWorkspaceLocalHTTPServers(raw json.RawMessage) ([]localHTTPServerInfo, error) {
	var payload struct {
		ProjectID string `json:"projectId"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	}
	projectPath := workspaceProjectLocalPath(payload.ProjectID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ports, err := workspaceListListeningPorts(ctx)
	if err != nil {
		return []localHTTPServerInfo{}, nil
	}

	seen := map[int]bool{}
	servers := make([]localHTTPServerInfo, 0, len(ports))
	for _, port := range ports {
		if port.Port <= 0 || seen[port.Port] {
			continue
		}
		seen[port.Port] = true
		server := probeLocalHTTPServer(ctx, port)
		if server.Status == 0 {
			continue
		}
		server.SameProject = sameOrChildPath(projectPath, port.OwnerPath)
		servers = append(servers, server)
	}
	sort.SliceStable(servers, func(i, j int) bool {
		if servers[i].SameProject != servers[j].SameProject {
			return servers[i].SameProject
		}
		return servers[i].Port < servers[j].Port
	})
	return servers, nil
}

func killWorkspaceLocalHTTPServer(raw json.RawMessage) error {
	var payload struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	if payload.Port <= 0 || payload.Port > 65535 {
		return errors.New("valid port is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ports, err := workspaceListListeningPorts(ctx)
	if err != nil {
		return err
	}
	killed := false
	for _, port := range ports {
		if port.Port != payload.Port || port.PID <= 0 || port.PID == os.Getpid() {
			continue
		}
		if err := workspaceKillProcess(port.PID); err != nil {
			return err
		}
		killed = true
	}
	if !killed {
		return errors.New("no local HTTP server found for port")
	}
	return nil
}

func probeLocalHTTPServer(ctx context.Context, port workspaceListeningPort) localHTTPServerInfo {
	address := "http://127.0.0.1:" + strconv.Itoa(port.Port)
	requestCtx, cancel := context.WithTimeout(ctx, localHTTPProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, address, nil)
	if err != nil {
		return localHTTPServerInfo{Address: address, Port: port.Port}
	}
	client := http.Client{Timeout: localHTTPProbeTimeout}
	response, err := client.Do(request)
	if err != nil {
		return localHTTPServerInfo{Address: address, Port: port.Port}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	title := htmlTitle(string(body))
	if title == "" {
		title = firstNonEmptyServer(port.ProcessName, "localhost:"+strconv.Itoa(port.Port))
	}
	return localHTTPServerInfo{
		Title:       title,
		Address:     address,
		Port:        port.Port,
		Status:      response.StatusCode,
		OwnerPath:   port.OwnerPath,
		ProcessName: port.ProcessName,
	}
}

func defaultWorkspaceListListeningPorts(ctx context.Context) ([]workspaceListeningPort, error) {
	if runtime.GOOS == "windows" {
		return listWindowsListeningPorts(ctx)
	}
	if ports, err := listLsofListeningPorts(ctx); err == nil {
		return ports, nil
	}
	return listSSListeningPorts(ctx)
}

func listLsofListeningPorts(ctx context.Context) ([]workspaceListeningPort, error) {
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	ports := make([]workspaceListeningPort, 0)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		port := parsePortFromAddress(fields[len(fields)-1])
		if port <= 0 {
			continue
		}
		ports = append(ports, workspaceListeningPort{
			Port:        port,
			PID:         pid,
			ProcessName: fields[0],
			OwnerPath:   processCWD(pid),
		})
	}
	return ports, nil
}

func listSSListeningPorts(ctx context.Context) ([]workspaceListeningPort, error) {
	output, err := exec.CommandContext(ctx, "ss", "-ltnp").Output()
	if err != nil {
		return nil, err
	}
	pidRE := regexp.MustCompile(`pid=([0-9]+)`)
	nameRE := regexp.MustCompile(`users:\(\("([^"]+)"`)
	ports := make([]workspaceListeningPort, 0)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "LISTEN" {
			continue
		}
		port := parsePortFromAddress(fields[3])
		if port <= 0 {
			continue
		}
		pid := 0
		if match := pidRE.FindStringSubmatch(line); len(match) == 2 {
			pid, _ = strconv.Atoi(match[1])
		}
		processName := ""
		if match := nameRE.FindStringSubmatch(line); len(match) == 2 {
			processName = match[1]
		}
		ports = append(ports, workspaceListeningPort{
			Port:        port,
			PID:         pid,
			ProcessName: processName,
			OwnerPath:   processCWD(pid),
		})
	}
	return ports, nil
}

func listWindowsListeningPorts(ctx context.Context) ([]workspaceListeningPort, error) {
	output, err := exec.CommandContext(ctx, "netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return nil, err
	}
	ports := make([]workspaceListeningPort, 0)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		port := parsePortFromAddress(fields[1])
		if port <= 0 {
			continue
		}
		pid, _ := strconv.Atoi(fields[4])
		ports = append(ports, workspaceListeningPort{Port: port, PID: pid})
	}
	return ports, nil
}

func workspaceProjectLocalPath(projectID string) string {
	if strings.TrimSpace(projectID) == "" {
		return ""
	}
	state, err := workspaceStore().LoadState()
	if err != nil {
		return ""
	}
	project := state.ProjectsByID[projectID]
	return project.LocalPath
}

func processCWD(pid int) string {
	if pid <= 0 || runtime.GOOS != "linux" {
		return ""
	}
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	if err != nil {
		return ""
	}
	return cwd
}

func parsePortFromAddress(address string) int {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0
	}
	if host, portValue, err := net.SplitHostPort(address); err == nil {
		_ = host
		port, _ := strconv.Atoi(portValue)
		return port
	}
	idx := strings.LastIndex(address, ":")
	if idx < 0 || idx == len(address)-1 {
		return 0
	}
	port, _ := strconv.Atoi(strings.Trim(address[idx+1:], "[]"))
	return port
}

func htmlTitle(body string) string {
	lower := strings.ToLower(body)
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	closeStart := strings.Index(lower[start:], ">")
	if closeStart < 0 {
		return ""
	}
	titleStart := start + closeStart + 1
	end := strings.Index(lower[titleStart:], "</title>")
	if end < 0 {
		return ""
	}
	title := strings.TrimSpace(body[titleStart : titleStart+end])
	return strings.Join(strings.Fields(title), " ")
}

func sameOrChildPath(projectPath string, ownerPath string) bool {
	projectPath = strings.TrimSpace(projectPath)
	ownerPath = strings.TrimSpace(ownerPath)
	if projectPath == "" || ownerPath == "" {
		return false
	}
	projectPath = filepath.Clean(projectPath)
	ownerPath = filepath.Clean(ownerPath)
	if ownerPath == projectPath {
		return true
	}
	rel, err := filepath.Rel(projectPath, ownerPath)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func killProcess(pid int) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
