package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ai-agent-manager/internal/workspace/events"
	"ai-agent-manager/internal/workspace/gitservice"
	"ai-agent-manager/internal/workspace/readmodels"
)

const (
	workspaceCheckpointVersion       = 2
	workspaceCheckpointVersionLegacy = 1
	workspaceCheckpointTriggerPrompt = "before_user_prompt"
	workspaceCheckpointTriggerSafety = "before_restore"
	workspaceCheckpointModeCode      = "code"
	workspaceCheckpointModeChat      = "chat"
	workspaceCheckpointModeBoth      = "code_and_chat"

	filesystemCheckpointMaxFileSize = 10 * 1024 * 1024
	filesystemCheckpointMaxBytes    = 200 * 1024 * 1024
	filesystemCheckpointMaxFiles    = 5000
	workspaceCheckpointMessagesFile = "messages.jsonl.gz"
)

type workspaceCheckpointRecord struct {
	Version          int                          `json:"version"`
	ID               string                       `json:"id"`
	ChatID           string                       `json:"chatId"`
	ProjectID        string                       `json:"projectId"`
	LocalPath        string                       `json:"localPath"`
	Title            string                       `json:"title"`
	CreatedAt        int64                        `json:"createdAt"`
	Trigger          string                       `json:"trigger"`
	PromptPreview    string                       `json:"promptPreview,omitempty"`
	RestoreOf        string                       `json:"restoreOf,omitempty"`
	Code             gitservice.CodeCheckpoint    `json:"code"`
	Messages         []readmodels.TranscriptEntry `json:"messages,omitempty"`
	ChatMessageIDs   []string                     `json:"chatMessageIds"`
	ChatMessageCount int                          `json:"chatMessageCount,omitempty"`
	ChatSnapshotPath string                       `json:"chatSnapshotPath,omitempty"`
	FileModes        map[string]uint32            `json:"fileModes,omitempty"`
}

type workspaceCheckpointSummary struct {
	ID               string `json:"id"`
	ChatID           string `json:"chatId"`
	ProjectID        string `json:"projectId"`
	Title            string `json:"title"`
	CreatedAt        int64  `json:"createdAt"`
	Trigger          string `json:"trigger"`
	PromptPreview    string `json:"promptPreview,omitempty"`
	RestoreOf        string `json:"restoreOf,omitempty"`
	CodeKind         string `json:"codeKind"`
	CodeStatus       string `json:"codeStatus"`
	CodeWarning      string `json:"codeWarning,omitempty"`
	BranchName       string `json:"branchName,omitempty"`
	Commit           string `json:"commit,omitempty"`
	FileCount        int    `json:"fileCount,omitempty"`
	ChatMessageCount int    `json:"chatMessageCount"`
}

type workspaceCheckpointRestoreResult struct {
	OK               bool                           `json:"ok"`
	Mode             string                         `json:"mode"`
	Checkpoint       workspaceCheckpointSummary     `json:"checkpoint"`
	SafetyCheckpoint workspaceCheckpointSummary     `json:"safetyCheckpoint"`
	CodeResult       *gitservice.BranchActionResult `json:"codeResult,omitempty"`
	ChatRestored     bool                           `json:"chatRestored,omitempty"`
}

type workspaceCreateCheckpointArgs struct {
	ChatID        string
	Trigger       string
	PromptPreview string
	RestoreOf     string
}

func workspaceCreateCheckpoint(args workspaceCreateCheckpointArgs) (workspaceCheckpointRecord, error) {
	chat, project, err := workspaceChatProjectRequired(args.ChatID)
	if err != nil {
		return workspaceCheckpointRecord{}, err
	}
	messages, err := workspaceChatMessages(chat.ID)
	if err != nil {
		return workspaceCheckpointRecord{}, err
	}

	id := "checkpoint-" + randomID()
	createdAt := time.Now().UnixMilli()
	record := workspaceCheckpointRecord{
		Version:          workspaceCheckpointVersion,
		ID:               id,
		ChatID:           chat.ID,
		ProjectID:        project.ID,
		LocalPath:        project.LocalPath,
		Title:            checkpointTitle(args.Trigger, args.PromptPreview, createdAt),
		CreatedAt:        createdAt,
		Trigger:          firstNonEmpty(args.Trigger, workspaceCheckpointTriggerPrompt),
		PromptPreview:    trimPromptPreview(args.PromptPreview),
		RestoreOf:        strings.TrimSpace(args.RestoreOf),
		ChatMessageIDs:   transcriptEntryIDs(messages),
		ChatMessageCount: len(messages),
		ChatSnapshotPath: workspaceCheckpointMessagesFile,
		FileModes:        map[string]uint32{},
	}

	code, err := gitservice.CreateCodeCheckpoint(context.Background(), project.LocalPath, id, record.Title)
	if err != nil {
		code = gitservice.CodeCheckpoint{
			Kind:    gitservice.CodeCheckpointKindNone,
			Status:  gitservice.StatusUnknown,
			Warning: err.Error(),
		}
	}
	if code.Kind == gitservice.CodeCheckpointKindNone {
		if fsCode, fsErr := workspaceCreateFilesystemCodeCheckpoint(project.LocalPath, workspaceCheckpointFilesDir(id)); fsErr == nil {
			code = fsCode
		} else if code.Warning == "" {
			code.Warning = fsErr.Error()
		} else {
			code.Warning = code.Warning + "; " + fsErr.Error()
		}
	}
	record.Code = code

	if err := workspaceWriteCheckpointMessagesSnapshot(record.ID, messages); err != nil {
		return workspaceCheckpointRecord{}, err
	}
	if err := workspaceWriteCheckpointRecord(record); err != nil {
		return workspaceCheckpointRecord{}, err
	}
	_ = workspacePruneCheckpointsForChat(record.ChatID)
	return record, nil
}

func workspaceListCheckpointsForProject(projectID string) []workspaceCheckpointSummary {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return []workspaceCheckpointSummary{}
	}
	records := workspaceReadCheckpointRecords()
	summaries := make([]workspaceCheckpointSummary, 0, len(records))
	for _, record := range records {
		if record.ProjectID != projectID {
			continue
		}
		summaries = append(summaries, workspaceCheckpointSummaryFromRecord(record))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt > summaries[j].CreatedAt
	})
	return summaries
}

func workspaceListCheckpoints(raw json.RawMessage) (map[string]any, error) {
	var payload struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	chatID, err := workspaceMaterializeImportedChatIfNeeded(payload.ChatID)
	if err != nil {
		return nil, err
	}
	payload.ChatID = chatID
	_, project, err := workspaceChatProjectRequired(payload.ChatID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"checkpoints": workspaceListCheckpointsForProject(project.ID)}, nil
}

func workspaceRestoreCheckpoint(raw json.RawMessage) (workspaceCheckpointRestoreResult, string, error) {
	var payload struct {
		ChatID       string `json:"chatId"`
		CheckpointID string `json:"checkpointId"`
		Mode         string `json:"mode"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return workspaceCheckpointRestoreResult{}, "", err
	}
	mode := strings.TrimSpace(payload.Mode)
	if mode == "" {
		mode = workspaceCheckpointModeBoth
	}
	if mode != workspaceCheckpointModeCode && mode != workspaceCheckpointModeChat && mode != workspaceCheckpointModeBoth {
		return workspaceCheckpointRestoreResult{}, "", errors.New("unsupported checkpoint restore mode")
	}
	chatID, err := workspaceMaterializeImportedChatIfNeeded(payload.ChatID)
	if err != nil {
		return workspaceCheckpointRestoreResult{}, "", err
	}
	payload.ChatID = chatID
	if status := workspaceAgentCoordinator().ActiveStatuses()[payload.ChatID]; status != "" {
		return workspaceCheckpointRestoreResult{}, "", fmt.Errorf("chat is %s; cancel the active turn before restoring a checkpoint", status)
	}
	chat, project, err := workspaceChatProjectRequired(payload.ChatID)
	if err != nil {
		return workspaceCheckpointRestoreResult{}, "", err
	}
	record, err := workspaceReadCheckpointRecord(payload.CheckpointID)
	if err != nil {
		return workspaceCheckpointRestoreResult{}, "", err
	}
	if record.ProjectID != project.ID {
		return workspaceCheckpointRestoreResult{}, "", errors.New("checkpoint belongs to a different project")
	}
	if (mode == workspaceCheckpointModeChat || mode == workspaceCheckpointModeBoth) && record.ChatID != chat.ID {
		return workspaceCheckpointRestoreResult{}, "", errors.New("chat restore requires a checkpoint from this chat")
	}

	safety, err := workspaceCreateCheckpoint(workspaceCreateCheckpointArgs{
		ChatID:        chat.ID,
		Trigger:       workspaceCheckpointTriggerSafety,
		PromptPreview: "Safety checkpoint before restore",
		RestoreOf:     record.ID,
	})
	if err != nil {
		return workspaceCheckpointRestoreResult{}, "", err
	}

	var codeResult *gitservice.BranchActionResult
	if mode == workspaceCheckpointModeCode || mode == workspaceCheckpointModeBoth {
		result, err := workspaceRestoreCheckpointCode(record)
		if err != nil {
			return workspaceCheckpointRestoreResult{}, "", err
		}
		codeResult = &result
		if !result.OK {
			return workspaceCheckpointRestoreResult{}, "", errors.New(result.Message)
		}
	}
	chatRestored := false
	if mode == workspaceCheckpointModeChat || mode == workspaceCheckpointModeBoth {
		if err := workspaceRestoreCheckpointChat(record); err != nil {
			return workspaceCheckpointRestoreResult{}, "", err
		}
		chatRestored = true
	}

	return workspaceCheckpointRestoreResult{
		OK:               true,
		Mode:             mode,
		Checkpoint:       workspaceCheckpointSummaryFromRecord(record),
		SafetyCheckpoint: workspaceCheckpointSummaryFromRecord(safety),
		CodeResult:       codeResult,
		ChatRestored:     chatRestored,
	}, project.ID, nil
}

func workspaceRestoreCheckpointCode(record workspaceCheckpointRecord) (gitservice.BranchActionResult, error) {
	switch record.Code.Kind {
	case gitservice.CodeCheckpointKindGit:
		return gitservice.RestoreCodeCheckpoint(context.Background(), record.Code)
	case "filesystem":
		return workspaceRestoreFilesystemCodeCheckpoint(record), nil
	default:
		return gitservice.BranchActionResult{
			OK:              false,
			Title:           "Checkpoint unavailable",
			Message:         firstNonEmpty(record.Code.Warning, "This checkpoint does not contain a code snapshot."),
			SnapshotChanged: false,
		}, nil
	}
}

func workspaceRestoreCheckpointChat(record workspaceCheckpointRecord) error {
	messages, err := workspaceCheckpointMessages(record)
	if err != nil {
		return err
	}
	event, err := events.New(events.TypeChatRestoredToCheckpoint, map[string]any{
		"chatId":       record.ChatID,
		"checkpointId": record.ID,
		"messages":     messages,
	})
	if err != nil {
		return err
	}
	return workspaceStore().Append(events.StreamMessages, event)
}

func workspaceCheckpointSummaryFromRecord(record workspaceCheckpointRecord) workspaceCheckpointSummary {
	commit := record.Code.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	return workspaceCheckpointSummary{
		ID:               record.ID,
		ChatID:           record.ChatID,
		ProjectID:        record.ProjectID,
		Title:            record.Title,
		CreatedAt:        record.CreatedAt,
		Trigger:          record.Trigger,
		PromptPreview:    record.PromptPreview,
		RestoreOf:        record.RestoreOf,
		CodeKind:         record.Code.Kind,
		CodeStatus:       record.Code.Status,
		CodeWarning:      record.Code.Warning,
		BranchName:       record.Code.BranchName,
		Commit:           commit,
		FileCount:        record.Code.FileCount,
		ChatMessageCount: workspaceCheckpointMessageCount(record),
	}
}

func workspaceCheckpointsDir() string {
	return filepath.Join(workspaceDataDir(), "checkpoints")
}

func workspaceCheckpointDir(checkpointID string) string {
	return filepath.Join(workspaceCheckpointsDir(), safeSegment(checkpointID))
}

func workspaceCheckpointRecordPath(checkpointID string) string {
	return filepath.Join(workspaceCheckpointDir(checkpointID), "checkpoint.json")
}

func workspaceCheckpointFilesDir(checkpointID string) string {
	return filepath.Join(workspaceCheckpointDir(checkpointID), "files")
}

func workspaceWriteCheckpointRecord(record workspaceCheckpointRecord) error {
	if record.ID == "" {
		return errors.New("checkpoint id is required")
	}
	dir := workspaceCheckpointDir(record.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	tmp := workspaceCheckpointRecordPath(record.ID) + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, workspaceCheckpointRecordPath(record.ID))
}

func workspaceCheckpointMessagesPath(checkpointID string) string {
	return filepath.Join(workspaceCheckpointDir(checkpointID), workspaceCheckpointMessagesFile)
}

func workspaceWriteCheckpointMessagesSnapshot(checkpointID string, messages []readmodels.TranscriptEntry) error {
	if checkpointID == "" {
		return errors.New("checkpoint id is required")
	}
	dir := workspaceCheckpointDir(checkpointID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := workspaceCheckpointMessagesPath(checkpointID) + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	encoder := json.NewEncoder(gzipWriter)
	for _, message := range messages {
		if err := encoder.Encode(message); err != nil {
			_ = gzipWriter.Close()
			_ = file.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := gzipWriter.Close(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, workspaceCheckpointMessagesPath(checkpointID))
}

func workspaceCheckpointMessages(record workspaceCheckpointRecord) ([]readmodels.TranscriptEntry, error) {
	if len(record.Messages) > 0 || strings.TrimSpace(record.ChatSnapshotPath) == "" {
		return append([]readmodels.TranscriptEntry(nil), record.Messages...), nil
	}
	file, err := os.Open(workspaceCheckpointMessagesPath(record.ID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("checkpoint chat snapshot not found")
		}
		return nil, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	decoder := json.NewDecoder(gzipReader)
	messages := make([]readmodels.TranscriptEntry, 0, record.ChatMessageCount)
	for {
		var entry readmodels.TranscriptEntry
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		messages = append(messages, entry)
	}
	return messages, nil
}

func workspaceCheckpointMessageCount(record workspaceCheckpointRecord) int {
	switch {
	case record.ChatMessageCount > 0:
		return record.ChatMessageCount
	case len(record.ChatMessageIDs) > 0:
		return len(record.ChatMessageIDs)
	default:
		return len(record.Messages)
	}
}

func workspaceReadCheckpointRecord(checkpointID string) (workspaceCheckpointRecord, error) {
	checkpointID = safeSegment(checkpointID)
	if checkpointID == "" {
		return workspaceCheckpointRecord{}, errors.New("checkpointId is required")
	}
	data, err := os.ReadFile(workspaceCheckpointRecordPath(checkpointID))
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceCheckpointRecord{}, errors.New("checkpoint not found")
		}
		return workspaceCheckpointRecord{}, err
	}
	var record workspaceCheckpointRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return workspaceCheckpointRecord{}, err
	}
	if (record.Version != workspaceCheckpointVersion && record.Version != workspaceCheckpointVersionLegacy) || record.ID == "" {
		return workspaceCheckpointRecord{}, errors.New("unsupported checkpoint record")
	}
	if record.ChatMessageCount == 0 {
		record.ChatMessageCount = workspaceCheckpointMessageCount(record)
	}
	if len(record.ChatMessageIDs) == 0 && len(record.Messages) > 0 {
		record.ChatMessageIDs = transcriptEntryIDs(record.Messages)
	}
	return record, nil
}

func workspaceReadCheckpointRecords() []workspaceCheckpointRecord {
	entries, err := os.ReadDir(workspaceCheckpointsDir())
	if err != nil {
		return []workspaceCheckpointRecord{}
	}
	records := make([]workspaceCheckpointRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		record, err := workspaceReadCheckpointRecord(entry.Name())
		if err == nil {
			records = append(records, record)
		}
	}
	return records
}

func workspacePruneCheckpointsForChat(chatID string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	keep := workspaceCheckpointRetentionLimit()
	if keep <= 0 {
		return nil
	}
	records := workspaceReadCheckpointRecords()
	chatRecords := make([]workspaceCheckpointRecord, 0, len(records))
	for _, record := range records {
		if record.ChatID == chatID {
			chatRecords = append(chatRecords, record)
		}
	}
	if len(chatRecords) <= keep {
		return nil
	}
	sort.SliceStable(chatRecords, func(i, j int) bool {
		return chatRecords[i].CreatedAt > chatRecords[j].CreatedAt
	})
	for _, record := range chatRecords[keep:] {
		if record.ID == "" {
			continue
		}
		if err := os.RemoveAll(workspaceCheckpointDir(record.ID)); err != nil {
			return err
		}
	}
	return nil
}

func workspaceCheckpointRetentionLimit() int {
	value := strings.TrimSpace(os.Getenv("AI_AGENT_MANAGER_CHECKPOINT_RETENTION"))
	if value == "" {
		return 20
	}
	parsed := parsePositiveInt(value, 20)
	if parsed < 0 {
		return 20
	}
	return parsed
}

func workspaceCreateFilesystemCodeCheckpoint(localPath string, filesDir string) (gitservice.CodeCheckpoint, error) {
	root := strings.TrimSpace(localPath)
	if root == "" {
		return gitservice.CodeCheckpoint{}, errors.New("project path is empty")
	}
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return gitservice.CodeCheckpoint{}, err
	}
	fileCount := 0
	var byteCount int64
	warnings := []string{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			warnings = append(warnings, walkErr.Error())
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if shouldSkipFilesystemCheckpointDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > filesystemCheckpointMaxFileSize {
			warnings = append(warnings, rel+" skipped: file too large")
			return nil
		}
		if fileCount >= filesystemCheckpointMaxFiles || byteCount+info.Size() > filesystemCheckpointMaxBytes {
			warnings = append(warnings, "filesystem checkpoint limit reached")
			return filepath.SkipAll
		}
		target := filepath.Join(filesDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyCheckpointFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		fileCount++
		byteCount += info.Size()
		return nil
	})
	if err != nil {
		return gitservice.CodeCheckpoint{}, err
	}
	return gitservice.CodeCheckpoint{
		Kind:      "filesystem",
		Status:    gitservice.StatusReady,
		FileCount: fileCount,
		ByteCount: byteCount,
		Warning:   strings.Join(warnings, "; "),
	}, nil
}

func workspaceRestoreFilesystemCodeCheckpoint(record workspaceCheckpointRecord) gitservice.BranchActionResult {
	result := gitservice.BranchActionResult{SnapshotChanged: true}
	root := strings.TrimSpace(record.LocalPath)
	if root == "" {
		return gitservice.BranchActionResult{OK: false, Title: "Restore failed", Message: "Project path is empty.", SnapshotChanged: false}
	}
	filesDir := workspaceCheckpointFilesDir(record.ID)
	keep := map[string]bool{}
	err := filepath.WalkDir(filesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == filesDir || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filesDir, path)
		if err != nil {
			return nil
		}
		keep[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		return gitservice.BranchActionResult{OK: false, Title: "Restore failed", Message: err.Error(), SnapshotChanged: false}
	}

	dirs := []string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if shouldSkipFilesystemCheckpointDir(rel) {
				return filepath.SkipDir
			}
			dirs = append(dirs, path)
			return nil
		}
		if !keep[rel] {
			_ = os.Remove(path)
		}
		return nil
	})
	if err != nil {
		return gitservice.BranchActionResult{OK: false, Title: "Restore failed", Message: err.Error(), SnapshotChanged: false}
	}
	for rel := range keep {
		source := filepath.Join(filesDir, filepath.FromSlash(rel))
		target := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(source)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return gitservice.BranchActionResult{OK: false, Title: "Restore failed", Message: err.Error(), SnapshotChanged: false}
		}
		if err := copyCheckpointFile(source, target, info.Mode().Perm()); err != nil {
			return gitservice.BranchActionResult{OK: false, Title: "Restore failed", Message: err.Error(), SnapshotChanged: false}
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
	result.OK = true
	return result
}

func shouldSkipFilesystemCheckpointDir(rel string) bool {
	name := filepath.Base(filepath.FromSlash(rel))
	switch name {
	case ".git", ".abolqasem", "node_modules", "vendor", "dist", "build", ".next", ".turbo", ".cache", "coverage", "target":
		return true
	default:
		return false
	}
}

func copyCheckpointFile(source string, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func transcriptEntryIDs(entries []readmodels.TranscriptEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if id, ok := entry["_id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func checkpointTitle(trigger string, prompt string, createdAt int64) string {
	if strings.TrimSpace(trigger) == workspaceCheckpointTriggerSafety {
		return "نقطه بازگشت ایمنی قبل از بازگردانی"
	}
	preview := trimPromptPreview(prompt)
	if preview != "" {
		return "قبل از: " + preview
	}
	return "نقطه بازگشت " + time.UnixMilli(createdAt).Format("2006-01-02 15:04:05")
}

func trimPromptPreview(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= 120 {
		return value
	}
	runes := []rune(value)
	return string(runes[:120])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
