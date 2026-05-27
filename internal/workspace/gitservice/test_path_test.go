package gitservice

import (
	"path/filepath"
	"testing"
)

func canonicalGitserviceTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks %s failed: %v", path, err)
	}
	return canonical
}
