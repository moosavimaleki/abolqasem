package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the persistent background server",
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the persistent background server",
	Run: func(cmd *cobra.Command, args []string) {
		if !isServiceInstalled() {
			fmt.Println("Service is not installed. Run: ai-agent-manager install")
			return
		}
		if err := restartService(); err != nil {
			fmt.Printf("Service restart failed: %v\n", err)
			return
		}
		fmt.Println("Successfully restarted service")
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the persistent background server",
	Run: func(cmd *cobra.Command, args []string) {
		if !isServiceInstalled() {
			fmt.Println("Service is not installed.")
			return
		}
		if err := stopService(); err != nil {
			fmt.Printf("Service stop failed: %v\n", err)
			return
		}
		fmt.Println("Successfully stopped service")
	},
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the persistent background server",
	Run: func(cmd *cobra.Command, args []string) {
		if !isServiceInstalled() {
			fmt.Println("Service is not installed. Run: ai-agent-manager install")
			return
		}
		if err := startService(); err != nil {
			fmt.Printf("Service start failed: %v\n", err)
			return
		}
		fmt.Println("Successfully started service")
	},
}

func init() {
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	rootCmd.AddCommand(serviceCmd)
}
