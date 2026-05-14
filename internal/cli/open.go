package cli

import (
	"ai-session-viewer/internal/platform"
	"fmt"

	"github.com/spf13/cobra"
)

var startServer bool

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the viewer in the default browser",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Opening browser...")
		if startServer {
			fmt.Println("Starting server is not yet integrated with the open command.")
		}
		
		err := platform.OpenBrowser("http://127.0.0.1:9090")
		if err != nil {
			fmt.Printf("Failed to open browser: %v\n", err)
		}
	},
}

func init() {
	openCmd.Flags().BoolVar(&startServer, "start-server", false, "Start the server if it is not running")
	rootCmd.AddCommand(openCmd)
}
