package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPutGetAndRejectTraversal(t *testing.T) {
	old := rootDir
	tmp := t.TempDir()
	rootDir = func() string { return tmp }
	t.Cleanup(func() { rootDir = old })
	if err := Put("gateway-key", "secret-value"); err != nil {
		t.Fatal(err)
	}
	value, err := Get("gateway-key")
	if err != nil || value != "secret-value" {
		t.Fatalf("got %q, %v", value, err)
	}
	if _, err := path("../escape"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	info, err := os.Stat(filepath.Join(rootDir(), "secrets", "gateway-key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("secret mode is %o", info.Mode().Perm())
	}
}
