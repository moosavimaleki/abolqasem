package cli

import (
	"ai-session-viewer/internal/adapters"
	"ai-session-viewer/internal/adapters/claude"
	"ai-session-viewer/internal/adapters/codex"
	"ai-session-viewer/internal/adapters/gemini"
	"ai-session-viewer/internal/state"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var hookAgent string

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Process an AI agent hook event",
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(0) // Exit 0 to avoid crashing agent
		}

		var adapter adapters.AgentAdapter
		switch hookAgent {
		case "codex":
			adapter = codex.New()
		case "claude":
			adapter = claude.New()
		case "gemini":
			adapter = gemini.New()
		default:
			fmt.Fprintf(os.Stderr, "Unknown agent: %s\n", hookAgent)
			os.Exit(0)
		}

		event, err := adapter.NormalizeHookInput(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing hook input: %v\n", err)
			os.Exit(0)
		}

		// Try to send to local server
		payload, _ := json.Marshal(event)
		resp, err := http.Post("http://127.0.0.1:9090/api/hook", "application/json", bytes.NewBuffer(payload))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// Successfully delivered
				// If Gemini, output empty JSON to stdout
				if hookAgent == "gemini" {
					fmt.Println("{}")
				}
				return
			}
		}

		// Server is down or returned error, save as pending
		if err := state.SavePendingEvent(event); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving pending event: %v\n", err)
		}

		if hookAgent == "gemini" {
			fmt.Println("{}")
		}
	},
}

func init() {
	hookCmd.Flags().StringVar(&hookAgent, "agent", "codex", "Agent type (codex, claude, gemini)")
	rootCmd.AddCommand(hookCmd)
}
