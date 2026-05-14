package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ai-session-viewer",
	Short: "AI Session Viewer - A local web viewer for AI coding agents",
	Long: `AI Session Viewer is a lightweight, zero-token, local tool designed to 
display sessions from AI agents like Claude Code, Gemini CLI, and Codex in a browser.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Root flags can be added here
}
