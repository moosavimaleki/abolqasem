package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjectRunnableScriptsListsRootFilesPackageAndMakeTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "setup.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"test":"bun test","dev":"bun run dev"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bun.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte("build:\n\t@echo build\n.PHONY: build\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scripts := discoverProjectRunnableScripts(root)
	byID := map[string]projectRunnableScript{}
	for _, script := range scripts {
		byID[script.ID] = script
	}
	if byID["file:setup.sh"].Command != "./setup.sh" {
		t.Fatalf("shell script missing: %#v", scripts)
	}
	if byID["package:dev"].Command != "bun run dev" || byID["package:test"].Command != "bun run test" {
		t.Fatalf("package scripts missing: %#v", scripts)
	}
	if byID["make:build"].Command != "make build" {
		t.Fatalf("make target missing: %#v", scripts)
	}
}
