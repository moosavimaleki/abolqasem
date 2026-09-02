package boundedlog

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFileCapsWritesWithoutFailingTheCaller(t *testing.T) {
	log, path, err := Open(t.TempDir(), "test-", "test-current.log")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	data := bytes.Repeat([]byte("x"), int(maxFileBytes)+1024)
	written, err := log.Write(data)
	if err != nil || written != len(data) {
		t.Fatalf("Write() = (%d, %v), want (%d, nil)", written, err, len(data))
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != maxFileBytes {
		t.Fatalf("log size = %d, want %d", info.Size(), maxFileBytes)
	}
}

func TestOpenPrunesOldMatchingLogs(t *testing.T) {
	directory := t.TempDir()
	for index := range maxFiles {
		path := filepath.Join(directory, "codex-old-"+strconv.Itoa(index)+".log")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log, _, err := Open(directory, "codex-", "codex-current.log")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) >= maxFiles+1 {
		t.Fatalf("matching logs were not pruned: %d entries", len(entries))
	}
}
