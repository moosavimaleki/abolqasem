package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"abolqasem/internal/state"
)

func workspaceTransientProviderEnv(provider string) ([]string, func(), error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil, nil, fmt.Errorf("unsupported commit message provider: %s", provider)
	}

	tempHome, err := os.MkdirTemp("", "codex-rtl-commit-message-"+provider+"-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempHome)
	}

	baseEnv := state.CurrentProviderProxyEnv()
	var sourceRoot string
	var targetRoot string
	var skipDir string
	env := baseEnv

	switch provider {
	case "codex":
		sourceRoot = workspaceCodexRootDir()
		targetRoot = filepath.Join(tempHome, ".codex")
		env = workspaceSetEnvValue(env, "CODEX_HOME", targetRoot)
		skipDir = "sessions"
	case "claude":
		sourceRoot = workspaceClaudeRootDir()
		targetRoot = filepath.Join(tempHome, ".claude")
		env = workspaceSetEnvValue(env, "CLAUDE_CONFIG_DIR", targetRoot)
		env = workspaceSetEnvValue(env, "CLAUDE_HOME", targetRoot)
		skipDir = "projects"
	default:
		cleanup()
		return nil, nil, fmt.Errorf("unsupported commit message provider: %s", provider)
	}

	if err := workspaceCopyProviderHome(sourceRoot, targetRoot, skipDir); err != nil {
		cleanup()
		return nil, nil, err
	}

	return env, cleanup, nil
}

func workspaceCopyProviderHome(sourceRoot string, targetRoot string, skipDir string) error {
	sourceRoot = strings.TrimSpace(sourceRoot)
	targetRoot = strings.TrimSpace(targetRoot)
	if targetRoot == "" {
		return fmt.Errorf("invalid target root: %s", targetRoot)
	}
	if sourceRoot == "" {
		return nil
	}
	sourceRoot = filepath.Clean(sourceRoot)
	targetRoot = filepath.Clean(targetRoot)
	if targetRoot == "." || targetRoot == string(filepath.Separator) {
		return fmt.Errorf("invalid target root: %s", targetRoot)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return err
	}

	info, err := os.Stat(sourceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("provider root is not a directory: %s", sourceRoot)
	}

	return filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(targetRoot, 0o755)
		}
		if entry.IsDir() && strings.EqualFold(filepath.Base(rel), skipDir) {
			return filepath.SkipDir
		}
		targetPath := filepath.Join(targetRoot, rel)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return workspaceCopyFile(path, targetPath)
	})
}

func workspaceCopyFile(sourcePath string, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Chmod(info.Mode().Perm())
}

func workspaceSetEnvValue(env []string, key string, value string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return env
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return env
	}

	replaced := false
	prefix := key + "="
	next := make([]string, 0, len(env)+1)
	for _, entry := range env {
		entryKey, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(entryKey, key) {
			if !replaced {
				next = append(next, prefix+value)
				replaced = true
			}
			continue
		}
		next = append(next, entry)
	}
	if !replaced {
		next = append(next, prefix+value)
	}
	return next
}

func workspaceCodexRootDir() string {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func workspaceClaudeRootDir() string {
	if value := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("CLAUDE_HOME")); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}
