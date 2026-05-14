package parser

import (
	"regexp"
	"strings"
)

var codeRegex = regexp.MustCompile(`(?i)^(?:import|package|def |func |class |struct |select |update |insert |delete |#include|from .* import |<!doctype|<html|\{.*\}|\[.*\]|/\*.*\*/|//.*|/\*|#|---)`)

func DetectDirection(text string) string {
	if codeRegex.MatchString(strings.TrimSpace(text)) {
		return "ltr"
	}

	persianCount := 0
	latinCount := 0

	for _, r := range text {
		if r >= '\u0600' && r <= '\u06FF' {
			persianCount++
		} else if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			latinCount++
		}
	}

	if persianCount > latinCount {
		return "rtl"
	}
	if latinCount > persianCount {
		return "ltr"
	}
	return "auto"
}
