package cli

import (
	"ai-agent-manager/internal/platform"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the viewer in the default browser",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureServiceRunning(10 * time.Second); err != nil {
			return fmt.Errorf("start service: %w", err)
		}
		if err := platform.OpenBrowser(currentBaseURL()); err != nil {
			return fmt.Errorf("open browser: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(openCmd)
}
