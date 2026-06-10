package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestInstallDesiredStateInstallsServiceAllHooksAndRestarts(t *testing.T) {
	restoreInstallFunctions(t)

	var calls []string
	installPersistentService = func() error {
		calls = append(calls, "service")
		return nil
	}
	installAgentHook = func(agent string) error {
		calls = append(calls, "hook:"+agent)
		return nil
	}
	restartInstalledService = func() error {
		calls = append(calls, "restart")
		return nil
	}
	waitForInstalledServer = func(timeout time.Duration) bool {
		calls = append(calls, "health")
		return true
	}

	successful, err := installDesiredState()
	if err != nil {
		t.Fatalf("installDesiredState() error = %v", err)
	}
	if !reflect.DeepEqual(successful, supportedInstallAgents) {
		t.Fatalf("installDesiredState() agents = %v, want %v", successful, supportedInstallAgents)
	}
	wantCalls := []string{"service", "hook:codex", "hook:claude", "hook:gemini", "restart", "health"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("installDesiredState() calls = %v, want %v", calls, wantCalls)
	}
}

func TestInstallDesiredStateReportsAnyHookFailure(t *testing.T) {
	restoreInstallFunctions(t)

	var calls []string
	installPersistentService = func() error { return nil }
	installAgentHook = func(agent string) error {
		calls = append(calls, agent)
		if agent == "claude" {
			return errors.New("broken config")
		}
		return nil
	}
	restartInstalledService = func() error {
		t.Fatal("service must not restart after a partial hook install")
		return nil
	}

	successful, err := installDesiredState()
	if err == nil || !strings.Contains(err.Error(), "install claude hook") {
		t.Fatalf("installDesiredState() error = %v, want claude hook failure", err)
	}
	if !reflect.DeepEqual(calls, supportedInstallAgents) {
		t.Fatalf("installDesiredState() hook calls = %v, want %v", calls, supportedInstallAgents)
	}
	if !reflect.DeepEqual(successful, []string{"codex", "gemini"}) {
		t.Fatalf("installDesiredState() successful = %v", successful)
	}
}

func TestInstallDesiredStateRequiresHealthyService(t *testing.T) {
	restoreInstallFunctions(t)

	installPersistentService = func() error { return nil }
	installAgentHook = func(agent string) error { return nil }
	restartInstalledService = func() error { return nil }
	waitForInstalledServer = func(timeout time.Duration) bool { return false }

	_, err := installDesiredState()
	if err == nil || !strings.Contains(err.Error(), "did not become healthy") {
		t.Fatalf("installDesiredState() error = %v, want health failure", err)
	}
}

func TestUninstallDesiredStateRemovesAllHooksAndService(t *testing.T) {
	restoreInstallFunctions(t)

	var calls []string
	uninstallAgentHook = func(agent string) error {
		calls = append(calls, "hook:"+agent)
		return nil
	}
	isPersistentServiceInstalled = func() bool { return true }
	uninstallPersistentService = func() error {
		calls = append(calls, "service")
		return nil
	}

	if err := uninstallDesiredState(); err != nil {
		t.Fatalf("uninstallDesiredState() error = %v", err)
	}
	wantCalls := []string{"hook:codex", "hook:claude", "hook:gemini", "service"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("uninstallDesiredState() calls = %v, want %v", calls, wantCalls)
	}
}

func TestInstallCommandsDoNotExposeLegacySelectionFlags(t *testing.T) {
	for _, command := range []*cobra.Command{installCmd, uninstallCmd} {
		for _, flag := range []string{"agent", "all", "scope", "startup", "no-hooks"} {
			if command.Flags().Lookup(flag) != nil {
				t.Fatalf("%s unexpectedly exposes --%s", command.Name(), flag)
			}
		}
	}
}

func restoreInstallFunctions(t *testing.T) {
	t.Helper()
	originalInstallService := installPersistentService
	originalInstallHook := installAgentHook
	originalRestartService := restartInstalledService
	originalWaitForServer := waitForInstalledServer
	originalUninstallHook := uninstallAgentHook
	originalServiceInstalled := isPersistentServiceInstalled
	originalUninstallService := uninstallPersistentService
	t.Cleanup(func() {
		installPersistentService = originalInstallService
		installAgentHook = originalInstallHook
		restartInstalledService = originalRestartService
		waitForInstalledServer = originalWaitForServer
		uninstallAgentHook = originalUninstallHook
		isPersistentServiceInstalled = originalServiceInstalled
		uninstallPersistentService = originalUninstallService
	})
}
