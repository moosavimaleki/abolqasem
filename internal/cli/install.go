package cli

import (
	"ai-agent-manager/internal/adapters"
	"ai-agent-manager/internal/adapters/claude"
	"ai-agent-manager/internal/adapters/codex"
	"ai-agent-manager/internal/adapters/gemini"
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
			successful := []string{}
			for _, a := range []string{"codex", "claude", "gemini"} {
				adapter, _ := getAdapter(a)
				fmt.Printf("Installing %s hook...\n", a)
				if err := adapter.InstallHook(scope); err != nil {
					fmt.Printf("Failed for %s: %v\n", a, err)
				} else {
					fmt.Printf("Successfully installed %s hook\n", a)
					successful = append(successful, a)
				}
			}
			printTrustNotice(successful)
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
			printTrustNotice([]string{installAgent})
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

func printTrustNotice(agents []string) {
	if len(agents) == 0 {
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
		case "gemini":
			names = append(names, "Gemini CLI")
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
	return names[0] + ", " + names[1] + ", and " + names[2]
}
