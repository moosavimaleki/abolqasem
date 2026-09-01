package maintenance

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type Job func(context.Context) error

type Scheduler struct {
	Interval       time.Duration
	Jitter         time.Duration
	Backoff        time.Duration
	MaxBackoff     time.Duration
	MaxAttempts    int
	Now            func() time.Time
	Rand           *rand.Rand
	RunImmediately bool
	running        chan struct{}
	once           sync.Once
}

func (s *Scheduler) Start(ctx context.Context, job Job) {
	if s.Interval <= 0 {
		s.Interval = time.Hour
	}
	if s.Backoff <= 0 {
		s.Backoff = time.Second
	}
	if s.MaxBackoff <= 0 {
		s.MaxBackoff = 15 * time.Minute
	}
	if s.MaxAttempts <= 0 {
		s.MaxAttempts = 3
	}
	s.once.Do(func() { s.running = make(chan struct{}, 1) })
	go func() {
		if s.RunImmediately {
			s.run(ctx, job)
		}
		timer := time.NewTimer(s.nextDelay())
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.run(ctx, job)
				timer.Reset(s.nextDelay())
			}
		}
	}()
}

func (s *Scheduler) run(ctx context.Context, job Job) {
	select {
	case s.running <- struct{}{}:
		defer func() { <-s.running }()
		delay := s.Backoff
		for attempt := 0; attempt < s.MaxAttempts; attempt++ {
			if err := job(ctx); err == nil || ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < s.MaxBackoff {
				delay *= 2
				if delay > s.MaxBackoff {
					delay = s.MaxBackoff
				}
			}
		}
	default:
		// A previous run is still in flight; never overlap maintenance jobs.
	}
}

func (s *Scheduler) nextDelay() time.Duration {
	delay := s.Interval
	if s.Jitter <= 0 {
		return delay
	}
	random := s.Rand
	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return delay + time.Duration(random.Int63n(int64(s.Jitter)+1))
}
