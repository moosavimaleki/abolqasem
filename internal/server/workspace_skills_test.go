package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestParseInstalledSkillsLock(t *testing.T) {
	snapshot := parseInstalledSkillsLock(map[string]any{
		"version": float64(1),
		"skills": map[string]any{
			"zeta": map[string]any{
				"source":      "owner/zeta",
				"sourceType":  "github",
				"sourceUrl":   "https://github.com/owner/zeta",
				"skillPath":   "skills/zeta/SKILL.md",
				"installedAt": "2026-05-01T01:00:00.000Z",
				"updatedAt":   "2026-05-01T02:00:00.000Z",
				"pluginName":  "zeta-plugin",
			},
			"alpha": map[string]any{
				"source":     "owner/alpha",
				"sourceType": "github",
			},
			"ignored": "not an object",
		},
	}, "/tmp/.skill-lock.json")

	if snapshot.LockFilePath != "/tmp/.skill-lock.json" {
		t.Fatalf("unexpected lock path: %q", snapshot.LockFilePath)
	}
	names := []string{}
	for _, skill := range snapshot.Skills {
		names = append(names, skill.Name)
	}
	if !reflect.DeepEqual(names, []string{"alpha", "zeta"}) {
		t.Fatalf("unexpected skill names: %#v", names)
	}
	if snapshot.Skills[0].Source != "owner/alpha" || snapshot.Skills[0].SourceURL != "" {
		t.Fatalf("unexpected alpha skill: %#v", snapshot.Skills[0])
	}
	if snapshot.Skills[1].SkillPath != "skills/zeta/SKILL.md" || snapshot.Skills[1].PluginName != "zeta-plugin" {
		t.Fatalf("unexpected zeta skill: %#v", snapshot.Skills[1])
	}
}

func TestListInstalledSkillsReadsLockAndFallsBackToCodexSkills(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".skill-lock.json")
	if err := os.WriteFile(lockPath, []byte(`{"skills":{"alpha":{"source":"owner/alpha","sourceType":"github"}}}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	locked := listInstalledSkills(lockPath)
	if len(locked.Skills) != 1 || locked.Skills[0].Name != "alpha" {
		t.Fatalf("unexpected locked skills: %#v", locked)
	}

	codexHome := filepath.Join(dir, "codex")
	if err := os.MkdirAll(filepath.Join(codexHome, "skills", "local-skill"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "skills", "local-skill", "SKILL.md"), []byte("# Local"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	fallback := listInstalledSkills(filepath.Join(dir, "missing.json"))
	if len(fallback.Skills) != 1 || fallback.Skills[0].Name != "local-skill" || fallback.Skills[0].SourceType != "codex" {
		t.Fatalf("unexpected fallback skills: %#v", fallback)
	}
}

func TestSkillsSearchNormalizesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "test" || r.URL.Query().Get("limit") != "5" {
			t.Fatalf("unexpected search query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":       "test",
			"searchType":  "fuzzy",
			"count":       2,
			"duration_ms": 12,
			"skills": []map[string]any{
				{"id": "owner/repo", "skillId": "repo", "name": "Repo", "installs": 7, "source": "owner/repo"},
				{"id": "bad"},
			},
		})
	}))
	defer server.Close()

	previous := skillsSearchBaseURL
	skillsSearchBaseURL = server.URL
	t.Cleanup(func() { skillsSearchBaseURL = previous })

	snapshot, err := searchSkills(" test ", 5)
	if err != nil {
		t.Fatalf("searchSkills returned error: %v", err)
	}
	if snapshot.Query != "test" || snapshot.Count != 2 || len(snapshot.Skills) != 1 {
		t.Fatalf("unexpected search snapshot: %#v", snapshot)
	}
}

func TestSkillCommandValidationAndBuilders(t *testing.T) {
	source, err := assertSafeSkillSource(" owner/repo ")
	if err != nil || source != "owner/repo" {
		t.Fatalf("unexpected safe source result: %q %v", source, err)
	}
	skillID, err := assertSafeSkillID(" my-skill_1 ")
	if err != nil || skillID != "my-skill_1" {
		t.Fatalf("unexpected safe skill id result: %q %v", skillID, err)
	}
	if _, err := assertSafeSkillSource("https://github.com/owner/repo"); err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("expected unsafe source error, got %v", err)
	}
	if _, err := assertSafeSkillID("../nope"); err == nil || err.Error() != "Skill id is invalid." {
		t.Fatalf("expected unsafe skill id error, got %v", err)
	}

	installCommand, err := buildInstallSkillCommand("owner/repo", "my-skill")
	if err != nil {
		t.Fatalf("buildInstallSkillCommand returned error: %v", err)
	}
	if !reflect.DeepEqual(installCommand[1:], []string{"skills", "add", "owner/repo", "--skill", "my-skill", "--global", "--agent", "universal", "claude-code", "--yes"}) {
		t.Fatalf("unexpected install command: %#v", installCommand)
	}
	uninstallCommand, err := buildUninstallSkillCommand("my-skill")
	if err != nil {
		t.Fatalf("buildUninstallSkillCommand returned error: %v", err)
	}
	if !reflect.DeepEqual(uninstallCommand[1:], []string{"skills", "remove", "my-skill", "--global", "--agent", "universal", "claude-code", "--yes"}) {
		t.Fatalf("unexpected uninstall command: %#v", uninstallCommand)
	}
}

func TestInstallAndUninstallSkillReturnAbolqasemShape(t *testing.T) {
	previous := runSkillCLICommand
	runSkillCLICommand = func(command []string) (skillCommandOutput, error) {
		return skillCommandOutput{CWD: "/home/test", Stdout: "ok", Stderr: ""}, nil
	}
	t.Cleanup(func() { runSkillCLICommand = previous })

	installed, err := installSkill("owner/repo", "my-skill")
	if err != nil {
		t.Fatalf("installSkill returned error: %v", err)
	}
	if installed.Source != "owner/repo" || installed.SkillID != "my-skill" || installed.CWD != "/home/test" || installed.Stdout != "ok" {
		t.Fatalf("unexpected install result: %#v", installed)
	}

	uninstalled, err := uninstallSkill("my-skill")
	if err != nil {
		t.Fatalf("uninstallSkill returned error: %v", err)
	}
	if uninstalled.SkillID != "my-skill" || uninstalled.CWD != "/home/test" {
		t.Fatalf("unexpected uninstall result: %#v", uninstalled)
	}
}

func TestWorkspaceInstallSkillTracksAndDeduplicatesActiveOperation(t *testing.T) {
	previousCommand := runSkillCLICommand
	previousOperations := skillOperations
	skillOperations = newSkillOperationTracker()
	t.Cleanup(func() {
		runSkillCLICommand = previousCommand
		skillOperations = previousOperations
	})

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var calls atomic.Int32
	runSkillCLICommand = func(command []string) (skillCommandOutput, error) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		return skillCommandOutput{CWD: "/home/test", Stdout: "ok"}, nil
	}

	firstOperation, err := skillOperations.startInstall("owner/repo", "repo")
	if err != nil {
		t.Fatalf("startInstall returned error: %v", err)
	}
	<-started

	secondOperation, err := skillOperations.startInstall("owner/repo", "repo")
	if err != nil {
		t.Fatalf("second startInstall returned error: %v", err)
	}
	if firstOperation != secondOperation {
		t.Fatalf("expected duplicate active install to reuse the active operation")
	}

	snapshot := workspaceListSkillOperations()
	if len(snapshot.Operations) != 1 {
		t.Fatalf("expected one active operation, got %#v", snapshot.Operations)
	}
	if snapshot.Operations[0].Status != skillOperationRunning {
		t.Fatalf("expected running operation, got %#v", snapshot.Operations[0])
	}

	close(release)
	summary := skillOperations.wait(firstOperation)
	if summary.Status != skillOperationSucceeded {
		t.Fatalf("expected succeeded operation, got %#v", summary)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one CLI call for duplicate active install, got %d", calls.Load())
	}
}
