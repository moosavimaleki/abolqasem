package cli

import (
	"ai-session-viewer/internal/platform"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var startServer bool

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the viewer in the default browser",
	Run: func(cmd *cobra.Command, args []string) {
		baseURL := currentBaseURL()
		if !serverHealthy() {
			if !startServer {
				fmt.Printf("Viewer server is not running at %s\n", baseURL)
				return
			}
			if err := startServerInBackground(configuredPort()); err != nil {
				fmt.Printf("Failed to start server: %v\n", err)
				return
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if serverHealthy() {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			if !serverHealthy() {
				fmt.Printf("Server did not become healthy at %s\n", currentBaseURL())
				return
			}
		}

		err := platform.OpenBrowser(currentBaseURL())
		if err != nil {
			fmt.Printf("Failed to open browser: %v\n", err)
		}
	},
}

func init() {
	openCmd.Flags().BoolVar(&startServer, "start-server", false, "Start the server if it is not running")
	rootCmd.AddCommand(openCmd)
}
