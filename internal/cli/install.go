package cli

import (
	"abolqasem/internal/adapters"
	"abolqasem/internal/adapters/claude"
	"abolqasem/internal/adapters/codex"
	"abolqasem/internal/appinfo"
	"abolqasem/internal/state"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var supportedInstallAgents = []string{"codex", "claude"}

var (
	installPersistentService = installService
	restartInstalledService  = restartService
	waitForInstalledServer   = waitForServer
	installAgentHook         = func(agent string) error {
		adapter, err := getAdapter(agent)
		if err != nil {
			return err
		}
		return adapter.InstallHook(adapters.ScopeUser)
	}
	uninstallAgentHook = func(agent string) error {
		adapter, err := getAdapter(agent)
		if err != nil {
			return err
		}
		return adapter.UninstallHook(adapters.ScopeUser)
	}
	isPersistentServiceInstalled = isServiceInstalled
	uninstallPersistentService   = uninstallService
)

func getAdapter(agent string) (adapters.AgentAdapter, error) {
	switch agent {
	case "codex":
		return codex.New(), nil
	case "claude":
		return claude.New(), nil
	default:
		return nil, fmt.Errorf("unknown agent: %s", agent)
	}
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install " + appinfo.DisplayName + " service and agent hooks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		successful, err := installDesiredState()
		if err != nil {
			return fmt.Errorf("installation failed: %w", err)
		}
		fmt.Println("Installation complete")
		printTrustNotice(successful)
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall " + appinfo.DisplayName + " service and agent hooks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := uninstallDesiredState(); err != nil {
			return fmt.Errorf("uninstallation failed: %w", err)
		}
		fmt.Println("Uninstallation complete")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
}

func installDesiredState() ([]string, error) {
	fmt.Println("Installing persistent service...")
	if err := installPersistentService(); err != nil {
		return nil, fmt.Errorf("install service: %w", err)
	}
	fmt.Println("Persistent service installed")

	successful := make([]string, 0, len(supportedInstallAgents))
	var hookErrors []error
	for _, agent := range supportedInstallAgents {
		fmt.Printf("Installing %s hook...\n", agent)
		if err := installAgentHook(agent); err != nil {
			hookErrors = append(hookErrors, fmt.Errorf("install %s hook: %w", agent, err))
			continue
		}
		fmt.Printf("%s hook installed\n", agent)
		successful = append(successful, agent)
	}
	if len(hookErrors) > 0 {
		return successful, errors.Join(hookErrors...)
	}

	targetBaseURL := currentBaseURL()
	if port, ok := configuredServiceInstallPort(); ok {
		targetBaseURL = state.DefaultBaseURL(port)
		if err := state.SaveServerBaseURL(targetBaseURL); err != nil {
			return successful, fmt.Errorf("save service base URL: %w", err)
		}
	}

	fmt.Println("Restarting persistent service...")
	if err := restartInstalledService(); err != nil {
		return successful, fmt.Errorf("restart service: %w", err)
	}
	if !waitForInstalledServer(10 * time.Second) {
		return successful, fmt.Errorf("service did not become healthy at %s", targetBaseURL)
	}
	fmt.Printf("Service is running at %s\n", targetBaseURL)
	return successful, nil
}

func uninstallDesiredState() error {
	var uninstallErrors []error
	for _, agent := range supportedInstallAgents {
		fmt.Printf("Removing %s hook...\n", agent)
		if err := uninstallAgentHook(agent); err != nil {
			if !strings.Contains(err.Error(), "hook not found") {
				uninstallErrors = append(uninstallErrors, fmt.Errorf("remove %s hook: %w", agent, err))
			}
			continue
		}
		fmt.Printf("%s hook removed\n", agent)
	}

	if isPersistentServiceInstalled() {
		fmt.Println("Removing persistent service...")
		if err := uninstallPersistentService(); err != nil {
			uninstallErrors = append(uninstallErrors, fmt.Errorf("remove service: %w", err))
		} else {
			fmt.Println("Persistent service removed")
		}
	}
	return errors.Join(uninstallErrors...)
}

func printTrustNotice(agents []string) {
	if len(agents) == 0 {
		return
	}
	if os.Getenv(appinfo.EnvPrefix+"_SUPPRESS_TRUST_NOTICE") == "1" || os.Getenv(appinfo.LegacyEnvPrefix+"_SUPPRESS_TRUST_NOTICE") == "1" {
		return
	}
	fmt.Println("")
	fmt.Println("Next step:")
	fmt.Printf("  Open %s once.\n", humanAgentList(agents))
	fmt.Println("  If the agent asks you to trust or approve the installed hook command, accept it.")
	fmt.Println("  No manual config editing should be needed.")
}

func humanAgentList(agents []string) string {
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		switch agent {
		case "codex":
			names = append(names, "Codex")
		case "claude":
			names = append(names, "Claude Code")
		default:
			names = append(names, agent)
		}
	}
	if len(names) == 1 {
		return names[0]
	}
	if len(names) == 2 {
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}
