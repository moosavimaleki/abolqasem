package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func normalizeMCPRegistryInstallCommand(command []string) ([]string, error) {
	command = normalizeStringList(command)
	if len(command) == 0 {
		return nil, nil
	}
	switch mcpRegistryExecutableName(command[0]) {
	case "npm":
		if err := validateNPMRegistryInstallCommand(command); err != nil {
			return nil, err
		}
		command[0] = npmCommandName()
		return command, nil
	case "uv":
		if err := validateUVRegistryInstallCommand(command); err != nil {
			return nil, err
		}
		command[0] = uvCommandName()
		return command, nil
	default:
		return nil, errors.New("unsupported MCP registry install command")
	}
}

func validateNPMRegistryInstallCommand(command []string) error {
	if len(command) < 4 || command[1] != "cache" || command[2] != "add" {
		return errors.New("unsupported npm MCP registry install command")
	}
	if !isSafeRegistryInstallValue(command[3]) || strings.HasPrefix(command[3], "-") {
		return errors.New("invalid npm MCP registry package")
	}
	for index := 4; index < len(command); index++ {
		if command[index] != "--registry" || index+1 >= len(command) || !isSafeRegistryInstallValue(command[index+1]) {
			return errors.New("unsupported npm MCP registry install option")
		}
		index++
	}
	return nil
}

func validateUVRegistryInstallCommand(command []string) error {
	if len(command) != 4 || command[1] != "tool" || command[2] != "install" {
		return errors.New("unsupported uv MCP registry install command")
	}
	if !isSafeRegistryInstallValue(command[3]) || strings.HasPrefix(command[3], "-") {
		return errors.New("invalid uv MCP registry package")
	}
	return nil
}

func isSafeRegistryInstallValue(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func mcpRegistryExecutableName(value string) string {
	name := strings.ToLower(strings.TrimSpace(value))
	name = strings.ReplaceAll(name, "\\", "/")
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	name = strings.TrimSuffix(name, ".exe")
	name = strings.TrimSuffix(name, ".cmd")
	return name
}

func npmCommandName() string {
	if runtime.GOOS == "windows" {
		return "npm.cmd"
	}
	return "npm"
}

func uvCommandName() string {
	if runtime.GOOS == "windows" {
		return "uv.exe"
	}
	return "uv"
}

func defaultRunMCPRegistryInstallCommand(command []string) (mcpRegistryCommandOutput, error) {
	cwd, err := os.UserHomeDir()
	if err != nil {
		return mcpRegistryCommandOutput{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "DISABLE_TELEMETRY="+firstNonEmptyEnv("DISABLE_TELEMETRY", "1"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = fmt.Sprintf("MCP registry install command exited with error: %v", err)
		}
		return mcpRegistryCommandOutput{}, errors.New(message)
	}
	return mcpRegistryCommandOutput{CWD: cwd, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}
