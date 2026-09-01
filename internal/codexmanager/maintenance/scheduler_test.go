package maintenance

import (
	"context"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerRetriesWithBackoffAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	done := make(chan struct{})
	scheduler := &Scheduler{Interval: time.Hour, Backoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, MaxAttempts: 3, RunImmediately: true}
	scheduler.Start(ctx, func(context.Context) error {
		if calls.Add(1) == 3 {
			close(done)
		}
		return context.Canceled
	})
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("scheduler did not retry")
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestSchedulerJitterIsBounded(t *testing.T) {
	scheduler := Scheduler{Interval: time.Second, Jitter: 500 * time.Millisecond, Rand: rand.New(rand.NewSource(1))}
	for index := 0; index < 10; index++ {
		delay := scheduler.nextDelay()
		if delay < time.Second || delay > 1500*time.Millisecond {
			t.Fatalf("delay=%v", delay)
		}
	}
}
