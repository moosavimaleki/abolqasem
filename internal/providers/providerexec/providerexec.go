package providerexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	if filepath.Base(head) != provider {
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
		if executableWorks(candidate) {
			return candidate
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
		if executableWorks(candidate) {
			return candidate
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
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		switch provider {
		case "claude":
			candidates = append(candidates,
				filepath.Join(home, ".local", "bin", provider),
				filepath.Join(home, ".bun", "bin", provider),
			)
		case "codex":
			candidates = append(candidates,
				filepath.Join(home, ".local", "bin", provider),
				filepath.Join(home, ".bun", "install", "global", "node_modules", "@openai", "codex-linux-x64", "vendor", "x86_64-unknown-linux-musl", "bin", provider),
				filepath.Join(home, ".bun", "bin", provider),
			)
		default:
			candidates = append(candidates,
				filepath.Join(home, ".bun", "bin", provider),
				filepath.Join(home, ".local", "bin", provider),
			)
		}
	}
	if path, err := exec.LookPath(provider); err == nil {
		candidates = append(candidates, path)
	}
	return candidates
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
