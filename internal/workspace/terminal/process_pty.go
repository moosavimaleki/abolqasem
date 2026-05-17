//go:build !windows

package terminal

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type ptyProcess struct {
	file *os.File
	cmd  *exec.Cmd
}

func startProcess(cmd *exec.Cmd, cols int, rows int) (process, error) {
	file, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		return nil, err
	}
	return &ptyProcess{file: file, cmd: cmd}, nil
}

func (p *ptyProcess) Read(data []byte) (int, error) {
	return p.file.Read(data)
}

func (p *ptyProcess) Write(data []byte) (int, error) {
	return p.file.Write(data)
}

func (p *ptyProcess) Close() error {
	return p.file.Close()
}

func (p *ptyProcess) Resize(cols int, rows int) error {
	return pty.Setsize(p.file, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func (p *ptyProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func (p *ptyProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *ptyProcess) Wait() (int, *int) {
	err := p.cmd.Wait()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			exitCode := status.ExitStatus()
			if status.Signaled() {
				sig := int(status.Signal())
				return exitCode, &sig
			}
			return exitCode, nil
		}
		return exitErr.ExitCode(), nil
	}
	return 1, nil
}
