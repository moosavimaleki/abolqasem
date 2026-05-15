package cli

import (
	"ai-agent-manager/internal/adapters"
	"ai-agent-manager/internal/adapters/claude"
	"ai-agent-manager/internal/adapters/codex"
	"ai-agent-manager/internal/adapters/gemini"
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var installAgent string
var installScope string
var installAll bool
var installStartup string
var installNoHooks bool

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
	Short: "Install AI Agent Manager startup and hooks",
	Run: func(cmd *cobra.Command, args []string) {
		scope := adapters.InstallScope(installScope)
		if scope != adapters.ScopeUser && scope != adapters.ScopeProject {
			fmt.Println("Invalid scope. Use 'user' or 'project'")
			return
		}

		agents := selectedInstallAgents()
		if len(agents) == 0 && !installNoHooks {
			fmt.Println("No agents selected")
			return
		}

		serviceInstalled := isServiceInstalled()
		startup, err := resolveInstallStartup(cmd.InOrStdin(), scope, agents, serviceInstalled)
		if err != nil {
			fmt.Printf("Installation failed: %v\n", err)
			return
		}
		if startup == "" {
			return
		}

		if startup == "service" {
			if err := installService(); err != nil {
				fmt.Printf("Service installation failed: %v\n", err)
				return
			}
			fmt.Println("Successfully installed service")
		} else if serviceInstalled {
			if err := uninstallService(); err != nil {
				fmt.Printf("Service removal failed: %v\n", err)
				return
			}
			fmt.Println("Successfully removed service")
		}

		if installNoHooks {
			return
		}
		successful := installHooks(scope, agents)
		printTrustNotice(successful)
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall AI Agent Manager startup and hooks",
	Run: func(cmd *cobra.Command, args []string) {
		scope := adapters.InstallScope(installScope)
		agents := selectedInstallAgents()
		for _, agent := range agents {
			adapter, _ := getAdapter(agent)
			if err := adapter.UninstallHook(scope); err != nil {
				if !strings.Contains(err.Error(), "hook not found") {
					fmt.Printf("Failed to uninstall %s hook: %v\n", agent, err)
				}
			} else {
				fmt.Printf("Successfully uninstalled %s hook\n", agent)
			}
		}

		if isServiceInstalled() {
			if err := uninstallService(); err != nil {
				fmt.Printf("Service uninstallation failed: %v\n", err)
				return
			}
			fmt.Println("Successfully uninstalled service")
		}
	},
}

func init() {
	installCmd.Flags().StringVar(&installAgent, "agent", "codex", "Agent type (codex, claude, gemini)")
	installCmd.Flags().StringVar(&installScope, "scope", "user", "Installation scope (user, project)")
	installCmd.Flags().BoolVar(&installAll, "all", false, "Install for all supported agents")
	installCmd.Flags().StringVar(&installStartup, "startup", "", "Server startup mode: hook or service. Omit for interactive setup")
	installCmd.Flags().BoolVar(&installNoHooks, "no-hooks", false, "Install startup only and do not change agent hooks")

	uninstallCmd.Flags().StringVar(&installAgent, "agent", "codex", "Agent type (codex, claude, gemini)")
	uninstallCmd.Flags().StringVar(&installScope, "scope", "user", "Installation scope (user, project)")
	uninstallCmd.Flags().BoolVar(&installAll, "all", false, "Uninstall for all supported agents")

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
}

func selectedInstallAgents() []string {
	if installAll {
		return []string{"codex", "claude", "gemini"}
	}
	if _, err := getAdapter(installAgent); err != nil {
		return nil
	}
	return []string{installAgent}
}

func installHooks(scope adapters.InstallScope, agents []string) []string {
	successful := []string{}
	for _, agent := range agents {
		adapter, _ := getAdapter(agent)
		fmt.Printf("Installing %s hook...\n", agent)
		if err := adapter.InstallHook(scope); err != nil {
			fmt.Printf("Failed for %s: %v\n", agent, err)
			continue
		}
		fmt.Printf("Successfully installed %s hook\n", agent)
		successful = append(successful, agent)
	}
	return successful
}

func resolveInstallStartup(input io.Reader, scope adapters.InstallScope, agents []string, serviceInstalled bool) (string, error) {
	if installStartup != "" {
		if installStartup != "hook" && installStartup != "service" {
			return "", fmt.Errorf("invalid startup. Use 'hook' or 'service'")
		}
		return installStartup, nil
	}
	if !isInteractiveInput(input) {
		return "", fmt.Errorf("interactive install requires a terminal; pass --startup hook or --startup service")
	}

	hooksInstalled := anyHookInstalled(scope, agents)
	if serviceInstalled || hooksInstalled {
		printInstallState(serviceInstalled, hooksInstalled)
		ok, err := promptYesNo(input, "Change install mode? [y/N]: ", false)
		if err != nil || !ok {
			return "", err
		}
	}

	fmt.Println("Choose startup mode:")
	fmt.Println("  1) service - persistent background server")
	fmt.Println("  2) hook    - start server idempotently from agent hooks")
	return promptStartupMode(input)
}

func anyHookInstalled(scope adapters.InstallScope, agents []string) bool {
	for _, agent := range agents {
		adapter, _ := getAdapter(agent)
		installed, err := adapter.IsHookInstalled(scope)
		if err == nil && installed {
			return true
		}
	}
	return false
}

func describeInstallMode(serviceInstalled, hooksInstalled bool) string {
	if serviceInstalled {
		return "service"
	}
	if hooksInstalled {
		return "hook"
	}
	return "not installed"
}

func printInstallState(serviceInstalled, hooksInstalled bool) {
	fmt.Printf("Existing installation detected. Startup mode: %s\n", describeInstallMode(serviceInstalled, hooksInstalled))
	if hooksInstalled {
		fmt.Println("Agent hooks: installed")
		fmt.Println("Note: hooks record sessions; they can be used together with service startup.")
	} else {
		fmt.Println("Agent hooks: not installed")
	}
}

func promptStartupMode(input io.Reader) (string, error) {
	reader := bufio.NewReader(input)
	for {
		fmt.Print("Select 1 or 2: ")
		answer, err := reader.ReadString('\n')
		if err != nil && len(answer) == 0 {
			return "", err
		}
		switch strings.TrimSpace(strings.ToLower(answer)) {
		case "1", "service", "s":
			return "service", nil
		case "2", "hook", "h":
			return "hook", nil
		default:
			fmt.Println("Invalid choice.")
		}
	}
}

func promptYesNo(input io.Reader, prompt string, fallback bool) (bool, error) {
	reader := bufio.NewReader(input)
	fmt.Print(prompt)
	answer, err := reader.ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" {
		return fallback, nil
	}
	return answer == "y" || answer == "yes", nil
}

func isInteractiveInput(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

func printTrustNotice(agents []string) {
	if len(agents) == 0 {
		return
	}
	if os.Getenv("AI_AGENT_MANAGER_SUPPRESS_TRUST_NOTICE") == "1" {
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
