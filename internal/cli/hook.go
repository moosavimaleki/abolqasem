package cli

import (
	"bytes"
	"codex-rtl/internal/state"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Process a Codex hook event",
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(0) // Exit 0 to avoid crashing codex
		}

		var event state.HookEvent
		if err := json.Unmarshal(input, &event); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
			os.Exit(0)
		}

		if event.TranscriptPath == "" {
			fmt.Fprintln(os.Stderr, "Invalid event: transcript_path is empty")
			os.Exit(0)
		}

		// Fallback for session_id if missing
		if event.SessionID == "" {
			event.SessionID = filepath.Base(filepath.Dir(event.TranscriptPath))
		}

		// Try to send to local server
		payload, _ := json.Marshal(event)
		resp, err := http.Post("http://127.0.0.1:9090/api/hook", "application/json", bytes.NewBuffer(payload))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Successfully delivered
				return
			}
		}

		// Server is down or returned error, save as pending
		if err := state.SavePendingEvent(event); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving pending event: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
