package cli

import (
	"abolqasem/internal/appinfo"
	"abolqasem/internal/state"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	serviceName      = appinfo.Name
	launchAgentLabel = appinfo.LaunchAgentLabel
	windowsTaskName  = appinfo.WindowsTaskName
)

func serviceCommandArgs() []string {
	if port, ok := configuredServiceInstallPort(); ok {
		return []string{"__server", "--port", strconv.Itoa(port)}
	}
	return []string{"__server", "--auto-port"}
}

func serviceCommandUse() string {
	return strings.Join(serviceCommandArgs(), " ")
}

func configuredServiceInstallPort() (int, bool) {
	for _, key := range []string{
		"ABOLQASEM_SERVICE_PORT",
		"ABOLQASEM_DEV_PORT",
		"AI_AGENT_MANAGER_SERVICE_PORT",
		"AI_AGENT_MANAGER_DEV_PORT",
	} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 || port > 65535 {
			return 0, false
		}
		return port, true
	}
	return 0, false
}

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
		_ = uninstallLegacySystemdUserService()
		return installSystemdUserService(exe)
	case "darwin":
		_ = uninstallLegacyLaunchAgent()
		return installLaunchAgent(exe)
	case "windows":
		_ = uninstallLegacyScheduledTask()
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
		if err == nil {
			return true
		}
		_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", appinfo.LegacyName+".service"))
		return err == nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"))
		if err == nil {
			return true
		}
		_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents", appinfo.LegacyLaunchAgentLabel+".plist"))
		return err == nil
	case "windows":
		return exec.Command("schtasks", "/Query", "/TN", windowsTaskName).Run() == nil ||
			exec.Command("schtasks", "/Query", "/TN", appinfo.LegacyWindowsTaskName).Run() == nil
	default:
		return false
	}
}

func uninstallService() error {
	switch runtime.GOOS {
	case "linux":
		if err := uninstallSystemdUserService(); err != nil {
			return err
		}
		return uninstallLegacySystemdUserService()
	case "darwin":
		if err := uninstallLaunchAgent(); err != nil {
			return err
		}
		return uninstallLegacyLaunchAgent()
	case "windows":
		err := uninstallScheduledTask()
		legacyErr := uninstallLegacyScheduledTask()
		if err != nil && legacyErr != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func restartService() error {
	switch runtime.GOOS {
	case "linux":
		return restartSystemdUserService()
	case "darwin":
		if err := uninstallLaunchAgentByLabel(currentLaunchAgentLabel()); err != nil {
			return err
		}
		return installService()
	case "windows":
		taskName := currentWindowsTaskName()
		_ = runCommand("schtasks", "/End", "/TN", taskName)
		return runCommand("schtasks", "/Run", "/TN", taskName)
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func stopService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "--user", "stop", currentSystemdServiceName()+".service")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runCommand("launchctl", "unload", filepath.Join(home, "Library", "LaunchAgents", currentLaunchAgentLabel()+".plist"))
	case "windows":
		return runCommand("schtasks", "/End", "/TN", currentWindowsTaskName())
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func startService() error {
	switch runtime.GOOS {
	case "linux":
		return runCommand("systemctl", "--user", "start", currentSystemdServiceName()+".service")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return runCommand("launchctl", "load", "-w", filepath.Join(home, "Library", "LaunchAgents", currentLaunchAgentLabel()+".plist"))
	case "windows":
		return runCommand("schtasks", "/Run", "/TN", currentWindowsTaskName())
	default:
		return fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func serviceStatus() (string, error) {
	switch runtime.GOOS {
	case "linux":
		name := currentSystemdServiceName()
		status, statusErr := commandOutput("systemctl", "--user", "status", name+".service", "--no-pager")
		logs, logsErr := commandOutput("journalctl", "--user", "-u", name+".service", "-n", "80", "--no-pager")
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
		status, statusErr := commandOutput("launchctl", "print", "gui/"+uid+"/"+currentLaunchAgentLabel())
		return appendServiceLogFiles(status, statusErr)
	case "windows":
		status, statusErr := commandOutput("schtasks", "/Query", "/TN", currentWindowsTaskName(), "/V", "/FO", "LIST")
		return appendServiceLogFiles(status, statusErr)
	default:
		return "", fmt.Errorf("persistent service is not supported on %s", runtime.GOOS)
	}
}

func requireServiceInstalled() bool {
	if isServiceInstalled() {
		return true
	}
	fmt.Printf("Service is not installed. Run %s install.\n", appinfo.Name)
	return false
}

func serviceLogPaths() (string, string) {
	dir := state.GetStateDir()
	return filepath.Join(dir, "service.log"), filepath.Join(dir, "service.err.log")
}

func currentSystemdServiceName() string {
	if systemdUserServiceExists(serviceName) {
		return serviceName
	}
	if systemdUserServiceExists(appinfo.LegacyName) {
		return appinfo.LegacyName
	}
	return serviceName
}

func systemdUserServiceExists(name string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".config", "systemd", "user", name+".service"))
	return err == nil
}

func currentLaunchAgentLabel() string {
	if launchAgentExists(launchAgentLabel) {
		return launchAgentLabel
	}
	if launchAgentExists(appinfo.LegacyLaunchAgentLabel) {
		return appinfo.LegacyLaunchAgentLabel
	}
	return launchAgentLabel
}

func launchAgentExists(label string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, "Library", "LaunchAgents", label+".plist"))
	return err == nil
}

func currentWindowsTaskName() string {
	if exec.Command("schtasks", "/Query", "/TN", windowsTaskName).Run() == nil {
		return windowsTaskName
	}
	if exec.Command("schtasks", "/Query", "/TN", appinfo.LegacyWindowsTaskName).Run() == nil {
		return appinfo.LegacyWindowsTaskName
	}
	return windowsTaskName
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
Description=Abolqasem local viewer
After=network.target

[Service]
Type=simple
ExecStart=%s %s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, systemdQuote(exe), serviceCommandUse())
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

func uninstallLegacySystemdUserService() error {
	_ = runCommand("systemctl", "--user", "disable", "--now", appinfo.LegacyName+".service")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitPath := filepath.Join(home, ".config", "systemd", "user", appinfo.LegacyName+".service")
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return runCommand("systemctl", "--user", "daemon-reload")
}

func restartSystemdUserService() error {
	if err := runCommand("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return runCommand("systemctl", "--user", "restart", currentSystemdServiceName()+".service")
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
	programArguments := launchAgentProgramArguments(exe)
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s
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
`, launchAgentLabel, programArguments, xmlEscape(outLog), xmlEscape(errLog))
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = runCommand("launchctl", "unload", plistPath)
	return runCommand("launchctl", "load", "-w", plistPath)
}

func uninstallLaunchAgent() error {
	return uninstallLaunchAgentByLabel(launchAgentLabel)
}

func uninstallLegacyLaunchAgent() error {
	return uninstallLaunchAgentByLabel(appinfo.LegacyLaunchAgentLabel)
}

func uninstallLaunchAgentByLabel(label string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	_ = runCommand("launchctl", "unload", plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func installScheduledTask(exe string) error {
	outLog, errLog := serviceLogPaths()
	taskCommand := fmt.Sprintf(`cmd /c ""%s" %s >> "%s" 2>> "%s""`, exe, serviceCommandUse(), outLog, errLog)
	if err := runCommand("schtasks", "/Create", "/TN", windowsTaskName, "/SC", "ONLOGON", "/TR", taskCommand, "/F"); err != nil {
		return err
	}
	_ = runCommand("schtasks", "/Run", "/TN", windowsTaskName)
	return nil
}

func uninstallScheduledTask() error {
	return runCommand("schtasks", "/Delete", "/TN", windowsTaskName, "/F")
}

func uninstallLegacyScheduledTask() error {
	return runCommand("schtasks", "/Delete", "/TN", appinfo.LegacyWindowsTaskName, "/F")
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

func launchAgentProgramArguments(exe string) string {
	args := append([]string{exe}, serviceCommandArgs()...)
	lines := make([]string, 0, len(args))
	for _, arg := range args {
		lines = append(lines, "    <string>"+xmlEscape(arg)+"</string>")
	}
	return strings.Join(lines, "\n")
}

func xmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return strings.ReplaceAll(value, "'", "&apos;")
}
