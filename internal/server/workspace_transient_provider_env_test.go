package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-agent-manager/internal/workspace/agent"
)

func TestWorkspaceTransientProviderEnvCopiesProviderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	type fixture struct {
		provider         string
		sourceRoot       string
		sourceEnvKey     string
		targetEnvKey     string
		skipDir          string
		configRelPath    string
		sessionRelPath   string
		expectedCopyPath string
	}

	fixtures := []fixture{
		{
			provider:         "codex",
			sourceRoot:       filepath.Join(home, ".codex"),
			sourceEnvKey:     "CODEX_HOME",
			targetEnvKey:     "CODEX_HOME",
			skipDir:          "sessions",
			configRelPath:    "config/settings.json",
			sessionRelPath:   "sessions/2026/05/27/rollout-test.jsonl",
			expectedCopyPath: "config/settings.json",
		},
		{
			provider:         "claude",
			sourceRoot:       filepath.Join(home, ".claude"),
			sourceEnvKey:     "CLAUDE_HOME",
			targetEnvKey:     "CLAUDE_HOME",
			skipDir:          "projects",
			configRelPath:    "config/settings.json",
			sessionRelPath:   "projects/project-1/session.jsonl",
			expectedCopyPath: "config/settings.json",
		},
		{
			provider:         "gemini",
			sourceRoot:       filepath.Join(home, "gemini-base", ".gemini"),
			sourceEnvKey:     "GEMINI_CLI_HOME",
			targetEnvKey:     "GEMINI_CLI_HOME",
			skipDir:          "tmp",
			configRelPath:    "settings.json",
			sessionRelPath:   "tmp/container/chats/session-1.jsonl",
			expectedCopyPath: "settings.json",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.provider, func(t *testing.T) {
			if fixture.provider == "claude" {
				t.Setenv("CLAUDE_CONFIG_DIR", "")
			}
			if fixture.provider == "gemini" {
				t.Setenv(fixture.sourceEnvKey, filepath.Dir(fixture.sourceRoot))
			} else {
				t.Setenv(fixture.sourceEnvKey, fixture.sourceRoot)
			}
			writeProviderHomeFixture(t, fixture.sourceRoot, fixture.configRelPath, "config="+fixture.provider)
			writeProviderHomeFixture(t, fixture.sourceRoot, fixture.sessionRelPath, "session="+fixture.provider)

			env, cleanup, err := workspaceTransientProviderEnv(fixture.provider)
			if err != nil {
				t.Fatalf("workspaceTransientProviderEnv returned error: %v", err)
			}
			defer cleanup()

			targetEnvValue := envValue(env, fixture.targetEnvKey)
			if targetEnvValue == "" {
				t.Fatalf("expected %s in env, got %#v", fixture.targetEnvKey, env)
			}

			targetRoot := targetEnvValue
			if fixture.provider == "gemini" {
				targetRoot = filepath.Join(targetEnvValue, ".gemini")
			}
			if _, err := os.Stat(filepath.Join(targetRoot, fixture.expectedCopyPath)); err != nil {
				t.Fatalf("expected copied config at %s: %v", filepath.Join(targetRoot, fixture.expectedCopyPath), err)
			}
			if _, err := os.Stat(filepath.Join(targetRoot, fixture.skipDir)); !os.IsNotExist(err) {
				t.Fatalf("expected %s to be excluded from %s, got err=%v", fixture.skipDir, targetRoot, err)
			}

			tempHome := filepath.Dir(targetRoot)
			cleanup()
			if _, err := os.Stat(tempHome); !os.IsNotExist(err) {
				t.Fatalf("expected temp home to be removed, got err=%v", err)
			}
		})
	}
}

func TestStartWorkspaceTurnUsesTransientEnv(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	claudeEnvFile := filepath.Join(binDir, "claude.env")
	geminiEnvFile := filepath.Join(binDir, "gemini.env")
	codexEnvFile := filepath.Join(binDir, "codex.env")

	writeFakeExecutable(t, filepath.Join(binDir, "claude"), fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$CLAUDE_HOME" "$CLAUDE_CONFIG_DIR" > %q
printf '%%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"claude-output"}]}}'
printf '%%s\n' '{"type":"result","result":"done","duration_ms":1,"session_id":"claude-session-1"}'
`, claudeEnvFile), fmt.Sprintf(`@echo off
> "%s" (
  echo %%CLAUDE_HOME%%
  echo %%CLAUDE_CONFIG_DIR%%
)
echo {"type":"assistant","message":{"content":[{"type":"text","text":"claude-output"}]}}
echo {"type":"result","result":"done","duration_ms":1,"session_id":"claude-session-1"}
`, claudeEnvFile))

	writeFakeExecutable(t, filepath.Join(binDir, "gemini"), fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$GEMINI_CLI_HOME" > %q
echo gemini-output
`, geminiEnvFile), fmt.Sprintf(`@echo off
> "%s" echo %%GEMINI_CLI_HOME%%
echo gemini-output
`, geminiEnvFile))

	writeFakeExecutable(t, filepath.Join(binDir, "codex"), fmt.Sprintf(`#!/bin/sh
if [ "$1" = "app-server" ]; then
  printf '%%s\n' "$CODEX_HOME" > %q
  exit 0
fi
exit 1
`, codexEnvFile), fmt.Sprintf(`@echo off
if "%%~1"=="app-server" (
  > "%s" echo %%CODEX_HOME%%
  exit /b 0
)
exit /b 1
`, codexEnvFile))

	claudeTurn := startWorkspaceClaudeTurn(context.Background(), agent.TurnRequest{
		Content: "hello",
		Env: []string{
			"CLAUDE_HOME=" + filepath.Join(home, "isolated", ".claude"),
			"CLAUDE_CONFIG_DIR=" + filepath.Join(home, "isolated", ".claude"),
		},
	})
	if events := drainWorkspaceTurnEvents(claudeTurn); !hasTurnEvent(events, agent.TurnEventFinished, "") {
		t.Fatalf("expected finished claude turn, got %#v", events)
	}
	if lines := readArgsFile(t, claudeEnvFile); len(lines) != 2 || lines[0] != filepath.Join(home, "isolated", ".claude") || lines[1] != filepath.Join(home, "isolated", ".claude") {
		t.Fatalf("expected claude env override, got %#v", lines)
	}

	geminiTurn := startWorkspaceGeminiTurn(context.Background(), agent.TurnRequest{
		Content: "hello",
		Env: []string{
			"GEMINI_CLI_HOME=" + filepath.Join(home, "isolated-gemini"),
		},
	})
	if events := drainWorkspaceTurnEvents(geminiTurn); !hasTurnEvent(events, agent.TurnEventFinished, "") {
		t.Fatalf("expected finished gemini turn, got %#v", events)
	}
	if lines := readArgsFile(t, geminiEnvFile); len(lines) != 1 || lines[0] != filepath.Join(home, "isolated-gemini") {
		t.Fatalf("expected gemini env override, got %#v", lines)
	}

	process, err := startWorkspaceCodexProcess(context.Background(), t.TempDir(), []string{
		"CODEX_HOME=" + filepath.Join(home, "isolated-codex", ".codex"),
	})
	if err != nil {
		t.Fatalf("startWorkspaceCodexProcess returned error: %v", err)
	}
	defer process.Close()

	lines := waitForLines(t, codexEnvFile)
	if len(lines) != 1 || lines[0] != filepath.Join(home, "isolated-codex", ".codex") {
		t.Fatalf("expected codex env override, got %#v", lines)
	}
}

func writeProviderHomeFixture(t *testing.T, root string, relPath string, contents string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", fullPath, err)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(entry, prefix))
		}
	}
	return ""
}

func waitForLines(t *testing.T, path string) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if lines := readLinesIfReady(t, path); len(lines) > 0 {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readLinesIfReady(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return []string{}
	}
	lines := strings.Split(text, "\n")
	return lines
}
