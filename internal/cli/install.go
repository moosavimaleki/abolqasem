package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const markerStart = "# BEGIN codex-rtl-viewer"
const markerEnd = "# END codex-rtl-viewer"
const hookConfig = `
[features]
codex_hooks = true

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "codex-rtl hook"
timeout = 3
`

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the hook into Codex configuration",
	Run: func(cmd *cobra.Command, args []string) {
		configPath := getConfigPath()
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			os.MkdirAll(filepath.Dir(configPath), 0755)
			os.WriteFile(configPath, []byte(""), 0644)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			fmt.Printf("Error reading config: %v\n", err)
			return
		}

		content := string(data)
		if strings.Contains(content, markerStart) {
			fmt.Println("Hook already installed.")
			return
		}

		os.WriteFile(configPath+".bak", data, 0644)

		newContent := content + "\n" + markerStart + hookConfig + markerEnd + "\n"
		os.WriteFile(configPath, []byte(newContent), 0644)
		fmt.Println("Successfully installed codex-rtl hook to config.toml")
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the hook from Codex configuration",
	Run: func(cmd *cobra.Command, args []string) {
		configPath := getConfigPath()
		data, err := os.ReadFile(configPath)
		if err != nil {
			fmt.Printf("Error reading config: %v\n", err)
			return
		}

		content := string(data)
		if !strings.Contains(content, markerStart) {
			fmt.Println("Hook not found in config.")
			return
		}

		startIdx := strings.Index(content, markerStart)
		endIdx := strings.Index(content, markerEnd) + len(markerEnd)
		
		newContent := content[:startIdx] + content[endIdx:]
		newContent = strings.ReplaceAll(newContent, "\n\n\n", "\n\n")
		
		os.WriteFile(configPath, []byte(newContent), 0644)
		fmt.Println("Successfully uninstalled codex-rtl hook from config.toml")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
}
