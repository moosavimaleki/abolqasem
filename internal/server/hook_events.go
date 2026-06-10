package server

import (
	"ai-agent-manager/internal/state"
	"strings"
	"unicode"
)

func isResponseCompleteHookEvent(event state.HookEvent) bool {
	switch normalizedHookEventName(event.HookEventName) {
	case "stop", "afteragent", "turncomplete", "turncompleted", "responsecomplete", "responsecompleted":
		return true
	default:
		return false
	}
}

func normalizedHookEventName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(name))
}
