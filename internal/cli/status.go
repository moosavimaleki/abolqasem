package cli

import (
	"abolqasem/internal/adapters"
	"abolqasem/internal/appinfo"
	"abolqasem/internal/state"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the server and hook",
	Run: func(cmd *cobra.Command, args []string) {
		appState, err := state.LoadState()
		if err != nil {
			fmt.Printf("State: error (%v)\n", err)
			return
		}

		fmt.Println(appinfo.DisplayName)
		if serverHealthy() {
			fmt.Printf("Server: running (%s)\n", currentBaseURL())
		} else {
			fmt.Printf("Server: stopped (%s)\n", currentBaseURL())
		}
		if isServiceInstalled() {
			fmt.Println("Service: installed")
			outLog, errLog := serviceLogPaths()
			fmt.Printf("Server stdout log: %s\n", outLog)
			fmt.Printf("Server stderr log: %s\n", errLog)
		} else {
			fmt.Println("Service: not installed")
		}
		fmt.Printf("State dir: %s\n", state.GetStateDir())
		fmt.Printf("Sessions: %d\n", len(appState.Sessions))
		if latest, ok := appState.Sessions[appState.LatestSessionKey]; ok {
			fmt.Printf("Latest session: %s/%s (%s ago)\n", latest.Agent, latest.ProjectName, time.Since(latest.UpdatedAt).Round(time.Second))
		}
		for _, agent := range []string{"codex", "claude", "gemini"} {
			adapter, adapterErr := getAdapter(agent)
			if adapterErr != nil {
				continue
			}
			installed, hookErr := adapter.IsHookInstalled(adapters.ScopeUser)
			if hookErr != nil {
				fmt.Printf("Hook %s: error (%v)\n", agent, hookErr)
				continue
			}
			if installed {
				fmt.Printf("Hook %s: installed\n", agent)
			} else {
				fmt.Printf("Hook %s: not installed\n", agent)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
