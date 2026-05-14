package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs, nil
	}
	return path, nil
}

func ShellCommand(agent string) (string, error) {
	exe, err := ExecutablePath()
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(exe, ' ') {
		exe = fmt.Sprintf("%q", exe)
	}
	return exe + " hook --agent " + agent, nil
}

func CommandArgs(agent string) (string, []string, error) {
	exe, err := ExecutablePath()
	if err != nil {
		return "", nil, err
	}
	return exe, []string{"hook", "--agent", agent}, nil
}

func IsCommandMatch(command, agent string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	hookSuffix := " hook --agent " + agent
	if strings.HasSuffix(command, hookSuffix) || strings.Contains(command, hookSuffix) {
		return true
	}
	if strings.Contains(command, "codex-rtl hook") && agent == "codex" {
		return true
	}
	return false
}
