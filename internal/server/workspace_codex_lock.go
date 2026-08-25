package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/readmodels"
)

const (
	codexLockAvailable      = "available"
	codexLockOwnedByUs      = "owned_by_us"
	codexLockOwnedElsewhere = "owned_elsewhere"
	codexLockUnknown        = "unknown"
)

type workspaceCodexLockOwner struct {
	PID     int
	Command string
}

func workspaceChatRequired(chatID string) (readmodels.ChatRecord, error) {
	stateSnapshot, err := workspaceStore().LoadStateLight()
	if err != nil {
		return readmodels.ChatRecord{}, err
	}
	chat, ok := stateSnapshot.ChatsByID[strings.TrimSpace(chatID)]
	if !ok || chat.DeletedAt != 0 {
		return readmodels.ChatRecord{}, errors.New("chat not found")
	}
	return chat, nil
}

func workspaceEnsureCodexChatWritable(chatID string) error {
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return err
	}
	status := workspaceCodexLockStatus(chat)
	switch status.State {
	case codexLockAvailable, codexLockOwnedByUs:
		return nil
	case codexLockOwnedElsewhere:
		return errors.New("Codex session is locked by another process; take it over or wait for its owner to release it")
	default:
		return errors.New("Codex session ownership is unknown; claim or refresh the session before sending")
	}
}

func workspaceCodexLockStatus(chat readmodels.ChatRecord) readmodels.CodexLockStatus {
	if derefWorkspaceString(chat.Provider) != "codex" {
		return readmodels.CodexLockStatus{State: codexLockAvailable}
	}
	sessionID := firstNonEmpty(chat.NativeSessionID, derefWorkspaceString(chat.SessionToken))
	if sessionID == "" {
		return readmodels.CodexLockStatus{State: codexLockAvailable, Message: "A Codex session will be claimed when the first prompt is sent."}
	}
	if executionMode, owned := workspaceCodexSessions.ownedExecutionMode(chat.ID, sessionID); owned {
		return readmodels.CodexLockStatus{
			State:         codexLockOwnedByUs,
			SessionID:     sessionID,
			ExecutionMode: executionMode,
			CanRelease:    true,
			Message:       "This Abolqasem server owns the Codex session.",
		}
	}

	path, err := workspaceCodexSessionPathForChat(chat, sessionID)
	if err != nil {
		return readmodels.CodexLockStatus{
			State:     codexLockUnknown,
			SessionID: sessionID,
			Message:   "The Codex session file could not be located. Refresh or claim it before sending.",
		}
	}
	if !strings.HasSuffix(filepath.Base(path), "-"+sessionID+".jsonl") {
		// Imported/legacy transcript paths are read-only history sources, not the
		// durable Codex writer file. Do not run a costly lsof scan for them.
		return readmodels.CodexLockStatus{State: codexLockAvailable, SessionID: sessionID, SessionPath: path}
	}
	owners, err := workspaceCodexWritableOwners(path)
	if err != nil {
		return readmodels.CodexLockStatus{
			State:       codexLockUnknown,
			SessionID:   sessionID,
			SessionPath: path,
			Message:     "Could not inspect the Codex session owner: " + err.Error(),
		}
	}
	if len(owners) == 0 {
		return readmodels.CodexLockStatus{State: codexLockAvailable, SessionID: sessionID, SessionPath: path}
	}
	for _, owner := range owners {
		if executionMode, owned := workspaceCodexSessions.ownedExecutionModeByWriterPID(chat.ID, owner.PID); owned {
			return readmodels.CodexLockStatus{
				State:         codexLockOwnedByUs,
				SessionID:     sessionID,
				SessionPath:   path,
				ExecutionMode: executionMode,
				CanRelease:    true,
				Message:       "This Abolqasem server owns the Codex session.",
			}
		}
	}
	status := readmodels.CodexLockStatus{
		State:        codexLockOwnedElsewhere,
		SessionID:    sessionID,
		SessionPath:  path,
		OwnerPID:     owners[0].PID,
		OwnerCommand: owners[0].Command,
		CanTakeOver:  runtime.GOOS != "windows",
		Message:      "Another Codex process has the session writer. The chat is read-only until it is released or taken over.",
	}
	status.OtherWritableSessions = workspaceCodexOtherWritableSessionCount(owners[0].PID, path)
	return status
}

func workspaceCodexSessionPathForChat(chat readmodels.ChatRecord, sessionID string) (string, error) {
	if path := strings.TrimSpace(chat.NativeTranscriptPath); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return workspaceCodexSessionPath(sessionID)
}

func workspaceCodexSessionPath(sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", errors.New("Codex session id is empty")
	}
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		codexHome = filepath.Join(home, ".codex")
	}
	var matches []string
	for _, root := range []string{filepath.Join(codexHome, "sessions"), filepath.Join(codexHome, "archived_sessions")} {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry == nil || entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(entry.Name(), "-"+sessionID+".jsonl") {
				matches = append(matches, path)
			}
			return nil
		}); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if len(matches) != 1 {
		if len(matches) == 0 {
			return "", errors.New("session file not found")
		}
		return "", errors.New("more than one session file matched")
	}
	return matches[0], nil
}

// lsof reports every open descriptor. Only w/u descriptors make the process a
// session writer; readers such as the Abolqasem transcript view must not lock a chat.
func workspaceCodexWritableOwners(sessionPath string) ([]workspaceCodexLockOwner, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("writer inspection is not supported on Windows")
	}
	output, err := exec.Command("lsof", "-nP", "-Fpcfa", "--", sessionPath).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && workspaceCodexLsofExitMeansNoMatch(exitErr.ExitCode()) {
			// lsof uses exit code 1 when the selected path has no open files.
			// It may still print unrelated mount warnings (for example Docker
			// namespaces) to stderr, which must not turn an unlocked chat into
			// an unknown session-owner state.
			return workspaceCodexWritableOwnerRecords(string(output)), nil
		}
		return nil, err
	}
	return workspaceCodexWritableOwnerRecords(string(output)), nil
}

func workspaceCodexLsofExitMeansNoMatch(exitCode int) bool {
	return exitCode == 1
}

func workspaceCodexWritableOwnerRecords(output string) []workspaceCodexLockOwner {
	owners := map[int]workspaceCodexLockOwner{}
	currentPID := 0
	currentCommand := ""
	currentFD := ""
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			currentPID, _ = strconv.Atoi(line[1:])
			currentCommand = ""
			currentFD = ""
		case 'c':
			currentCommand = line[1:]
		case 'f':
			currentFD = line[1:]
			if currentPID > 0 && workspaceCodexWritableFD(currentFD) {
				owners[currentPID] = workspaceCodexLockOwner{PID: currentPID, Command: currentCommand}
			}
		case 'a':
			if currentPID > 0 && currentFD != "" && workspaceCodexWritableAccess(line[1:]) {
				owners[currentPID] = workspaceCodexLockOwner{PID: currentPID, Command: currentCommand}
			}
		}
	}
	result := make([]workspaceCodexLockOwner, 0, len(owners))
	for _, owner := range owners {
		result = append(result, owner)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PID < result[j].PID })
	return result
}

func workspaceCodexWritableFD(value string) bool {
	return strings.HasSuffix(value, "w") || strings.HasSuffix(value, "u")
}

func workspaceCodexWritableAccess(value string) bool {
	return value == "w" || value == "u"
}

func workspaceCodexOtherWritableSessionCount(pid int, targetPath string) int {
	if pid <= 0 || runtime.GOOS == "windows" {
		return 0
	}
	output, err := exec.Command("lsof", "-nP", "-p", strconv.Itoa(pid), "-Ffna").Output()
	if err != nil {
		return 0
	}
	seen := map[string]bool{}
	writable := false
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'f':
			writable = workspaceCodexWritableFD(line[1:])
		case 'a':
			writable = writable || workspaceCodexWritableAccess(line[1:])
		case 'n':
			path := line[1:]
			if writable && path != targetPath && strings.Contains(filepath.ToSlash(path), "/.codex/sessions/") && strings.HasSuffix(path, ".jsonl") {
				seen[path] = true
			}
		}
	}
	return len(seen)
}

func workspaceReleaseCodexSession(chatID string) (readmodels.CodexLockStatus, error) {
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return readmodels.CodexLockStatus{}, err
	}
	status := workspaceCodexLockStatus(chat)
	if status.State != codexLockOwnedByUs {
		return status, errors.New("this server does not own the Codex session")
	}
	if workspaceAgentCoordinator().ActiveStatuses()[chatID] != "" {
		return status, errors.New("cannot release a session while its turn is active")
	}
	workspaceCodexSessions.close(chatID)
	return workspaceCodexLockStatus(chat), nil
}

func workspaceClaimCodexSession(chatID string) (readmodels.CodexLockStatus, error) {
	return workspaceClaimCodexSessionWithMode(chatID, "dangerous")
}

func workspaceClaimCodexSessionWithMode(chatID string, executionMode string) (readmodels.CodexLockStatus, error) {
	executionMode = workspaceCodexExecutionPolicyFor(executionMode).mode
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return readmodels.CodexLockStatus{}, err
	}
	if derefWorkspaceString(chat.Provider) != "codex" {
		return readmodels.CodexLockStatus{}, errors.New("session ownership is only available for Codex chats")
	}
	status := workspaceCodexLockStatus(chat)
	if status.State == codexLockOwnedByUs || status.State == codexLockAvailable && status.SessionID == "" {
		return status, nil
	}
	if status.State != codexLockAvailable {
		return status, errors.New("Codex session is not available to claim")
	}
	project, err := workspaceProjectLocalPathRequired(chat.ProjectID)
	if err != nil {
		return status, err
	}
	session, err := workspaceCodexSessions.session(context.Background(), agent.TurnRequest{
		ChatID:        chat.ID,
		LocalPath:     project,
		Provider:      "codex",
		SessionToken:  status.SessionID,
		ExecutionMode: executionMode,
	})
	if err != nil {
		return workspaceCodexLockStatus(chat), err
	}
	session.startIdleDrain()
	return workspaceCodexLockStatus(chat), nil
}

func workspaceSetCodexExecutionMode(chatID string, executionMode string) (readmodels.CodexLockStatus, error) {
	if executionMode != "standard" && executionMode != "dangerous" {
		return readmodels.CodexLockStatus{}, errors.New("invalid Codex execution mode")
	}
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return readmodels.CodexLockStatus{}, err
	}
	status := workspaceCodexLockStatus(chat)
	if status.State != codexLockOwnedByUs {
		return status, errors.New("this server does not own the Codex session")
	}
	if workspaceAgentCoordinator().ActiveStatuses()[chatID] != "" {
		return status, errors.New("cannot change execution mode while a turn is active")
	}
	executionMode = workspaceCodexExecutionPolicyFor(executionMode).mode
	if status.ExecutionMode == executionMode {
		return status, nil
	}
	workspaceCodexSessions.close(chatID)
	return workspaceClaimCodexSessionWithMode(chatID, executionMode)
}

func workspaceReloadCodexAuth(chatID string) (readmodels.CodexLockStatus, error) {
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return readmodels.CodexLockStatus{}, err
	}
	if derefWorkspaceString(chat.Provider) != "codex" {
		return readmodels.CodexLockStatus{}, errors.New("account reload is only available for Codex chats")
	}
	status := workspaceCodexLockStatus(chat)
	if status.State != codexLockOwnedByUs {
		return status, errors.New("this server does not own the Codex session")
	}
	if workspaceAgentCoordinator().ActiveStatuses()[chatID] != "" {
		return status, errors.New("cannot reload Codex authentication while a turn is active")
	}

	executionMode := workspaceCodexExecutionPolicyFor(status.ExecutionMode).mode
	projectPath, err := workspaceProjectLocalPathRequired(chat.ProjectID)
	if err != nil {
		return status, err
	}
	sessionID := status.SessionID
	if sessionID == "" {
		sessionID = derefWorkspaceString(chat.SessionToken)
	}

	workspaceCodexSessions.close(chatID)
	session, err := workspaceCodexSessions.session(context.Background(), agent.TurnRequest{
		ChatID:        chat.ID,
		LocalPath:     projectPath,
		Provider:      "codex",
		SessionToken:  sessionID,
		ExecutionMode: executionMode,
	})
	if err != nil {
		return workspaceCodexLockStatus(chat), fmt.Errorf("reload Codex authentication: %w", err)
	}
	session.startIdleDrain()
	return workspaceCodexLockStatus(chat), nil
}

func workspaceTakeOverCodexSession(chatID string, confirmed bool, executionMode string) (readmodels.CodexLockStatus, error) {
	if !confirmed {
		return readmodels.CodexLockStatus{}, errors.New("takeover requires explicit confirmation")
	}
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return readmodels.CodexLockStatus{}, err
	}
	status := workspaceCodexLockStatus(chat)
	if status.State == codexLockAvailable {
		return status, nil
	}
	if status.State != codexLockOwnedElsewhere || status.OwnerPID <= 0 {
		return status, errors.New("Codex session is not available for takeover")
	}
	if runtime.GOOS == "windows" {
		return status, errors.New("takeover is not supported on Windows")
	}
	if status.OwnerPID == os.Getpid() {
		return status, errors.New("refusing to terminate the current server")
	}
	process, err := os.FindProcess(status.OwnerPID)
	if err != nil {
		return status, err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return status, fmt.Errorf("terminate Codex owner: %w", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		owners, inspectErr := workspaceCodexWritableOwners(status.SessionPath)
		if inspectErr == nil && len(owners) == 0 {
			return workspaceClaimCodexSessionWithMode(chatID, executionMode)
		}
	}
	owners, inspectErr := workspaceCodexWritableOwners(status.SessionPath)
	if inspectErr != nil {
		return status, inspectErr
	}
	for _, owner := range owners {
		if owner.PID == os.Getpid() {
			return status, errors.New("refusing to terminate the current server")
		}
		ownerProcess, findErr := os.FindProcess(owner.PID)
		if findErr == nil {
			_ = ownerProcess.Signal(syscall.SIGKILL)
		}
	}
	time.Sleep(100 * time.Millisecond)
	return workspaceClaimCodexSessionWithMode(chatID, executionMode)
}
