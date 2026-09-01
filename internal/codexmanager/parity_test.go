package codexmanager_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"abolqasem/internal/codexmanager/history"
)

func TestGoldenFixturesAreRedactedAndDecodable(t *testing.T) {
	root := "testdata"
	auth, err := os.ReadFile(filepath.Join(root, "account_auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(auth, []byte("REDACTED_ACCESS_TOKEN")) || bytes.Contains(auth, []byte("sk-")) {
		t.Fatalf("auth fixture is not safely redacted: %s", auth)
	}
	var decoded map[string]any
	if err := json.Unmarshal(auth, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["email"] != "fixture@example.invalid" {
		t.Fatal("fixture identity changed")
	}
	historyData, err := os.ReadFile(filepath.Join(root, "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(historyData), []byte("\n")) {
		var sample history.Sample
		if err := json.Unmarshal(line, &sample); err != nil {
			t.Fatal(err)
		}
		if sample.Account != "fixture" || sample.At.IsZero() {
			t.Fatalf("invalid history sample: %#v", sample)
		}
	}
}
