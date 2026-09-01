package opencode

import (
	"context"
	"os/exec"
	"time"
)

// cliOutputTimeout bounds commands which have already printed their JSON but
// keep OpenCode's local helper process alive. We accept a complete JSON result
// in sessions.go, so waiting for that helper only harms the host application.
const cliOutputTimeout = 2 * time.Second

func execCommandContextDefault(ctx context.Context, command string, args ...string) ([]byte, error) {
	return execCommandWithDirectory(ctx, "", command, args...)
}

func execCommandWithDirectory(ctx context.Context, directory string, command string, args ...string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, cliOutputTimeout)
	defer cancel()
	process := exec.CommandContext(bounded, command, args...)
	process.Dir = directory
	return process.CombinedOutput()
}
