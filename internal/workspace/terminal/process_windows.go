//go:build windows

package terminal

import (
	"io"
	"os/exec"
)

type pipeProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func startProcess(cmd *exec.Cmd, _ int, _ int) (process, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	return &pipeProcess{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

func (p *pipeProcess) Read(data []byte) (int, error) {
	return p.stdout.Read(data)
}

func (p *pipeProcess) Write(data []byte) (int, error) {
	return p.stdin.Write(data)
}

func (p *pipeProcess) Close() error {
	_ = p.stdin.Close()
	return p.stdout.Close()
}

func (p *pipeProcess) Resize(int, int) error {
	return nil
}

func (p *pipeProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func (p *pipeProcess) Wait() (int, *int) {
	err := p.cmd.Wait()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 1, nil
}
