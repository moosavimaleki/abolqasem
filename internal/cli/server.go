package cli

import (
	viewer "ai-agent-manager"
	"ai-agent-manager/internal/server"
	"ai-agent-manager/internal/state"
	"log"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var port int

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the local HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		appState, err := state.LoadState()
		if err != nil {
			log.Printf("Warning: failed to load state, continuing with a fresh store: %v", err)
			appState = &state.AppState{Sessions: map[string]state.SessionMeta{}}
		}

		if err := state.ProcessPendingEvents(appState); err != nil {
			log.Printf("Warning: Failed to process pending events: %v", err)
		}
		if err := state.SaveState(appState); err != nil {
			log.Printf("Warning: failed to persist migrated state: %v", err)
		}
		if err := state.SaveServerBaseURL((&url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:" + strconv.Itoa(port),
		}).String()); err != nil {
			log.Printf("Warning: failed to persist server URL: %v", err)
		}

		server.DiscoverSessionsOnce()
		server.StartDiscoveryLoop(90 * time.Second)
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
