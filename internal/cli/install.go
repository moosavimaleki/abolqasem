package cli

import (
	"ai-session-viewer/internal/adapters"
	"ai-session-viewer/internal/adapters/claude"
	"ai-session-viewer/internal/adapters/codex"
	"ai-session-viewer/internal/adapters/gemini"
	"fmt"

	"github.com/spf13/cobra"
)

var installAgent string
var installScope string
var installAll bool

func getAdapter(agent string) (adapters.AgentAdapter, error) {
	switch agent {
	case "codex":
		return codex.New(), nil
	case "claude":
		return claude.New(), nil
	case "gemini":
		return gemini.New(), nil
	default:
		return nil, fmt.Errorf("unknown agent: %s", agent)
	}
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the hook into AI agent configuration",
	Run: func(cmd *cobra.Command, args []string) {
		scope := adapters.InstallScope(installScope)
		if scope != adapters.ScopeUser && scope != adapters.ScopeProject {
			fmt.Println("Invalid scope. Use 'user' or 'project'")
			return
		}

		if installAll {
			for _, a := range []string{"codex", "claude", "gemini"} {
				adapter, _ := getAdapter(a)
				fmt.Printf("Installing %s hook...\n", a)
				if err := adapter.InstallHook(scope); err != nil {
					fmt.Printf("Failed for %s: %v\n", a, err)
				} else {
					fmt.Printf("Successfully installed %s hook\n", a)
				}
			}
			return
		}

		adapter, err := getAdapter(installAgent)
		if err != nil {
			fmt.Println(err)
			return
		}

		if err := adapter.InstallHook(scope); err != nil {
			fmt.Printf("Installation failed: %v\n", err)
		} else {
			fmt.Println("Successfully installed hook")
		}
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the hook from AI agent configuration",
	Run: func(cmd *cobra.Command, args []string) {
		scope := adapters.InstallScope(installScope)
		adapter, err := getAdapter(installAgent)
		if err != nil {
			fmt.Println(err)
			return
		}

		if err := adapter.UninstallHook(scope); err != nil {
			fmt.Printf("Uninstallation failed: %v\n", err)
		} else {
			fmt.Println("Successfully uninstalled hook")
		}
	},
}

func init() {
	installCmd.Flags().StringVar(&installAgent, "agent", "codex", "Agent type (codex, claude, gemini)")
	installCmd.Flags().StringVar(&installScope, "scope", "user", "Installation scope (user, project)")
	installCmd.Flags().BoolVar(&installAll, "all", false, "Install for all supported agents")
	
	uninstallCmd.Flags().StringVar(&installAgent, "agent", "codex", "Agent type (codex, claude, gemini)")
	uninstallCmd.Flags().StringVar(&installScope, "scope", "user", "Installation scope (user, project)")

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
}
