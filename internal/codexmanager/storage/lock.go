package storage

import (
	"context"
	"time"

	"github.com/gofrs/flock"
)

func WithLock(ctx context.Context, paths Paths, fn func() error) error {
	if err := EnsureDirs(paths); err != nil {
		return err
	}
	lock := flock.New(paths.LockFile())
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return context.DeadlineExceeded
	}
	defer lock.Unlock() //nolint:errcheck // the OS releases the lock when this process exits.
	return fn()
}
