package cli

import (
	viewer "ai-session-viewer"
	"ai-session-viewer/internal/server"
	"ai-session-viewer/internal/state"
	"log"

	"github.com/spf13/cobra"
)

var port int

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the local HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		appState, err := state.LoadState()
		if err != nil {
			log.Fatalf("Failed to load state: %v", err)
		}

		if err := state.ProcessPendingEvents(appState); err != nil {
			log.Printf("Warning: Failed to process pending events: %v", err)
		}

		server.SetWebFS(viewer.WebAssets)
		if err := server.Start(port); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	},
}

func init() {
	serverCmd.Flags().IntVarP(&port, "port", "p", 9090, "Port to listen on")
	rootCmd.AddCommand(serverCmd)
}
