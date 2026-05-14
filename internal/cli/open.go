package cli

import (
	"ai-agent-manager/internal/platform"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var startServer bool

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the viewer in the default browser",
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureServerRunning(5 * time.Second); err != nil {
			fmt.Printf("Failed to start server: %v\n", err)
			return
		}

		err := platform.OpenBrowser(currentBaseURL())
		if err != nil {
			fmt.Printf("Failed to open browser: %v\n", err)
		}
	},
}

func init() {
	openCmd.Flags().BoolVar(&startServer, "start-server", false, "Deprecated: the server starts automatically when needed")
	rootCmd.AddCommand(openCmd)
}
