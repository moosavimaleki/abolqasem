package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	serviceName       = "ai-agent-manager"
	launchAgentLabel  = "com.ai-agent-manager"
	windowsTaskName   = "AI Agent Manager"
	serviceCommandUse = "server --auto-port"
)

func installService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "linux":
		return installSystemdUserService(exe)
	case "darwin":
		return installLaunchAgent(exe)
	case "windows":
		return installScheduledTask(exe)
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func isServiceInstalled() bool {
	switch runtime.GOOS {
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", serviceName+".service"))
		return err == nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"))
		return err == nil
	case "windows":
		return exec.Command("schtasks", "/Query", "/TN", windowsTaskName).Run() == nil
	default:
		return false
	}
}

func uninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallSystemdUserService()
	case "darwin":
		return uninstallLaunchAgent()
	case "windows":
		return uninstallScheduledTask()
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func installSystemdUserService(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return err
	}
	unitPath := filepath.Join(unitDir, serviceName+".service")
	unit := fmt.Sprintf(`[Unit]
Description=AI Agent Manager local viewer
After=network.target

[Service]
Type=simple
ExecStart=%s server --auto-port
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, systemdQuote(exe))
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
	}
	if err := runCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return runCommand("systemctl", "--user", "enable", "--now", serviceName+".service")
}

func uninstallSystemdUserService() error {
	_ = runCommand("systemctl", "--user", "disable", "--now", serviceName+".service")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", serviceName+".service")
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return runCommand("systemctl", "--user", "daemon-reload")
}

func installLaunchAgent(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return err
	}
	plistPath := filepath.Join(agentDir, launchAgentLabel+".plist")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>server</string>
    <string>--auto-port</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
`, launchAgentLabel, xmlEscape(exe))
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = runCommand("launchctl", "unload", plistPath)
	return runCommand("launchctl", "load", "-w", plistPath)
}

func uninstallLaunchAgent() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	_ = runCommand("launchctl", "unload", plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func installScheduledTask(exe string) error {
	taskCommand := fmt.Sprintf(`"%s" %s`, exe, serviceCommandUse)
	if err := runCommand("schtasks", "/Create", "/TN", windowsTaskName, "/SC", "ONLOGON", "/TR", taskCommand, "/F"); err != nil {
		return err
	}
	_ = runCommand("schtasks", "/Run", "/TN", windowsTaskName)
	return nil
}

func uninstallScheduledTask() error {
	return runCommand("schtasks", "/Delete", "/TN", windowsTaskName, "/F")
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func systemdQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func xmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return strings.ReplaceAll(value, "'", "&apos;")
}
