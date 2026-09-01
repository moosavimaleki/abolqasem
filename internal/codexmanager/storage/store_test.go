package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteJSONIsAtomicAndRotatesBackups(t *testing.T) {
	paths := Paths{Home: t.TempDir()}
	path, err := paths.Account("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(paths, path, map[string]string{"value": "one"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(paths, path, map[string]string{"value": "two"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := ReadJSON(path, &value); err != nil || value["value"] != "two" {
		t.Fatalf("got %#v, %v", value, err)
	}
	backup, err := os.ReadFile(path + ".BAK1")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) == "" || filepath.Base(path) != "alpha.json" {
		t.Fatal("backup missing")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestPathsRejectTraversalAndLockRuns(t *testing.T) {
	paths := Paths{Home: t.TempDir()}
	if _, err := paths.Account("../bad"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	called := false
	if err := WithLock(context.Background(), paths, func() error { called = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("lock callback was not called")
	}
}

func TestWithLockHonorsContextWhileAnotherCallerHoldsTheFile(t *testing.T) {
	paths := Paths{Home: t.TempDir()}
	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = WithLock(context.Background(), paths, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := WithLock(ctx, paths, func() error { return nil }); err == nil {
		t.Fatal("expected lock acquisition to time out")
	}
	close(release)
}
