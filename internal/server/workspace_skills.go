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
	"time"
)

const skillsSearchDefaultLimit = 100

var (
	safeSkillSourceRE   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	safeSkillIDRE       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	skillsSearchBaseURL = "https://skills.sh/api/search"
	runSkillCLICommand  = defaultRunSkillCLICommand
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

type skillCommandOutput struct {
	CWD    string
	Stdout string
	Stderr string
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
	return installSkill(payload.Source, payload.SkillID)
}

func workspaceUninstallSkill(raw json.RawMessage) (skillUninstallResult, error) {
	var payload struct {
		SkillID string `json:"skillId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return skillUninstallResult{}, err
	}
	return uninstallSkill(payload.SkillID)
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
