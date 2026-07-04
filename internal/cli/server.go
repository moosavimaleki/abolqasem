package cli

import (
	viewer "ai-agent-manager"
	"ai-agent-manager/internal/server"
	"ai-agent-manager/internal/state"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var port int
var autoPort bool

var serverCmd = &cobra.Command{
	Use:     "__server",
	Aliases: []string{"server"},
	Hidden:  true,
	Short:   "Internal: start the local HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		listener, actualPort, err := serverListener()
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}

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
		if err := state.SaveServerRuntime((&url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:" + strconv.Itoa(actualPort),
		}).String(), os.Getpid()); err != nil {
			log.Printf("Warning: failed to persist server URL: %v", err)
		}

		server.SetWebFS(viewer.WebAssets)
		go server.DiscoverSessionsOnce()
		server.StartDiscoveryLoop(90 * time.Second)
		if err := server.Serve(listener); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	},
}

func init() {
	serverCmd.Flags().IntVarP(&port, "port", "p", 9092, "Port to listen on")
	serverCmd.Flags().BoolVar(&autoPort, "auto-port", false, "Listen on the first available port starting at 9092")
	rootCmd.AddCommand(serverCmd)
}

func serverListener() (net.Listener, int, error) {
	if autoPort {
		for {
			if baseURL, info, ok := discoverRunningServerInfo(); ok {
				_ = state.SaveServerRuntime(baseURL, info.PID)
				log.Printf("Server already healthy at %s; waiting before taking over", baseURL)
				time.Sleep(30 * time.Second)
				continue
			}
			return listenOnAvailablePort()
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	return listener, port, err
}
