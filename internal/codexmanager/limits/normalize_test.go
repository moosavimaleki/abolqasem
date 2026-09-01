package limits

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeGoldenPayload(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "rate_limits.json"))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		t.Fatal(err)
	}
	fetched := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	snapshot := Normalize(payload, fetched)
	if snapshot.Plan != "plus" || snapshot.ReachedType != "weekly" || len(snapshot.Limits) != 2 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	codex := snapshot.Limits[0]
	if codex.ID != "codex" || len(codex.Windows) != 2 || codex.Windows[0].RemainingPercent != 82.5 || codex.Windows[1].Label != "weekly" {
		t.Fatalf("unexpected codex limit: %#v", codex)
	}
	if codex.Windows[0].ResetAt == nil || !codex.Windows[0].ResetAt.Equal(time.Unix(1770000000, 0).UTC()) {
		t.Fatalf("unexpected reset: %#v", codex.Windows[0].ResetAt)
	}
}

func TestNormalizeUsesResetAfterAndClampsPercent(t *testing.T) {
	snapshot := Normalize(map[string]any{"rate_limit": map[string]any{"primary_window": map[string]any{"used_percent": 102, "reset_after_seconds": 30}}}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	window := snapshot.Limits[0].Windows[0]
	if window.RemainingPercent != 0 || !window.Reached || window.ResetAt == nil || !window.ResetAt.Equal(snapshot.FetchedAt.Add(30*time.Second)) {
		t.Fatalf("unexpected normalized window: %#v", window)
	}
}
