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
		if !requireServiceInstalled() {
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
		if !requireServiceInstalled() {
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
		if !requireServiceInstalled() {
			return
		}
		if err := startService(); err != nil {
			fmt.Printf("Service start failed: %v\n", err)
			return
		}
		fmt.Println("Successfully started service")
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show service status and recent logs",
	Run: func(cmd *cobra.Command, args []string) {
		if !requireServiceInstalled() {
			return
		}
		output, err := serviceStatus()
		if err != nil {
			fmt.Printf("Service status failed: %v\n", err)
			if output != "" {
				fmt.Println(output)
			}
			return
		}
		fmt.Println(output)
	},
}

func init() {
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	rootCmd.AddCommand(serviceCmd)
}
