package cli

import (
	"ai-agent-manager/internal/appinfo"
	"ai-agent-manager/internal/state"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart " + appinfo.DisplayName + " in the active startup mode",
	Run: func(cmd *cobra.Command, args []string) {
		if err := restartActiveMode(); err != nil {
			fmt.Printf("Restart failed: %v\n", err)
			return
		}
		fmt.Println("Successfully restarted")
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}

func restartActiveMode() error {
	if isServiceInstalled() {
		return restartService()
	}
	if err := stopHookServer(); err != nil {
		return err
	}
	return ensureServerRunning(5 * time.Second)
}

func stopHookServer() error {
	baseURL, info, ok := discoverRunningServerInfo()
	if !ok {
		return nil
	}
	pid := info.PID
	if pid <= 0 {
		pid = statePIDFallback()
	}
	if pid <= 0 {
		return fmt.Errorf("server is running at %s but its process id is unknown", baseURL)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := terminateProcess(process); err != nil {
		return err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !serverHealthyAt(baseURL) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("server at %s did not stop", baseURL)
}

func statePIDFallback() int {
	return state.LoadServerPID()
}
