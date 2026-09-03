// Package codexutil contains small parsers shared by Codex transcript readers.
package codexutil

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var execCommandFieldPattern = regexp.MustCompile(`(?:^|[,\{]\s*)["']?cmd["']?\s*:\s*("(?:\\.|[^"\\])*")`)
var execWorkdirFieldPattern = regexp.MustCompile(`(?:^|[,\{]\s*)["']?workdir["']?\s*:\s*("(?:\\.|[^"\\])*")`)
var exitCodePattern = regexp.MustCompile(`(?i)(?:exited with code|exit(?:ed)? code\s*[:=]?)\s*(-?[0-9]+)`)

// ExtractExecCommand returns the commands and common working directory encoded
// in one or more tools.exec_command calls from a Codex transcript item.
func ExtractExecCommand(input string) (string, string, bool) {
	const marker = "tools.exec_command("
	commands := make([]string, 0, 1)
	workdirs := make([]string, 0, 1)
	for remaining := input; ; {
		markerIndex := strings.Index(remaining, marker)
		if markerIndex < 0 {
			break
		}
		remaining = remaining[markerIndex+len(marker):]
		segment := remaining
		if nextIndex := strings.Index(segment, marker); nextIndex >= 0 {
			segment = segment[:nextIndex]
		}
		command := extractJSONStringField(segment, execCommandFieldPattern)
		if strings.TrimSpace(command) != "" {
			commands = append(commands, command)
			workdirs = append(workdirs, extractJSONStringField(segment, execWorkdirFieldPattern))
		}
	}
	if len(commands) == 0 {
		return "", "", false
	}
	cwd := workdirs[0]
	for _, workdir := range workdirs[1:] {
		if workdir != cwd {
			cwd = ""
			break
		}
	}
	return strings.Join(commands, "\n"), cwd, true
}

func extractJSONStringField(input string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(input)
	if len(match) != 2 {
		return ""
	}
	var value string
	if err := json.Unmarshal([]byte(match[1]), &value); err != nil {
		return ""
	}
	return value
}

// CommandCompletion derives a stable command status from a Codex tool output.
func CommandCompletion(output string) (string, int, bool) {
	if match := exitCodePattern.FindStringSubmatch(output); len(match) == 2 {
		if exitCode, err := strconv.Atoi(match[1]); err == nil {
			if exitCode == 0 {
				return "completed", exitCode, true
			}
			return "failed", exitCode, true
		}
	}
	if strings.Contains(output, "Script completed") {
		return "completed", 0, true
	}
	return "completed", 0, false
}
