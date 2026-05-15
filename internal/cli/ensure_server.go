package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var ensureServerCmd = &cobra.Command{
	Use:     "__ensure-server",
	Aliases: []string{"ensure-server"},
	Hidden:  true,
	Short:   "Internal: start the local server if it is not already running",
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureServerRunningForHook(5 * time.Second); err != nil {
			fmt.Printf("Failed to ensure server: %v\n", err)
			return
		}
		fmt.Printf("Server ready at %s\n", currentBaseURL())
	},
}

func init() {
	rootCmd.AddCommand(ensureServerCmd)
}
