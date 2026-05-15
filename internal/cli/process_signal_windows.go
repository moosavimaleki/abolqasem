//go:build windows

package cli

import "os"

func terminateProcess(process *os.Process) error {
	return process.Kill()
}
