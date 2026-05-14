package cli

import (
	"ai-agent-manager/internal/state"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

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

		adapter, err := getAdapter(strings.ToLower(hookAgent))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			emitGeminiAck(hookAgent)
			os.Exit(0)
		}

		event, err := adapter.NormalizeHookInput(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing hook input: %v\n", err)
			emitGeminiAck(hookAgent)
			os.Exit(0)
		}
		event = state.NormalizeAndValidateEvent(event)

		payload, _ := json.Marshal(event)
		client := &http.Client{Timeout: 750 * time.Millisecond}
		resp, err := client.Post(state.LoadServerBaseURL()+"/api/hook", "application/json", bytes.NewBuffer(payload))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				emitGeminiAck(hookAgent)
				return
			}
		}

		if err := state.SavePendingEvent(event); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving pending event: %v\n", err)
		}

		emitGeminiAck(hookAgent)
	},
}

func init() {
	hookCmd.Flags().StringVar(&hookAgent, "agent", "codex", "Agent type (codex, claude, gemini)")
	rootCmd.AddCommand(hookCmd)
}

func emitGeminiAck(agent string) {
	if strings.EqualFold(agent, "gemini") {
		fmt.Println("{}")
	}
}
