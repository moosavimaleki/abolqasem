package sessioninterop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"
)

const geminiVisibleToolTextLimit = 4000

func exportGeminiSession(args ExportArgs) (ExportResult, error) {
	now := time.Now().UTC()
	sessionToken := generateSessionToken()
	localPath := normalizeGeminiProjectPath(args.LocalPath)
	container, err := geminiProjectIdentifier(localPath)
	if err != nil {
		return ExportResult{}, err
	}
	projectDir := filepath.Join(geminiRootDir(), "tmp", container)
	chatsDir := filepath.Join(projectDir, "chats")
	transcriptPath := filepath.Join(chatsDir, "session-"+now.Format("2006-01-02T15-04")+"-"+sessionToken[:8]+".jsonl")
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		return ExportResult{}, err
	}
	file, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return ExportResult{}, err
	}
	defer file.Close()
	meta := map[string]any{
		"sessionId":   sessionToken,
		"projectHash": geminiProjectHash(localPath),
		"startTime":   now.Format(time.RFC3339),
		"lastUpdated": now.Format(time.RFC3339),
		"kind":        "main",
	}
	if err := writeJSONLRecord(file, meta); err != nil {
		return ExportResult{}, err
	}
	for _, record := range geminiRecordsFromEntries(args.Entries) {
		if err := writeJSONLRecord(file, record); err != nil {
			return ExportResult{}, err
		}
	}
	return ExportResult{SessionToken: sessionToken, TranscriptPath: transcriptPath, ProjectPath: projectDir}, nil
}

func geminiRecordsFromEntries(entries []readmodels.TranscriptEntry) []map[string]any {
	records := make([]map[string]any, 0, len(entries))
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		timestamp := entryTimestampRFC3339(entry)
		switch transcript.Kind(entry) {
		case transcript.KindUserPrompt:
			records = append(records, map[string]any{
				"id":        entryID(entry, "gemini-user", index),
				"timestamp": timestamp,
				"type":      "user",
				"content":   stringValueAny(entry["content"]),
			})
		case transcript.KindAssistantText:
			records = append(records, map[string]any{
				"id":        entryID(entry, "gemini-assistant", index),
				"timestamp": timestamp,
				"type":      "gemini",
				"content":   stringValueAny(entry["text"]),
				"model":     "converted",
			})
		case transcript.KindToolCall:
			tool, _ := entry["tool"].(map[string]any)
			toolCall := map[string]any{
				"id":     stringValueAny(tool["toolId"]),
				"name":   stringValueAny(tool["toolName"]),
				"args":   tool["input"],
				"result": []map[string]any{},
				"status": "pending",
			}
			toolCall["functionCall"] = map[string]any{
				"name": toolCall["name"],
				"args": firstNonNil(toolCall["args"], map[string]any{}),
			}
			if index+1 < len(entries) && transcript.Kind(entries[index+1]) == transcript.KindToolResult && stringValueAny(entries[index+1]["toolId"]) == stringValueAny(tool["toolId"]) {
				toolCall["result"] = geminiToolFunctionResponse(toolCall, entries[index+1])
				if boolValueAny(entries[index+1]["isError"]) {
					toolCall["status"] = "error"
				} else {
					toolCall["status"] = "success"
				}
				index++
			}
			records = append(records, map[string]any{
				"id":        entryID(entry, "gemini-tool-call", index),
				"timestamp": timestamp,
				"type":      "gemini",
				"content":   geminiToolCallVisibleContent(toolCall),
				"model":     "converted",
				"toolCalls": []map[string]any{toolCall},
			})
		case transcript.KindToolResult:
			records = append(records, map[string]any{
				"id":        entryID(entry, "gemini-tool-result", index),
				"timestamp": timestamp,
				"type":      "info",
				"content":   "Tool result for " + stringValueAny(entry["toolId"]) + "\n" + stringValueAny(entry["content"]),
			})
		case transcript.KindCompactBoundary:
			records = append(records, map[string]any{
				"id":        entryID(entry, "gemini-compact-boundary", index),
				"timestamp": timestamp,
				"type":      "info",
				"content":   "Conversation checkpoint saved.",
			})
		case transcript.KindCompactSummary:
			records = append(records, map[string]any{
				"id":        entryID(entry, "gemini-compact-summary", index),
				"timestamp": timestamp,
				"type":      "info",
				"content":   "Conversation summary:\n" + stringValueAny(entry["summary"]),
			})
		}
	}
	return records
}

func geminiToolFunctionResponse(toolCall map[string]any, resultEntry readmodels.TranscriptEntry) []map[string]any {
	response := map[string]any{
		"output": stringValueAny(resultEntry["content"]),
	}
	if boolValueAny(resultEntry["isError"]) {
		response["error"] = true
	}
	return []map[string]any{
		{
			"functionResponse": map[string]any{
				"id":       stringValueAny(toolCall["id"]),
				"name":     stringValueAny(toolCall["name"]),
				"response": response,
			},
		},
	}
}

func geminiProjectIdentifier(localPath string) (string, error) {
	localPath = normalizeGeminiProjectPath(localPath)
	registryPath := filepath.Join(geminiRootDir(), "projects.json")
	return withGeminiRegistryLock(registryPath, func() (string, error) {
		projects, err := readGeminiProjectsRegistry(registryPath)
		if err != nil {
			return "", err
		}
		if existing := strings.TrimSpace(projects[localPath]); existing != "" {
			if geminiSlugBelongsToProject(existing, localPath) {
				if err := ensureGeminiOwnershipMarkers(existing, localPath); err != nil {
					return "", err
				}
				return existing, nil
			}
			delete(projects, localPath)
		}
		if existing, err := findGeminiSlugForProject(localPath); err != nil {
			return "", err
		} else if existing != "" {
			projects[localPath] = existing
			if err := writeGeminiProjectsRegistry(registryPath, projects); err != nil {
				return "", err
			}
			if err := ensureGeminiOwnershipMarkers(existing, localPath); err != nil {
				return "", err
			}
			return existing, nil
		}
		slug := geminiContainerName(localPath)
		used := map[string]bool{}
		for projectPath, projectSlug := range projects {
			if normalizeGeminiProjectPath(projectPath) != localPath {
				used[projectSlug] = true
			}
		}
		for index := 0; ; index++ {
			candidate := slug
			if index > 0 {
				candidate = slug + "-" + fmt.Sprint(index)
			}
			if used[candidate] || !geminiSlugBelongsToProject(candidate, localPath) {
				continue
			}
			if err := ensureGeminiOwnershipMarkers(candidate, localPath); err != nil {
				return "", err
			}
			projects[localPath] = candidate
			if err := writeGeminiProjectsRegistry(registryPath, projects); err != nil {
				return "", err
			}
			return candidate, nil
		}
	})
}

func readGeminiProjectsRegistry(registryPath string) (map[string]string, error) {
	projects := map[string]string{}
	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return projects, nil
		}
		return nil, err
	}
	var payload struct {
		Projects map[string]string `json:"projects"`
	}
	if err := json.Unmarshal(data, &payload); err == nil && payload.Projects != nil {
		return payload.Projects, nil
	}
	var loose struct {
		Projects map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &loose); err != nil {
		return projects, nil
	}
	for key, value := range loose.Projects {
		if text, ok := value.(string); ok {
			projects[key] = text
		}
	}
	return projects, nil
}

func writeGeminiProjectsRegistry(registryPath string, projects map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{"projects": projects}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmpPath := registryPath + "." + generateSessionToken() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, registryPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func findGeminiSlugForProject(localPath string) (string, error) {
	for _, baseDir := range geminiProjectBaseDirs() {
		entries, err := os.ReadDir(baseDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			owner, ok := geminiReadMarker(filepath.Join(baseDir, entry.Name()))
			if ok && normalizeGeminiProjectPath(owner) == localPath {
				return entry.Name(), nil
			}
		}
	}
	return "", nil
}

func geminiSlugBelongsToProject(slug string, localPath string) bool {
	for _, baseDir := range geminiProjectBaseDirs() {
		owner, ok := geminiReadMarker(filepath.Join(baseDir, slug))
		if ok && normalizeGeminiProjectPath(owner) != localPath {
			return false
		}
	}
	return true
}

func ensureGeminiOwnershipMarkers(slug string, localPath string) error {
	for _, baseDir := range geminiProjectBaseDirs() {
		slugDir := filepath.Join(baseDir, slug)
		if err := os.MkdirAll(slugDir, 0o755); err != nil {
			return err
		}
		markerPath := filepath.Join(slugDir, ".project_root")
		if owner, ok := geminiReadMarker(slugDir); ok && normalizeGeminiProjectPath(owner) != localPath {
			return fmt.Errorf("gemini project slug %q already belongs to %s", slug, owner)
		}
		if err := os.WriteFile(markerPath, []byte(localPath), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func geminiProjectBaseDirs() []string {
	root := geminiRootDir()
	return []string{
		filepath.Join(root, "tmp"),
		filepath.Join(root, "history"),
	}
}

func geminiReadMarker(slugDir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(slugDir, ".project_root"))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func normalizeGeminiProjectPath(localPath string) string {
	trimmed := strings.TrimSpace(localPath)
	if trimmed == "" {
		return ""
	}
	resolved, err := filepath.Abs(trimmed)
	if err != nil {
		resolved = trimmed
	}
	resolved = filepath.Clean(resolved)
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(resolved)
		if volume != "" {
			resolved = strings.ToLower(volume) + resolved[len(volume):]
		}
	}
	return resolved
}

func withGeminiRegistryLock(registryPath string, action func() (string, error)) (string, error) {
	lockPath := registryPath + ".lock"
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		return "", err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			defer os.Remove(lockPath)
			return action()
		} else if !os.IsExist(err) {
			return "", err
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for gemini registry lock")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func geminiToolCallVisibleContent(toolCall map[string]any) string {
	parts := []string{fmt.Sprintf("Tool call: %s", stringValueAny(toolCall["name"]))}
	if argsText := strings.TrimSpace(mustJSONString(toolCall["args"])); argsText != "" && argsText != "null" {
		parts = append(parts, "Arguments:", argsText)
	}
	if resultText := strings.TrimSpace(flattenUnknown(toolCall["result"])); resultText != "" {
		parts = append(parts, "Result:", resultText)
	}
	if status := strings.TrimSpace(stringValueAny(toolCall["status"])); status != "" {
		parts = append(parts, "Status: "+status)
	}
	text := strings.Join(parts, "\n")
	if len(text) <= geminiVisibleToolTextLimit {
		return text
	}
	return text[:geminiVisibleToolTextLimit] + "\n...[truncated]"
}
