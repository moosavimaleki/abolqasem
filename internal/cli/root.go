package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ai-agent-manager",
	Short: "AI Agent Manager - A local web viewer for AI coding agents",
	Long: `AI Agent Manager is a lightweight, zero-token, local tool designed to 
display sessions from AI agents like Claude Code, Gemini CLI, and Codex in a browser.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
