package cli

import (
	"ai-agent-manager/internal/appinfo"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the " + appinfo.DisplayName + " service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := restartActiveMode(); err != nil {
			return fmt.Errorf("restart failed: %w", err)
		}
		fmt.Println("Successfully restarted")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}

func restartActiveMode() error {
	if !isServiceInstalled() {
		return fmt.Errorf("service is not installed; run %s install", appinfo.Name)
	}
	if err := restartService(); err != nil {
		return err
	}
	if !waitForServer(10 * time.Second) {
		return fmt.Errorf("service did not become healthy at %s", currentBaseURL())
	}
	return nil
}
