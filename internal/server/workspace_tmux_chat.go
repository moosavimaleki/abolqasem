package server

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"abolqasem/internal/providers/catalog"
	"abolqasem/internal/providers/providerexec"
	"abolqasem/internal/state"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/events"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/tmuxruntime"
)

func workspaceSendTmuxChat(command agent.SendCommand) (agent.SendResult, bool, error) {
	store := &workspaceEventStore{store: workspaceStore()}
	chat, handled, err := workspaceResolveTmuxChat(store, command)
	if err != nil || !handled {
		return agent.SendResult{}, handled, err
	}

	projectPath, err := workspaceProjectLocalPathRequired(chat.ProjectID)
	if err != nil {
		return agent.SendResult{}, true, err
	}
	if strings.TrimSpace(command.Provider) != "" && (chat.Provider == nil || *chat.Provider != command.Provider) {
		if err := store.SetChatProvider(chat.ID, command.Provider); err != nil {
			return agent.SendResult{}, true, err
		}
	}
	if command.PlanMode != chat.PlanMode {
		if err := store.SetPlanMode(chat.ID, command.PlanMode); err != nil {
			return agent.SendResult{}, true, err
		}
	}
	if strings.TrimSpace(command.Provider) != "" {
		chat.Provider = &command.Provider
	}

	provider := workspaceTmuxProviderForChat(chat, command.Provider)
	runtimeCommand := workspaceTmuxCommandForChat(chat, command.Provider)
	if strings.TrimSpace(command.ChatID) == "" {
		runtimeCommand = workspaceTmuxCommandWithModel(runtimeCommand, provider, command.Model)
	}
	if runtimeCommand == "" && !tmuxruntime.SessionExists(context.Background(), chat.TmuxSession) {
		return agent.SendResult{}, true, errors.New("choose how to launch this tmux session first")
	}
	if err := tmuxruntime.EnsureSession(context.Background(), chat.TmuxSession, projectPath, runtimeCommand); err != nil {
		return agent.SendResult{}, true, err
	}
	if err := tmuxruntime.Send(context.Background(), chat.TmuxSession, command.Content, true); err != nil {
		return agent.SendResult{}, true, err
	}
	_ = appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatRuntimeSet, time.Now().UnixMilli(), map[string]any{
		"chatId":      chat.ID,
		"lastSummary": workspacePromptPreview(command.Content),
	})
	return agent.SendResult{ChatID: chat.ID}, true, nil
}

func workspaceCancelTmuxChat(chatID string) (bool, error) {
	if strings.TrimSpace(chatID) == "" {
		return false, nil
	}
	chat, err := (&workspaceEventStore{store: workspaceStore()}).RequireChat(chatID)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(chat.TmuxSession) == "" {
		return false, nil
	}
	return true, tmuxruntime.Interrupt(context.Background(), chat.TmuxSession)
}

type workspaceRestartTmuxCommand struct {
	ChatID      string
	Provider    string
	TmuxCommand string
}

var restartTmuxRuntime = tmuxruntime.RestartSession

func decodeRestartTmuxCommand(raw json.RawMessage) (workspaceRestartTmuxCommand, error) {
	var payload struct {
		ChatID      string `json:"chatId"`
		Provider    string `json:"provider"`
		TmuxCommand string `json:"tmuxCommand"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return workspaceRestartTmuxCommand{}, err
	}
	payload.ChatID = strings.TrimSpace(payload.ChatID)
	payload.Provider = normalizeWorkspaceTmuxProvider(payload.Provider)
	payload.TmuxCommand = strings.TrimSpace(payload.TmuxCommand)
	if payload.ChatID == "" {
		return workspaceRestartTmuxCommand{}, errors.New("chatId is required")
	}
	if payload.Provider == "" {
		return workspaceRestartTmuxCommand{}, errors.New("provider is required")
	}
	if payload.TmuxCommand == "" {
		return workspaceRestartTmuxCommand{}, errors.New("tmuxCommand is required")
	}
	return workspaceRestartTmuxCommand{
		ChatID:      payload.ChatID,
		Provider:    payload.Provider,
		TmuxCommand: payload.TmuxCommand,
	}, nil
}

func workspaceRestartTmuxChat(request workspaceRestartTmuxCommand) error {
	chat, project, err := workspaceChatProjectRequired(request.ChatID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(chat.TmuxSession) == "" {
		return errors.New("chat has no tmux session")
	}
	provider := normalizeWorkspaceTmuxProvider(request.Provider)
	if provider == "" {
		return errors.New("provider is required")
	}
	tmuxCommand := strings.TrimSpace(request.TmuxCommand)
	if tmuxCommand == "" {
		return errors.New("tmuxCommand is required")
	}
	if err := (&workspaceEventStore{store: workspaceStore()}).SetTmuxLaunch(chat.ID, provider, tmuxCommand); err != nil {
		return err
	}
	chat.Provider = &provider
	chat.TmuxCommand = tmuxCommand
	command := workspaceTmuxCommandForChat(chat, "")
	return restartTmuxRuntime(context.Background(), chat.TmuxSession, project.LocalPath, command)
}

func workspaceApplyRuntimePreferences(command workspaceRuntimePreferenceCommand) error {
	chat, _, err := workspaceChatProjectRequired(command.ChatID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(chat.TmuxSession) == "" {
		return errors.New("chat has no tmux session")
	}
	provider := workspaceTmuxProviderForChat(chat, command.Provider)
	switch provider {
	case "codex":
		effort := catalog.DefaultCodexReasoningEffort
		if command.ModelOptions != nil && command.ModelOptions.Codex != nil && catalog.IsCodexReasoningEffort(command.ModelOptions.Codex.ReasoningEffort) {
			effort = command.ModelOptions.Codex.ReasoningEffort
		}
		return tmuxruntime.ApplyCodexRuntimePreferences(context.Background(), chat.TmuxSession, command.Model, effort)
	case "claude":
		return tmuxruntime.ApplyClaudeRuntimePreferences(context.Background(), chat.TmuxSession, workspaceClaudeRuntimeModelCommand(command.Model))
	case "gemini":
		return tmuxruntime.ApplyGeminiRuntimePreferences(context.Background(), chat.TmuxSession, catalog.NormalizeServerModel("gemini", command.Model))
	default:
		return errors.New("live runtime preference changes are not supported for this provider")
	}
}

func workspaceClaudeRuntimeModelCommand(model string) string {
	model = catalog.NormalizeServerModel("claude", model)
	provider, ok := catalog.Get("claude")
	if !ok {
		return model
	}
	for _, candidate := range provider.Models {
		if candidate.ID == model && len(candidate.Aliases) > 0 {
			return candidate.Aliases[0]
		}
	}
	return model
}

func workspaceResolveTmuxChat(store *workspaceEventStore, command agent.SendCommand) (readmodels.ChatRecord, bool, error) {
	if strings.TrimSpace(command.ChatID) == "" {
		if strings.TrimSpace(command.ProjectID) == "" {
			return readmodels.ChatRecord{}, true, errors.New("missing projectId for new chat")
		}
		chat, err := store.CreateChat(command.ProjectID)
		return chat, true, err
	}

	chat, err := store.RequireChat(command.ChatID)
	if err != nil {
		return readmodels.ChatRecord{}, true, err
	}
	if strings.TrimSpace(chat.TmuxSession) == "" {
		return readmodels.ChatRecord{}, false, nil
	}
	return chat, true, nil
}

func workspaceSyncTmuxRuntimeFromHook(meta state.SessionMeta, event state.HookEvent) (string, bool, error) {
	chatID, chat, ok := workspaceTmuxChatForHook(meta, event)
	if !ok {
		return "", false, nil
	}

	timestamp := workspaceMaxInt64(time.Now().UnixMilli(), meta.UpdatedAt.UnixMilli())
	runtimeData := map[string]any{
		"chatId":               chatID,
		"nativeSessionId":      strings.TrimSpace(meta.SessionID),
		"nativeTranscriptPath": strings.TrimSpace(meta.TranscriptPath),
	}
	if summary := workspaceHookRuntimeSummary(meta, event, chat); summary != "" {
		runtimeData["lastSummary"] = summary
	}
	if err := appendWorkspaceStoreEvent(workspaceStore(), events.StreamChats, events.TypeChatRuntimeSet, timestamp, runtimeData); err != nil {
		return chatID, true, err
	}

	sessionToken := strings.TrimSpace(meta.SessionID)
	if sessionToken == "" {
		return chatID, true, nil
	}
	if err := (&workspaceEventStore{store: workspaceStore()}).SetSessionToken(chatID, sessionToken); err != nil {
		return chatID, true, err
	}
	return chatID, true, nil
}

func workspaceTmuxChatForHook(meta state.SessionMeta, event state.HookEvent) (string, readmodels.ChatRecord, bool) {
	storeState, err := workspaceStore().LoadStateLight()
	if err != nil {
		return "", readmodels.ChatRecord{}, false
	}
	if chatID := workspaceStoredChatIDForLegacyMeta(storeState, meta); chatID != "" {
		if chat, ok := storeState.ChatsByID[chatID]; ok && chat.DeletedAt == 0 && strings.TrimSpace(chat.TmuxSession) != "" {
			return chatID, chat, true
		}
	}

	provider := normalizeWorkspaceTmuxProvider(meta.Agent)
	cwd := resolveWorkspaceLocalPath(meta.Cwd)
	promptPreview := workspacePromptPreview(event.PromptPreview)
	var matched readmodels.ChatRecord
	matchedID := ""
	for chatID, chat := range storeState.ChatsByID {
		if !workspaceTmuxHookCandidate(storeState, chat, provider, cwd, promptPreview, meta) {
			continue
		}
		if matchedID != "" {
			return "", readmodels.ChatRecord{}, false
		}
		matchedID = chatID
		matched = chat
	}
	if matchedID == "" {
		return "", readmodels.ChatRecord{}, false
	}
	return matchedID, matched, true
}

func workspaceTmuxHookCandidate(
	storeState readmodels.StoreState,
	chat readmodels.ChatRecord,
	provider string,
	cwd string,
	promptPreview string,
	meta state.SessionMeta,
) bool {
	if chat.DeletedAt != 0 || strings.TrimSpace(chat.TmuxSession) == "" {
		return false
	}
	if provider != "" {
		chatProvider := strings.TrimSpace(derefWorkspaceString(chat.Provider))
		if chatProvider != "" && !strings.EqualFold(chatProvider, provider) {
			return false
		}
	}
	if existing := strings.TrimSpace(derefWorkspaceString(chat.SessionToken)); existing != "" && !strings.EqualFold(existing, strings.TrimSpace(meta.SessionID)) {
		return false
	}
	if existing := strings.TrimSpace(chat.NativeSessionID); existing != "" && !strings.EqualFold(existing, strings.TrimSpace(meta.SessionID)) {
		return false
	}
	if existing := strings.TrimSpace(chat.NativeTranscriptPath); existing != "" && !sameWorkspacePath(existing, meta.TranscriptPath) {
		return false
	}
	if cwd == "" || !workspaceChatProjectPathMatches(storeState, chat, cwd) {
		return false
	}
	if promptPreview == "" {
		return false
	}
	return workspacePromptPreviewMatches(chat.LastSummary, promptPreview)
}

func workspaceChatProjectPathMatches(storeState readmodels.StoreState, chat readmodels.ChatRecord, cwd string) bool {
	project, ok := storeState.ProjectsByID[chat.ProjectID]
	if !ok || project.DeletedAt != 0 {
		return false
	}
	return sameWorkspacePath(project.LocalPath, cwd)
}

func workspacePromptPreviewMatches(left string, right string) bool {
	left = workspacePromptPreview(left)
	right = workspacePromptPreview(right)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(left, right) || strings.HasPrefix(left, right) || strings.HasPrefix(right, left)
}

func sameWorkspacePath(left string, right string) bool {
	left = resolveWorkspaceLocalPath(left)
	right = resolveWorkspaceLocalPath(right)
	if left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func workspaceHookRuntimeSummary(meta state.SessionMeta, event state.HookEvent, chat readmodels.ChatRecord) string {
	return workspacePromptPreview(firstNonEmpty(event.LastPreview, event.PromptPreview, meta.LastPreview, meta.FirstPreview, chat.LastSummary))
}

func workspaceResolveTmuxProviderCommand(provider string, command string) string {
	return providerexec.ResolveCommand(provider, command)
}

func workspaceTmuxCommandForChat(chat readmodels.ChatRecord, providerOverride string) string {
	provider := workspaceTmuxProviderForChat(chat, providerOverride)
	if settings, err := state.LoadSettings(); err == nil {
		providerexec.SetConfiguredExecutables(settings.ProviderExecutables)
	}
	base := strings.TrimSpace(chat.TmuxCommand)
	if base == "" {
		return ""
	}
	base = workspaceResolveTmuxProviderCommand(provider, base)
	token := firstNonEmpty(chat.NativeSessionID, derefWorkspaceString(chat.SessionToken), derefWorkspaceString(chat.PendingForkSessionToken))
	token = strings.TrimSpace(token)
	if token == "" || !workspaceTmuxCommandSupportsResume(base, provider) {
		return base
	}
	switch provider {
	case "claude", "gemini":
		return strings.TrimSpace(base + " --resume " + shellQuote(token))
	default:
		return strings.TrimSpace(base + " resume " + shellQuote(token))
	}
}

func workspaceTmuxProviderForChat(chat readmodels.ChatRecord, providerOverride string) string {
	if provider := normalizeWorkspaceTmuxProvider(firstNonEmpty(providerOverride, derefWorkspaceString(chat.Provider))); provider != "" {
		return provider
	}
	if provider := workspaceTmuxProviderFromNativeTranscriptPath(chat.NativeTranscriptPath); provider != "" {
		return provider
	}
	if provider := workspaceTmuxProviderFromCommand(chat.TmuxCommand); provider != "" {
		return provider
	}
	return "codex"
}

func workspaceTmuxProviderFromNativeTranscriptPath(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if path == "" {
		return ""
	}
	for _, part := range strings.Split(path, "/") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case ".codex":
			return "codex"
		case ".claude":
			return "claude"
		case ".gemini":
			return "gemini"
		}
	}
	return ""
}

func workspaceTmuxProviderFromCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return normalizeWorkspaceTmuxProvider(workspaceTmuxCommandBase(fields[0]))
}

func workspaceTmuxCommandSupportsResume(command string, provider string) bool {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	binary := workspaceTmuxCommandBase(parts[0])
	return binary == normalizeWorkspaceTmuxProvider(provider)
}

func workspaceTmuxCommandWithModel(command string, provider string, model string) string {
	command = strings.TrimSpace(command)
	model = strings.TrimSpace(model)
	if command == "" || model == "" || normalizeWorkspaceTmuxProvider(provider) == "" || workspaceTmuxCommandHasModelFlag(command) {
		return command
	}
	return strings.TrimSpace(command + " --model " + shellQuote(model))
}

func workspaceTmuxCommandHasModelFlag(command string) bool {
	fields := strings.Fields(command)
	for index, field := range fields {
		switch {
		case field == "--model", field == "-m":
			return true
		case strings.HasPrefix(field, "--model="), strings.HasPrefix(field, "-m="):
			return true
		case (field == "-c" || field == "--config") && index+1 < len(fields) && strings.HasPrefix(fields[index+1], "model="):
			return true
		}
	}
	return false
}

func workspaceTmuxCommandBase(path string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	base := strings.ToLower(filepath.Base(normalized))
	return strings.TrimSuffix(base, ".exe")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func normalizeWorkspaceTmuxProvider(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	switch provider {
	case "claude", "codex", "gemini":
		return provider
	default:
		return ""
	}
}

func workspacePromptPreview(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	runes := []rune(content)
	if len(runes) <= 160 {
		return content
	}
	return strings.TrimSpace(string(runes[:160]))
}
