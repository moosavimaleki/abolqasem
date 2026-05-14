package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the server and hook",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("AI Session Viewer Status:")
		// TODO: Implement status logic (check server, check config, read state)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
