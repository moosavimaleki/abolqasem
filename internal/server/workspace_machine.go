package server

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	workspaceMachinePlatform      = func() string { return runtime.GOOS }
	workspaceMachineHostname      = os.Hostname
	workspaceMachineCommandOutput = func(command string, args ...string) (string, error) {
		output, err := exec.Command(command, args...).Output()
		return string(output), err
	}
)

func workspaceMachineName() string {
	if workspaceMachinePlatform() == "darwin" {
		if computerName, err := workspaceMachineCommandOutput("scutil", "--get", "ComputerName"); err == nil {
			if trimmed := strings.TrimSpace(computerName); trimmed != "" {
				return trimmed
			}
		}
	}

	hostname, err := workspaceMachineHostname()
	if err != nil {
		return "This Machine"
	}
	trimmed := strings.TrimSpace(hostname)
	trimmed = stripLocalNetworkSuffix(trimmed)
	if trimmed == "" {
		return "This Machine"
	}
	return trimmed
}

func stripLocalNetworkSuffix(value string) string {
	lower := strings.ToLower(value)
	for _, suffix := range []string{".local", ".lan"} {
		if strings.HasSuffix(lower, suffix) {
			return value[:len(value)-len(suffix)]
		}
	}
	return value
}

func workspacePlatform() string {
	if workspaceMachinePlatform() == "windows" {
		return "win32"
	}
	return workspaceMachinePlatform()
}
