package history

import (
	"context"
	"testing"
	"time"

	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/storage"
)

func historySnapshot(account string, at time.Time, primary, weekly float64) limits.Snapshot {
	return limits.Snapshot{Account: account, Plan: "plus", FetchedAt: at, Limits: []limits.Limit{{ID: "codex", Windows: []limits.Window{{Label: "5h", RemainingPercent: primary}, {Label: "weekly", RemainingPercent: weekly}}}}}
}

func TestStoreAppendReadRenamePruneAndSeries(t *testing.T) {
	store := Store{Paths: storage.Paths{Home: t.TempDir()}}
	ctx := context.Background()
	start := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 4; index++ {
		appended, err := store.Append(ctx, historySnapshot("alpha", start.Add(time.Duration(index)*time.Hour), float64(80-index), float64(60-index)))
		if err != nil || !appended {
			t.Fatalf("append %d: appended=%v err=%v", index, appended, err)
		}
	}
	appended, err := store.Append(ctx, historySnapshot("alpha", start.Add(3*time.Hour), 77, 57))
	if err != nil || appended {
		t.Fatalf("duplicate append: appended=%v err=%v", appended, err)
	}
	series, err := store.SeriesIn("alpha", "weekly", start, 2, "+03:30")
	if err != nil || series.Timezone != "UTC+03:30" || len(series.Points) != 2 || series.Points[0].Value != 60 || series.Points[1].Value != 57 || series.Points[0].At.Hour() != 11 {
		t.Fatalf("series=%#v err=%v", series, err)
	}
	renamed, err := store.Rename(ctx, "alpha", "beta")
	if err != nil || renamed != 4 {
		t.Fatalf("renamed=%d err=%v", renamed, err)
	}
	removed, err := store.Prune(ctx, 90*time.Minute, start.Add(4*time.Hour))
	if err != nil || removed != 3 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	rows, err := store.Read("beta", time.Time{}, 10)
	if err != nil || len(rows) != 1 || rows[0].Windows["5h"] != 77 {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
}

func TestStoreReadPageReturnsBoundedNewestSamplesBeforeCursor(t *testing.T) {
	store := Store{Paths: storage.Paths{Home: t.TempDir()}}
	ctx := context.Background()
	start := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 5; index++ {
		if _, err := store.Append(ctx, historySnapshot("alpha", start.Add(time.Duration(index)*time.Hour), float64(index), float64(index))); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.ReadPage("alpha", start, start.Add(5*time.Hour), 2)
	if err != nil || len(page) != 2 || !page[0].At.Equal(start.Add(3*time.Hour)) || !page[1].At.Equal(start.Add(4*time.Hour)) {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	older, err := store.ReadPage("alpha", start, page[0].At, 2)
	if err != nil || len(older) != 2 || !older[0].At.Equal(start.Add(time.Hour)) || !older[1].At.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("older page=%#v err=%v", older, err)
	}
}
