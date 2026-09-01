package codexmanager

import (
	"context"
	"fmt"
	"testing"
	"time"

	"abolqasem/internal/codexmanager/history"
	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/storage"
)

func BenchmarkHistoryAppendAndRead(b *testing.B) {
	paths := storage.Paths{Home: b.TempDir()}
	store := history.Store{Paths: paths}
	for index := 0; index < 500; index++ {
		_, _ = store.Append(context.Background(), limits.Snapshot{Account: "bench", FetchedAt: time.Now().Add(-time.Duration(500-index) * time.Minute), Limits: []limits.Limit{{ID: fmt.Sprint(index), Windows: []limits.Window{{Label: "weekly", RemainingPercent: float64(index % 100)}}}}})
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_, _ = store.Read("bench", time.Time{}, 120)
	}
}
