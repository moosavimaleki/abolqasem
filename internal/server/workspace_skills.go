package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const skillsSearchDefaultLimit = 100

var (
	safeSkillSourceRE   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	safeSkillIDRE       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	skillsSearchBaseURL = "https://skills.sh/api/search"
	runSkillCLICommand  = defaultRunSkillCLICommand
	skillOperations     = newSkillOperationTracker()
)

const (
	skillOperationInstall = "install"
	skillOperationRemove  = "uninstall"

	skillOperationQueued    = "queued"
	skillOperationRunning   = "running"
	skillOperationSucceeded = "succeeded"
	skillOperationFailed    = "failed"
)

type skillSearchResult struct {
	ID       string `json:"id"`
	SkillID  string `json:"skillId"`
	Name     string `json:"name"`
	Installs int    `json:"installs"`
	Source   string `json:"source"`
}

type skillSearchSnapshot struct {
	Query      string              `json:"query"`
	SearchType string              `json:"searchType"`
	Skills     []skillSearchResult `json:"skills"`
	Count      int                 `json:"count"`
	DurationMS int                 `json:"duration_ms"`
}

type installedSkillSummary struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceType  string `json:"sourceType"`
	SourceURL   string `json:"sourceUrl"`
	SkillPath   string `json:"skillPath,omitempty"`
	InstalledAt string `json:"installedAt"`
	UpdatedAt   string `json:"updatedAt"`
	PluginName  string `json:"pluginName,omitempty"`
}

type installedSkillsSnapshot struct {
	LockFilePath string                  `json:"lockFilePath"`
	Skills       []installedSkillSummary `json:"skills"`
}

type skillInstallResult struct {
	Source  string   `json:"source"`
	SkillID string   `json:"skillId"`
	Command []string `json:"command"`
	CWD     string   `json:"cwd"`
	Stdout  string   `json:"stdout"`
	Stderr  string   `json:"stderr"`
}

type skillUninstallResult struct {
	SkillID string   `json:"skillId"`
	Command []string `json:"command"`
	CWD     string   `json:"cwd"`
	Stdout  string   `json:"stdout"`
	Stderr  string   `json:"stderr"`
}

type skillOperationSummary struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Source     string   `json:"source,omitempty"`
	SkillID    string   `json:"skillId"`
	Status     string   `json:"status"`
	Error      string   `json:"error,omitempty"`
	Command    []string `json:"command,omitempty"`
	CWD        string   `json:"cwd,omitempty"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	EnqueuedAt string   `json:"enqueuedAt"`
	StartedAt  string   `json:"startedAt,omitempty"`
	FinishedAt string   `json:"finishedAt,omitempty"`
}

type skillOperationsSnapshot struct {
	Operations []skillOperationSummary `json:"operations"`
}

type skillCommandOutput struct {
	CWD    string
	Stdout string
	Stderr string
}

type skillOperation struct {
	summary skillOperationSummary
	done    chan struct{}
}

type skillOperationTracker struct {
	mu         sync.Mutex
	runMu      sync.Mutex
	sequence   int64
	operations map[string]*skillOperation
	order      []string
}

func newSkillOperationTracker() *skillOperationTracker {
	return &skillOperationTracker{
		operations: map[string]*skillOperation{},
		order:      []string{},
	}
}

func workspaceSearchSkills(raw json.RawMessage) (skillSearchSnapshot, error) {
	var payload struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return skillSearchSnapshot{}, err
	}
	return searchSkills(payload.Query, payload.Limit)
}

func workspaceInstallSkill(raw json.RawMessage) (skillInstallResult, error) {
	var payload struct {
		Source  string `json:"source"`
		SkillID string `json:"skillId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return skillInstallResult{}, err
	}
	operation, err := skillOperations.startInstall(payload.Source, payload.SkillID)
	if err != nil {
		return skillInstallResult{}, err
	}
	summary := skillOperations.wait(operation)
	if summary.Status == skillOperationFailed {
		return skillInstallResult{}, errors.New(summary.Error)
	}
	return skillInstallResult{
		Source:  summary.Source,
		SkillID: summary.SkillID,
		Command: summary.Command,
		CWD:     summary.CWD,
		Stdout:  summary.Stdout,
		Stderr:  summary.Stderr,
	}, nil
}

func workspaceUninstallSkill(raw json.RawMessage) (skillUninstallResult, error) {
	var payload struct {
		SkillID string `json:"skillId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return skillUninstallResult{}, err
	}
	operation, err := skillOperations.startUninstall(payload.SkillID)
	if err != nil {
		return skillUninstallResult{}, err
	}
	summary := skillOperations.wait(operation)
	if summary.Status == skillOperationFailed {
		return skillUninstallResult{}, errors.New(summary.Error)
	}
	return skillUninstallResult{
		SkillID: summary.SkillID,
		Command: summary.Command,
		CWD:     summary.CWD,
		Stdout:  summary.Stdout,
		Stderr:  summary.Stderr,
	}, nil
}

func workspaceListSkillOperations() skillOperationsSnapshot {
	return skillOperations.snapshot()
}

func (t *skillOperationTracker) startInstall(source string, skillID string) (*skillOperation, error) {
	safeSource, err := assertSafeSkillSource(source)
	if err != nil {
		return nil, err
	}
	safeSkillID, err := assertSafeSkillID(skillID)
	if err != nil {
		return nil, err
	}

	operation, created := t.enqueue(skillOperationInstall, safeSource, safeSkillID)
	if created {
		go t.runInstall(operation)
	}
	return operation, nil
}

func (t *skillOperationTracker) startUninstall(skillID string) (*skillOperation, error) {
	safeSkillID, err := assertSafeSkillID(skillID)
	if err != nil {
		return nil, err
	}

	operation, created := t.enqueue(skillOperationRemove, "", safeSkillID)
	if created {
		go t.runUninstall(operation)
	}
	return operation, nil
}

func (t *skillOperationTracker) enqueue(kind string, source string, skillID string) (*skillOperation, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if active := t.activeOperationLocked(kind, source, skillID); active != nil {
		return active, false
	}

	t.sequence++
	now := skillOperationTimestamp(time.Now())
	operation := &skillOperation{
		summary: skillOperationSummary{
			ID:         fmt.Sprintf("skill-%s-%d", kind, t.sequence),
			Kind:       kind,
			Source:     source,
			SkillID:    skillID,
			Status:     skillOperationQueued,
			EnqueuedAt: now,
		},
		done: make(chan struct{}),
	}
	t.operations[operation.summary.ID] = operation
	t.order = append(t.order, operation.summary.ID)
	t.trimLocked()
	return operation, true
}

func (t *skillOperationTracker) activeOperationLocked(kind string, source string, skillID string) *skillOperation {
	for _, operation := range t.operations {
		summary := operation.summary
		if summary.Kind != kind || summary.SkillID != skillID {
			continue
		}
		if kind == skillOperationInstall && summary.Source != source {
			continue
		}
		if isActiveSkillOperationStatus(summary.Status) {
			return operation
		}
	}
	return nil
}

func (t *skillOperationTracker) runInstall(operation *skillOperation) {
	t.runMu.Lock()
	defer t.runMu.Unlock()

	t.markRunning(operation)
	result, err := installSkill(operation.summary.Source, operation.summary.SkillID)
	t.finish(operation, skillOperationResult{
		command: result.Command,
		cwd:     result.CWD,
		stdout:  result.Stdout,
		stderr:  result.Stderr,
	}, err)
}

func (t *skillOperationTracker) runUninstall(operation *skillOperation) {
	t.runMu.Lock()
	defer t.runMu.Unlock()

	t.markRunning(operation)
	result, err := uninstallSkill(operation.summary.SkillID)
	t.finish(operation, skillOperationResult{
		command: result.Command,
		cwd:     result.CWD,
		stdout:  result.Stdout,
		stderr:  result.Stderr,
	}, err)
}

type skillOperationResult struct {
	command []string
	cwd     string
	stdout  string
	stderr  string
}

func (t *skillOperationTracker) markRunning(operation *skillOperation) {
	t.mu.Lock()
	defer t.mu.Unlock()

	operation.summary.Status = skillOperationRunning
	operation.summary.StartedAt = skillOperationTimestamp(time.Now())
}

func (t *skillOperationTracker) finish(operation *skillOperation, result skillOperationResult, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer close(operation.done)

	operation.summary.Command = result.command
	operation.summary.CWD = result.cwd
	operation.summary.Stdout = result.stdout
	operation.summary.Stderr = result.stderr
	operation.summary.FinishedAt = skillOperationTimestamp(time.Now())
	if err != nil {
		operation.summary.Status = skillOperationFailed
		operation.summary.Error = err.Error()
		return
	}
	operation.summary.Status = skillOperationSucceeded
}

func (t *skillOperationTracker) wait(operation *skillOperation) skillOperationSummary {
	<-operation.done

	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneSkillOperationSummary(operation.summary)
}

func (t *skillOperationTracker) snapshot() skillOperationsSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	operations := make([]skillOperationSummary, 0, len(t.order))
	for i := len(t.order) - 1; i >= 0; i-- {
		operation := t.operations[t.order[i]]
		if operation == nil {
			continue
		}
		operations = append(operations, cloneSkillOperationSummary(operation.summary))
	}
	return skillOperationsSnapshot{Operations: operations}
}

func (t *skillOperationTracker) trimLocked() {
	const maxCompletedSkillOperations = 50

	completed := 0
	for i := len(t.order) - 1; i >= 0; i-- {
		operation := t.operations[t.order[i]]
		if operation == nil || isActiveSkillOperationStatus(operation.summary.Status) {
			continue
		}
		completed++
		if completed <= maxCompletedSkillOperations {
			continue
		}
		delete(t.operations, t.order[i])
		t.order = append(t.order[:i], t.order[i+1:]...)
	}
}

func cloneSkillOperationSummary(summary skillOperationSummary) skillOperationSummary {
	clone := summary
	if summary.Command != nil {
		clone.Command = append([]string{}, summary.Command...)
	}
	return clone
}

func isActiveSkillOperationStatus(status string) bool {
	return status == skillOperationQueued || status == skillOperationRunning
}

func skillOperationTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func assertSafeSkillSource(source string) (string, error) {
	normalized := strings.TrimSpace(source)
	if !safeSkillSourceRE.MatchString(normalized) {
		return "", errors.New("Skill source must be an owner/repo pair.")
	}
	return normalized, nil
}

func assertSafeSkillID(skillID string) (string, error) {
	normalized := strings.TrimSpace(skillID)
	if !safeSkillIDRE.MatchString(normalized) {
		return "", errors.New("Skill id is invalid.")
	}
	return normalized, nil
}

func globalSkillLockPath() string {
	if xdgStateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "skills", ".skill-lock.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".agents", ".skill-lock.json")
	}
	return filepath.Join(".agents", ".skill-lock.json")
}

func parseInstalledSkillsLock(parsed any, lockFilePath string) installedSkillsSnapshot {
	skillsRecord := map[string]any{}
	if record, ok := parsed.(map[string]any); ok {
		if skills, ok := record["skills"].(map[string]any); ok {
			skillsRecord = skills
		}
	}

	skills := make([]installedSkillSummary, 0, len(skillsRecord))
	for name, entry := range skillsRecord {
		record, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		skills = append(skills, installedSkillSummary{
			Name:        name,
			Source:      jsonString(record["source"]),
			SourceType:  jsonString(record["sourceType"]),
			SourceURL:   jsonString(record["sourceUrl"]),
			SkillPath:   jsonString(record["skillPath"]),
			InstalledAt: jsonString(record["installedAt"]),
			UpdatedAt:   jsonString(record["updatedAt"]),
			PluginName:  jsonString(record["pluginName"]),
		})
	}
	sort.SliceStable(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return installedSkillsSnapshot{LockFilePath: lockFilePath, Skills: skills}
}

func listInstalledSkills(lockFilePath string) installedSkillsSnapshot {
	if strings.TrimSpace(lockFilePath) == "" {
		lockFilePath = globalSkillLockPath()
	}
	data, err := os.ReadFile(lockFilePath)
	if err == nil {
		var parsed any
		if json.Unmarshal(data, &parsed) == nil {
			return parseInstalledSkillsLock(parsed, lockFilePath)
		}
	}
	return installedSkillsSnapshot{
		LockFilePath: lockFilePath,
		Skills:       scanInstalledCodexSkills(),
	}
}

func searchSkills(query string, limit int) (skillSearchSnapshot, error) {
	normalizedQuery := strings.TrimSpace(query)
	if len(normalizedQuery) < 2 {
		return skillSearchSnapshot{
			Query:      normalizedQuery,
			SearchType: "fuzzy",
			Skills:     []skillSearchResult{},
			Count:      0,
			DurationMS: 0,
		}, nil
	}
	if limit <= 0 {
		limit = skillsSearchDefaultLimit
	}
	if limit > skillsSearchDefaultLimit {
		limit = skillsSearchDefaultLimit
	}

	endpoint, err := url.Parse(skillsSearchBaseURL)
	if err != nil {
		return skillSearchSnapshot{}, err
	}
	values := endpoint.Query()
	values.Set("q", normalizedQuery)
	values.Set("limit", fmt.Sprintf("%d", limit))
	endpoint.RawQuery = values.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return skillSearchSnapshot{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return skillSearchSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return skillSearchSnapshot{}, fmt.Errorf("Skills search failed with status %d.", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return skillSearchSnapshot{}, err
	}
	var payload skillSearchSnapshot
	if err := json.Unmarshal(body, &payload); err != nil {
		return skillSearchSnapshot{}, err
	}
	return normalizeSkillSearchSnapshot(payload, normalizedQuery), nil
}

func buildInstallSkillCommand(source string, skillID string) ([]string, error) {
	safeSource, err := assertSafeSkillSource(source)
	if err != nil {
		return nil, err
	}
	safeSkillID, err := assertSafeSkillID(skillID)
	if err != nil {
		return nil, err
	}
	return []string{
		npxCommandName(),
		"skills",
		"add",
		safeSource,
		"--skill",
		safeSkillID,
		"--global",
		"--agent",
		"universal",
		"claude-code",
		"--yes",
	}, nil
}

func buildUninstallSkillCommand(skillID string) ([]string, error) {
	safeSkillID, err := assertSafeSkillID(skillID)
	if err != nil {
		return nil, err
	}
	return []string{
		npxCommandName(),
		"skills",
		"remove",
		safeSkillID,
		"--global",
		"--agent",
		"universal",
		"claude-code",
		"--yes",
	}, nil
}

func installSkill(source string, skillID string) (skillInstallResult, error) {
	command, err := buildInstallSkillCommand(source, skillID)
	if err != nil {
		return skillInstallResult{}, err
	}
	output, err := runSkillCLICommand(command)
	if err != nil {
		return skillInstallResult{}, err
	}
	return skillInstallResult{
		Source:  command[3],
		SkillID: command[5],
		Command: command,
		CWD:     output.CWD,
		Stdout:  output.Stdout,
		Stderr:  output.Stderr,
	}, nil
}

func uninstallSkill(skillID string) (skillUninstallResult, error) {
	command, err := buildUninstallSkillCommand(skillID)
	if err != nil {
		return skillUninstallResult{}, err
	}
	output, err := runSkillCLICommand(command)
	if err != nil {
		return skillUninstallResult{}, err
	}
	return skillUninstallResult{
		SkillID: command[3],
		Command: command,
		CWD:     output.CWD,
		Stdout:  output.Stdout,
		Stderr:  output.Stderr,
	}, nil
}

func defaultRunSkillCLICommand(command []string) (skillCommandOutput, error) {
	cwd, err := os.UserHomeDir()
	if err != nil {
		return skillCommandOutput{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "DISABLE_TELEMETRY="+firstNonEmptyEnv("DISABLE_TELEMETRY", "1"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = fmt.Sprintf("skills CLI exited with error: %v", err)
		}
		return skillCommandOutput{}, errors.New(message)
	}
	return skillCommandOutput{CWD: cwd, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func scanInstalledCodexSkills() []installedSkillSummary {
	skillsDir := filepath.Join(codexHomeDir(), "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return []installedSkillSummary{}
	}
	skills := make([]installedSkillSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		info, err := os.Stat(skillPath)
		if err != nil || info.IsDir() {
			continue
		}
		skills = append(skills, installedSkillSummary{
			Name:        entry.Name(),
			Source:      "",
			SourceType:  "codex",
			SourceURL:   "",
			SkillPath:   skillPath,
			InstalledAt: "",
			UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}
	sort.SliceStable(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills
}

func normalizeSkillSearchSnapshot(payload skillSearchSnapshot, fallbackQuery string) skillSearchSnapshot {
	query := payload.Query
	if query == "" {
		query = fallbackQuery
	}
	searchType := payload.SearchType
	if searchType == "" {
		searchType = "fuzzy"
	}
	skills := make([]skillSearchResult, 0, len(payload.Skills))
	for _, skill := range payload.Skills {
		if skill.ID == "" || skill.SkillID == "" || skill.Name == "" || skill.Source == "" {
			continue
		}
		if skill.Installs < 0 {
			skill.Installs = 0
		}
		skills = append(skills, skill)
	}
	count := payload.Count
	if count < 0 {
		count = 0
	}
	durationMS := payload.DurationMS
	if durationMS < 0 {
		durationMS = 0
	}
	return skillSearchSnapshot{
		Query:      query,
		SearchType: searchType,
		Skills:     skills,
		Count:      count,
		DurationMS: durationMS,
	}
}

func codexHomeDir() string {
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		return resolveWorkspaceLocalPath(codexHome)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}

func npxCommandName() string {
	if runtime.GOOS == "windows" {
		return "npx.cmd"
	}
	return "npx"
}

func jsonString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmptyEnv(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
