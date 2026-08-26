package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxProjectRunnableScripts = 40

type projectRunnableScript struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Command string `json:"command"`
	Source  string `json:"source"`
}

func workspaceReadProjectRunnableScripts(raw json.RawMessage) ([]projectRunnableScript, error) {
	projectPath, err := workspaceProjectPathFromCommand(raw)
	if err != nil {
		return nil, err
	}
	return discoverProjectRunnableScripts(projectPath), nil
}

// discoverProjectRunnableScripts only inspects the project root. It never runs
// a file and intentionally avoids deep walking through dependency directories.
func discoverProjectRunnableScripts(projectPath string) []projectRunnableScript {
	root := resolveWorkspaceLocalPath(projectPath)
	seen := map[string]bool{}
	result := make([]projectRunnableScript, 0, 12)
	add := func(script projectRunnableScript) {
		if len(result) >= maxProjectRunnableScripts || script.ID == "" || script.Command == "" || seen[script.ID] {
			return
		}
		seen[script.ID] = true
		result = append(result, script)
	}

	quickActions, _ := readProjectQuickActions(root)
	for _, action := range quickActions {
		add(projectRunnableScript{ID: "saved:" + action.ID, Label: action.Label, Command: action.Command, Source: "saved"})
	}

	entries, err := os.ReadDir(root)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			extension := strings.ToLower(filepath.Ext(name))
			if extension != ".sh" && extension != ".bash" && extension != ".zsh" && extension != ".fish" && extension != ".bat" && extension != ".cmd" {
				continue
			}
			add(projectRunnableScript{ID: "file:" + name, Label: "./" + name, Command: "./" + shellQuote(name), Source: "file"})
		}
	}

	for _, script := range discoverPackageScripts(root) {
		add(script)
	}
	for _, script := range discoverMakeTargets(root) {
		add(script)
	}
	return result
}

func discoverPackageScripts(root string) []projectRunnableScript {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return nil
	}
	runner := "npm run"
	if fileExists(filepath.Join(root, "bun.lock")) || fileExists(filepath.Join(root, "bun.lockb")) {
		runner = "bun run"
	} else if fileExists(filepath.Join(root, "pnpm-lock.yaml")) {
		runner = "pnpm run"
	} else if fileExists(filepath.Join(root, "yarn.lock")) {
		runner = "yarn"
	}
	names := make([]string, 0, len(manifest.Scripts))
	for name := range manifest.Scripts {
		if strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]projectRunnableScript, 0, len(names))
	for _, name := range names {
		result = append(result, projectRunnableScript{ID: "package:" + name, Label: name, Command: runner + " " + shellQuote(name), Source: "package"})
	}
	return result
}

func discoverMakeTargets(root string) []projectRunnableScript {
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		data, err = os.ReadFile(filepath.Join(root, "makefile"))
	}
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	result := make([]projectRunnableScript, 0, 8)
	for _, line := range strings.Split(string(data), "\n") {
		if len(result) >= 12 || strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		target, _, ok := strings.Cut(line, ":")
		target = strings.TrimSpace(target)
		if !ok || target == "" || strings.ContainsAny(target, " $%") || seen[target] {
			continue
		}
		seen[target] = true
		result = append(result, projectRunnableScript{ID: "make:" + target, Label: "make " + target, Command: "make " + shellQuote(target), Source: "make"})
	}
	return result
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func shellQuote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t'\"\\$`;&|<>()[]{}*!?") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}
