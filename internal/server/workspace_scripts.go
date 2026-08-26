package server

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxProjectRunnableScripts = 100

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

// discoverProjectRunnableScripts discovers runnable project entry points without
// executing them. Script files are collected recursively, while dependency and
// build-output directories are skipped so a large checkout stays responsive.
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

	var filePaths []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && skippedRunnableScriptDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isRunnableScriptFile(path, entry) {
			filePaths = append(filePaths, path)
		}
		return nil
	})
	sort.Strings(filePaths)
	for _, path := range filePaths {
		relativePath, err := filepath.Rel(root, path)
		if err != nil || relativePath == "." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		relativePath = filepath.ToSlash(relativePath)
		label := "./" + relativePath
		add(projectRunnableScript{ID: "file:" + relativePath, Label: label, Command: "./" + shellQuote(relativePath), Source: "file"})
	}

	for _, script := range discoverPackageScripts(root) {
		add(script)
	}
	for _, script := range discoverMakeTargets(root) {
		add(script)
	}
	return result
}

func skippedRunnableScriptDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "build", "target", ".next", ".nuxt", ".venv", "venv", "__pycache__":
		return true
	default:
		return false
	}
}

func isRunnableScriptFile(path string, entry os.DirEntry) bool {
	name := strings.ToLower(entry.Name())
	switch filepath.Ext(name) {
	case ".sh", ".bash", ".zsh", ".fish", ".ksh", ".command", ".bat", ".cmd", ".ps1":
		return true
	}
	// Include extensionless executable scripts (and Python/Node scripts with a
	// shebang) so projects that keep entry points under scripts/ are complete.
	info, err := entry.Info()
	if err != nil || info.Mode()&0o111 == 0 {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var prefix [128]byte
	n, err := file.Read(prefix[:])
	return (err == nil || err == io.EOF) && strings.HasPrefix(string(prefix[:n]), "#!")
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
