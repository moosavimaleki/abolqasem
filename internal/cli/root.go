package cli

import (
	"abolqasem/internal/appinfo"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   appinfo.Name,
	Short: appinfo.DisplayName + " - A local web viewer for AI coding agents",
	Long: appinfo.DisplayName + ` is a lightweight, zero-token, local tool designed to
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
