package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abolqasem/internal/codexmanager/storage"
)

func TestPlanAndImportAreIdempotent(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "accounts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "accounts", "blue.json"), []byte(`{"tokens":{"refresh_token":"x"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	paths := storage.Paths{Home: target}
	plan, err := BuildPlan(source, paths)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := Import(context.Background(), plan, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 1 {
		t.Fatalf("expected one copied file, got %v", copied)
	}
	second, err := Import(context.Background(), plan, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("expected second import to be no-op, got %v", second)
	}
}

func TestPlanExcludesLegacyBackupFiles(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "accounts"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"active.json", "active.json.BAK1", "unrelated.txt"} {
		if err := os.WriteFile(filepath.Join(source, "accounts", name), []byte(`{}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := BuildPlan(source, storage.Paths{Home: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0] != "accounts/active.json" {
		t.Fatalf("unexpected migration files: %#v", plan.Files)
	}
}

func TestImportConvertsAndMergesLegacyHistoryWithoutConfigSecrets(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "history"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.json"), []byte(`{"gateway_api_key":"must-not-copy"}`), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := "{\"account\":\"alpha\",\"recorded_at\":\"2026-01-02T03:04:05Z\",\"plan_type\":\"plus\",\"primary_remaining_percent\":80,\"secondary_remaining_percent\":60}\n"
	if err := os.WriteFile(filepath.Join(source, "history", "rate-limits.jsonl"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	paths := storage.Paths{Home: target}
	plan, err := BuildPlan(source, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0] != "history/rate-limits.jsonl" {
		t.Fatalf("unexpected migration plan: %#v", plan.Files)
	}
	if _, err := Import(context.Background(), plan, paths); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "history", "rate-limits.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == legacy || !contains(string(data), `"primary":80`) || !contains(string(data), `"secondary":60`) {
		t.Fatalf("legacy history was not normalized: %s", data)
	}
	if _, err := os.Stat(filepath.Join(target, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy config was copied: %v", err)
	}
	again, err := Import(context.Background(), plan, paths)
	if err != nil || len(again) != 0 {
		t.Fatalf("history import must be idempotent: copied=%v err=%v", again, err)
	}
}

func TestImportAcceptsOriginalLimitsFileAndFoldsDuplicateIdentity(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	for _, root := range []string{"accounts", "status", "history"} {
		if err := os.MkdirAll(filepath.Join(source, root), 0700); err != nil {
			t.Fatal(err)
		}
	}
	credential := []byte(`{"email":"same@example.com","tokens":{"refresh_token":"shared"}}`)
	if err := os.WriteFile(filepath.Join(source, "accounts", "first.json"), credential, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "accounts", "second.json"), credential, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "status", "second.json"), []byte(`{"state":"ok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := "{\"account\":\"first\",\"recorded_at\":\"2026-01-02T03:04:05Z\",\"plan_type\":\"plus\",\"primary_remaining_percent\":80}\n"
	if err := os.WriteFile(filepath.Join(source, "history", "limits.jsonl"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	paths := storage.Paths{Home: target}
	plan, err := BuildPlan(source, paths)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := Import(context.Background(), plan, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 2 || copied[0] != "accounts/first.json" || copied[1] != "history/rate-limits.jsonl" {
		t.Fatalf("unexpected copied paths: %#v", copied)
	}
	if _, err := os.Stat(filepath.Join(target, "accounts", "second.json")); !os.IsNotExist(err) {
		t.Fatalf("duplicate identity must not create a second account: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "status", "second.json")); !os.IsNotExist(err) {
		t.Fatalf("duplicate identity must not create orphan status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "history", "rate-limits.jsonl")); err != nil {
		t.Fatalf("original history filename was not migrated: %v", err)
	}
}

func contains(value, want string) bool { return strings.Contains(value, want) }

func TestImportRollsBackFilesCreatedByFailedRun(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "accounts"), 0700); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(source, "accounts", "a.json")
	second := filepath.Join(source, "accounts", "b.json")
	if err := os.WriteFile(first, []byte(`{"tokens":{"refresh_token":"a"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{"tokens":{"refresh_token":"b"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	paths := storage.Paths{Home: target}
	plan, err := BuildPlan(source, paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(context.Background(), plan, paths); err == nil {
		t.Fatal("expected failed import")
	}
	if _, err := os.Stat(filepath.Join(target, "accounts", "a.json")); !os.IsNotExist(err) {
		t.Fatalf("rollback left copied file: %v", err)
	}
}
