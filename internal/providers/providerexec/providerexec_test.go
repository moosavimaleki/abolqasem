package providerexec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveCommandUsesWorkingBunCodex(t *testing.T) {
	SetConfiguredExecutables(nil)
	t.Cleanup(func() { SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setTestHome(t, home)
	codexPath := filepath.Join(home, ".bun", "bin", testExecutableName("codex"))
	writeExecutable(t, codexPath, "#!/bin/sh\nexit 0\n")

	command := ResolveCommand("codex", "codex --sandbox workspace-write")
	if command != codexPath+" --sandbox workspace-write" {
		t.Fatalf("expected bun codex command, got %q", command)
	}
}

func TestExecutableUsesWorkingLocalClaudeBeforeBrokenBunClaude(t *testing.T) {
	SetConfiguredExecutables(nil)
	t.Cleanup(func() { SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setTestHome(t, home)
	localClaudePath := filepath.Join(home, ".local", "bin", testExecutableName("claude"))
	writeExecutable(t, localClaudePath, "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(home, ".bun", "bin", testExecutableName("claude")), "#!/bin/sh\nexit 1\n")

	if got := Executable("claude"); got != localClaudePath {
		t.Fatalf("expected local claude command, got %q", got)
	}
}

func TestExecutableUsesConfiguredPathBeforeDetectedPath(t *testing.T) {
	SetConfiguredExecutables(nil)
	t.Cleanup(func() { SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setTestHome(t, home)
	configuredCodexPath := filepath.Join(home, "tools", testExecutableName("codex"))
	detectedCodexPath := filepath.Join(home, ".bun", "bin", testExecutableName("codex"))
	writeExecutable(t, configuredCodexPath, "#!/bin/sh\nexit 0\n")
	writeExecutable(t, detectedCodexPath, "#!/bin/sh\nexit 0\n")
	SetConfiguredExecutables(map[string]string{"codex": configuredCodexPath})

	if got := Executable("codex"); got != configuredCodexPath {
		t.Fatalf("expected configured codex command, got %q", got)
	}
}

func TestExecutableFindsWindowsExeForExtensionlessConfiguredPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows executable extension fallback")
	}
	SetConfiguredExecutables(nil)
	t.Cleanup(func() { SetConfiguredExecutables(nil) })
	home := t.TempDir()
	setTestHome(t, home)
	configuredCodexPath := filepath.Join(home, "tools", "codex")
	writeExecutable(t, configuredCodexPath+".exe", "#!/bin/sh\nexit 0\n")
	SetConfiguredExecutables(map[string]string{"codex": configuredCodexPath})

	if got := Executable("codex"); got != configuredCodexPath+".exe" {
		t.Fatalf("expected configured codex executable with .exe suffix, got %q", got)
	}
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func testExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	exitCode := 0
	if strings.Contains(content, "exit 1") {
		exitCode = 1
	}
	sourcePath := filepath.Join(t.TempDir(), "main.go")
	source := fmt.Sprintf("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(%d) }\n", exitCode)
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile source returned error: %v", err)
	}
	command := exec.Command("go", "build", "-o", path, sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go build fake executable failed: %v\n%s", err, output)
	}
}
