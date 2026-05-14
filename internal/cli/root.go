package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "codex-rtl",
	Short: "Codex RTL Viewer - A local web viewer for Codex sessions with RTL support",
	Long: `Codex RTL Viewer is a lightweight, zero-token, local tool designed to 
display Codex TUI sessions in a browser with proper Right-to-Left (RTL) 
rendering for Persian and other RTL languages.`,
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
