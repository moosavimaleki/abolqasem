package sessioninterop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

func TestExportNativeSessionRealCLISmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv("AI_AGENT_MANAGER_REAL_CLI_SMOKE")) == "" {
		t.Skip("set AI_AGENT_MANAGER_REAL_CLI_SMOKE=1 to run real provider CLI resume smoke tests")
	}

	for _, provider := range []string{"claude", "codex", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			if _, err := exec.LookPath(provider); err != nil {
				t.Skipf("%s CLI is not installed: %v", provider, err)
			}

			home := t.TempDir()
			projectDir := filepath.Join(home, "project")
			if err := os.MkdirAll(projectDir, 0o755); err != nil {
				t.Fatalf("mkdir project: %v", err)
			}
			setTestHome(t, home)
			t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
			t.Setenv("CLAUDE_HOME", filepath.Join(home, ".claude"))
			t.Setenv("GEMINI_CLI_HOME", filepath.Join(home, ".gemini"))

			needle := "hello smoke from " + provider
			result, err := ExportNativeSession(ExportArgs{
				Provider:  provider,
				LocalPath: projectDir,
				Entries: []readmodels.TranscriptEntry{
					transcript.New(transcript.KindUserPrompt, map[string]any{"content": needle}),
					transcript.New(transcript.KindAssistantText, map[string]any{"text": "assistant smoke reply"}),
				},
			})
			if err != nil {
				t.Fatalf("ExportNativeSession returned error: %v", err)
			}

			output := runProviderResumeSmoke(t, provider, projectDir, result.SessionToken, needle, home)
			if !strings.Contains(strings.ToLower(output), strings.ToLower(needle)) {
				t.Fatalf("expected %s resume output to mention %q, got:\n%s", provider, needle, output)
			}
		})
	}
}

func runProviderResumeSmoke(t *testing.T, provider string, cwd string, sessionToken string, needle string, home string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	prompt := "What was the first user message in this resumed session? Reply with only that message."
	var cmd *exec.Cmd
	switch provider {
	case "claude":
		cmd = exec.CommandContext(ctx, "claude",
			"--resume", sessionToken,
			"--print", prompt,
			"--output-format", "text",
			"--permission-mode", "plan",
			"--disable-slash-commands",
			"--no-chrome",
			"--tools", "",
		)
	case "codex":
		cmd = exec.CommandContext(ctx, "codex",
			"resume", sessionToken, prompt,
			"--ask-for-approval", "never",
			"--sandbox", "read-only",
		)
	case "gemini":
		cmd = exec.CommandContext(ctx, "gemini",
			"--resume", sessionToken,
			"--prompt", prompt,
			"--output-format", "text",
			"--approval-mode", "plan",
			"--skip-trust",
		)
	default:
		t.Fatalf("unsupported provider %q", provider)
	}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"CODEX_HOME="+filepath.Join(home, ".codex"),
		"CLAUDE_HOME="+filepath.Join(home, ".claude"),
		"GEMINI_CLI_HOME="+filepath.Join(home, ".gemini"),
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s resume smoke failed: %v\n%s", provider, err, string(output))
	}
	return string(output)
}
