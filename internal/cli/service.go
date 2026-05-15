package cli

import (
	"ai-agent-manager/internal/state"
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
	serviceCommandUse = "__server --auto-port"
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

func restartService() error {
	switch runtime.GOOS {
	case "linux":
		return restartSystemdUserService()
	case "darwin":
		if err := uninstallLaunchAgent(); err != nil {
			return err
		}
		return installService()
	case "windows":
		_ = runCommand("schtasks", "/End", "/TN", windowsTaskName)
		return runCommand("schtasks", "/Run", "/TN", windowsTaskName)
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func stopService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "--user", "stop", serviceName+".service")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runCommand("launchctl", "unload", filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"))
	case "windows":
		return runCommand("schtasks", "/End", "/TN", windowsTaskName)
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func startService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "--user", "start", serviceName+".service")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runCommand("launchctl", "load", "-w", filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"))
	case "windows":
		return runCommand("schtasks", "/Run", "/TN", windowsTaskName)
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func serviceStatus() (string, error) {
	switch runtime.GOOS {
	case "linux":
		status, statusErr := commandOutput("systemctl", "--user", "status", serviceName+".service", "--no-pager")
		logs, logsErr := commandOutput("journalctl", "--user", "-u", serviceName+".service", "-n", "80", "--no-pager")
		if statusErr != nil && logsErr != nil {
			return "", statusErr
		}
		if logsErr != nil {
			return status, nil
		}
		if statusErr != nil {
			return logs, nil
		}
		return status + "\n\nRecent logs:\n" + logs, nil
	case "darwin":
		uid := fmt.Sprintf("%d", os.Getuid())
		status, statusErr := commandOutput("launchctl", "print", "gui/"+uid+"/"+launchAgentLabel)
		return appendServiceLogFiles(status, statusErr)
	case "windows":
		status, statusErr := commandOutput("schtasks", "/Query", "/TN", windowsTaskName, "/V", "/FO", "LIST")
		return appendServiceLogFiles(status, statusErr)
	default:
		return "", fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func requireServiceInstalled() bool {
	if isServiceInstalled() {
		return true
	}
	fmt.Println("Startup mode is hook. Service commands are not needed.")
	return false
}

func serviceLogPaths() (string, string) {
	dir := state.GetStateDir()
	return filepath.Join(dir, "service.log"), filepath.Join(dir, "service.err.log")
}

func appendServiceLogFiles(status string, statusErr error) (string, error) {
	outLog, errLog := serviceLogPaths()
	sections := []string{}
	if strings.TrimSpace(status) != "" {
		sections = append(sections, status)
	}
	if text := readRecentFile(outLog, 120); text != "" {
		sections = append(sections, "Recent stdout:\n"+text)
	}
	if text := readRecentFile(errLog, 120); text != "" {
		sections = append(sections, "Recent stderr:\n"+text)
	}
	return strings.Join(sections, "\n\n"), statusErr
}

func readRecentFile(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
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
ExecStart=%s __server --auto-port
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

func restartSystemdUserService() error {
	if err := runCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return runCommand("systemctl", "--user", "restart", serviceName+".service")
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
	outLog, errLog := serviceLogPaths()
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
    <string>__server</string>
    <string>--auto-port</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, launchAgentLabel, xmlEscape(exe), xmlEscape(outLog), xmlEscape(errLog))
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
	outLog, errLog := serviceLogPaths()
	taskCommand := fmt.Sprintf(`cmd /c ""%s" %s >> "%s" 2>> "%s""`, exe, serviceCommandUse, outLog, errLog)
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

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
	}
	return text, nil
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
