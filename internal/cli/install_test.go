package cli

import (
	"ai-agent-manager/internal/adapters"
	"errors"
	"strings"
	"testing"
)

func TestRestartAfterInstallInvokesActiveMode(t *testing.T) {
	original := installRestartActiveMode
	t.Cleanup(func() {
		installRestartActiveMode = original
	})

	called := false
	installRestartActiveMode = func() error {
		called = true
		return nil
	}

	if err := restartAfterInstall(); err != nil {
		t.Fatalf("restartAfterInstall() error = %v", err)
	}
	if !called {
		t.Fatal("restartAfterInstall() did not restart the active startup mode")
	}
}

func TestRestartAfterInstallReturnsRestartError(t *testing.T) {
	original := installRestartActiveMode
	t.Cleanup(func() {
		installRestartActiveMode = original
	})

	wantErr := errors.New("restart failed")
	installRestartActiveMode = func() error {
		return wantErr
	}

	if err := restartAfterInstall(); !errors.Is(err, wantErr) {
		t.Fatalf("restartAfterInstall() error = %v, want %v", err, wantErr)
	}
}

func TestResolveInstallStartupAcceptsExplicitModes(t *testing.T) {
	original := installStartup
	t.Cleanup(func() {
		installStartup = original
	})

	for _, mode := range []string{"hook", "service"} {
		t.Run(mode, func(t *testing.T) {
			installStartup = mode

			got, err := resolveInstallStartup(strings.NewReader(""), adapters.ScopeUser, []string{"codex"}, false)
			if err != nil {
				t.Fatalf("resolveInstallStartup() error = %v", err)
			}
			if got != mode {
				t.Fatalf("resolveInstallStartup() = %q, want %q", got, mode)
			}
		})
	}
}

func TestResolveInstallStartupRejectsInvalidExplicitMode(t *testing.T) {
	original := installStartup
	t.Cleanup(func() {
		installStartup = original
	})

	installStartup = "invalid"

	_, err := resolveInstallStartup(strings.NewReader(""), adapters.ScopeUser, []string{"codex"}, false)
	if err == nil {
		t.Fatal("resolveInstallStartup() error = nil, want invalid startup error")
	}
}
