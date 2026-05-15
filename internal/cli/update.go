package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	updateScriptUnixURL    = "https://raw.githubusercontent.com/moosavimaleki/ai-agent-manager/main/scripts/install-release.sh"
	updateScriptWindowsURL = "https://raw.githubusercontent.com/moosavimaleki/ai-agent-manager/main/scripts/install-release.ps1"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update AI Agent Manager from GitHub and restart it",
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUpdate(); err != nil {
			fmt.Printf("Update failed: %v\n", err)
			return
		}
		fmt.Println("Successfully updated")
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate() error {
	startup := "hook"
	if isServiceInstalled() {
		startup = "service"
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binDir := filepath.Dir(exe)

	scriptPath, err := downloadUpdateScript()
	if err != nil {
		return err
	}
	defer os.Remove(scriptPath)

	if err := executeUpdateScript(scriptPath, startup, binDir); err != nil {
		return err
	}
	return restartActiveMode()
}

func downloadUpdateScript() (string, error) {
	url := updateScriptUnixURL
	suffix := ".sh"
	if runtime.GOOS == "windows" {
		url = updateScriptWindowsURL
		suffix = ".ps1"
	}
	if override := os.Getenv("AI_AGENT_MANAGER_UPDATE_SCRIPT_URL"); override != "" {
		url = override
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}

	file, err := os.CreateTemp("", "ai-agent-manager-update-*"+suffix)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(file.Name(), 0o755); err != nil {
			return "", err
		}
	}
	return file.Name(), nil
}

func executeUpdateScript(scriptPath, startup, binDir string) error {
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-Startup", startup, "-BinDir", binDir)
	} else {
		command = exec.Command("sh", scriptPath, "--startup", startup, "--bin-dir", binDir)
	}
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}
