package server

import (
	"testing"

	"abolqasem/internal/workspace/readmodels"
)

func TestWorkspaceRuntimeDoesNotUseLegacyTmuxMetadata(t *testing.T) {
	chat := readmodels.ChatRecord{TmuxSession: "abolqasem-legacy", TmuxCommand: "codex"}
	if workspaceChatHasTmuxRuntime(chat) {
		t.Fatal("legacy tmux metadata must not activate a tmux runtime")
	}
}
