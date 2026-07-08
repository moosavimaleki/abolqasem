package sessioninterop

import (
	"fmt"
	"strings"

	"abolqasem/internal/workspace/readmodels"
)

type ExportArgs struct {
	Provider           string
	LocalPath          string
	Title              string
	SourceSessionToken string
	Entries            []readmodels.TranscriptEntry
	PreferFork         bool
}

type ExportResult struct {
	SessionToken           string
	TranscriptPath         string
	ProjectPath            string
	PendingForkSourceToken string
}

func ExportNativeSession(args ExportArgs) (ExportResult, error) {
	provider := strings.ToLower(strings.TrimSpace(args.Provider))
	switch provider {
	case "claude":
		return exportClaudeSession(args)
	case "codex":
		return exportCodexSession(args)
	case "gemini":
		return exportGeminiSession(args)
	default:
		return ExportResult{}, fmt.Errorf("unsupported export provider: %s", args.Provider)
	}
}
