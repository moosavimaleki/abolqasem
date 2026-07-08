package providerexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var configuredExecutables = struct {
	sync.RWMutex
	paths map[string]string
}{paths: map[string]string{}}

func ResolveCommand(provider string, command string) string {
	provider = Normalize(provider)
	command = strings.TrimSpace(command)
	if provider == "" || command == "" {
		return command
	}
	head, suffix := commandHead(command)
	if executableBase(head) != provider {
		return command
	}
	if resolved := Executable(provider); resolved != "" {
		return resolved + suffix
	}
	return command
}

func ExecutableOrName(provider string) string {
	if resolved := Executable(provider); resolved != "" {
		return resolved
	}
	if normalized := Normalize(provider); normalized != "" {
		return normalized
	}
	return strings.TrimSpace(provider)
}

func Executable(provider string) string {
	provider = Normalize(provider)
	if provider == "" {
		return ""
	}
	candidates := configuredExecutableCandidates(provider)
	candidates = append(candidates, detectedExecutableCandidates(provider)...)
	for _, candidate := range candidates {
		for _, path := range executableCandidatePaths(candidate) {
			if executableWorks(path) {
				return path
			}
		}
	}
	return ""
}

func DetectExecutable(provider string) string {
	provider = Normalize(provider)
	if provider == "" {
		return ""
	}
	for _, candidate := range detectedExecutableCandidates(provider) {
		for _, path := range executableCandidatePaths(candidate) {
			if executableWorks(path) {
				return path
			}
		}
	}
	return ""
}

func SetConfiguredExecutables(paths map[string]string) {
	next := map[string]string{}
	for provider, path := range paths {
		provider = Normalize(provider)
		path = strings.TrimSpace(path)
		if provider != "" && path != "" {
			next[provider] = path
		}
	}
	configuredExecutables.Lock()
	configuredExecutables.paths = next
	configuredExecutables.Unlock()
}

func configuredExecutableCandidates(provider string) []string {
	configuredExecutables.RLock()
	defer configuredExecutables.RUnlock()
	if path := strings.TrimSpace(configuredExecutables.paths[provider]); path != "" {
		return []string{path}
	}
	return nil
}

func detectedExecutableCandidates(provider string) []string {
	candidates := []string{}
	executable := platformExecutableName(provider)
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		switch provider {
		case "claude":
			candidates = append(candidates,
				filepath.Join(home, ".local", "bin", executable),
				filepath.Join(home, ".bun", "bin", executable),
			)
		case "codex":
			candidates = append(candidates,
				filepath.Join(home, ".local", "bin", executable),
				filepath.Join(home, ".bun", "install", "global", "node_modules", "@openai", "codex-linux-x64", "vendor", "x86_64-unknown-linux-musl", "bin", executable),
				filepath.Join(home, ".bun", "bin", executable),
			)
		default:
			candidates = append(candidates,
				filepath.Join(home, ".bun", "bin", executable),
				filepath.Join(home, ".local", "bin", executable),
			)
		}
	}
	if path, err := exec.LookPath(provider); err == nil {
		candidates = append(candidates, path)
	}
	return candidates
}

func platformExecutableName(provider string) string {
	if runtime.GOOS == "windows" {
		return provider + ".exe"
	}
	return provider
}

func executableBase(path string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	if runtime.GOOS == "windows" {
		base = strings.TrimSuffix(base, ".exe")
	}
	return base
}

func executableCandidatePaths(path string) []string {
	path = strings.TrimSpace(path)
	if runtime.GOOS != "windows" || strings.EqualFold(filepath.Ext(path), ".exe") {
		return []string{path}
	}
	return []string{path, path + ".exe"}
}

func Normalize(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	switch provider {
	case "claude", "codex", "gemini":
		return provider
	default:
		return ""
	}
}

func commandHead(command string) (string, string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", ""
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", ""
	}
	head := fields[0]
	return head, strings.TrimPrefix(command, head)
}

func executableWorks(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, "--version").Run() == nil
}
