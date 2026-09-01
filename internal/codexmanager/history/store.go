package history

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/storage"
)

const maxReadSamples = 20_000

type Store struct {
	Paths storage.Paths
}

func (s Store) Append(ctx context.Context, snapshot limits.Snapshot) (bool, error) {
	sample, ok := sampleFromSnapshot(snapshot)
	if !ok {
		return false, nil
	}
	appended := false
	err := storage.WithLock(ctx, s.Paths, func() error {
		if duplicate, err := lastEquals(s.Paths.HistoryFile(), sample); err != nil {
			return err
		} else if duplicate {
			return nil
		}
		file, err := os.OpenFile(s.Paths.HistoryFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		data, err := json.Marshal(sample)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		appended = true
		return nil
	})
	return appended, err
}

func (s Store) Read(account string, since time.Time, max int) ([]Sample, error) {
	return s.ReadPage(account, since, time.Time{}, max)
}

// ReadPage returns the newest samples before before (when set) in ascending
// order. Keeping the cursor as a timestamp lets the HTTP layer paginate a
// large JSONL history without loading it all into the settings UI.
func (s Store) ReadPage(account string, since, before time.Time, max int) ([]Sample, error) {
	if max <= 0 || max > maxReadSamples {
		max = maxReadSamples
	}
	file, err := os.Open(s.Paths.HistoryFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make([]Sample, 0, min(max, 256))
	if err := scan(file, func(sample Sample) error {
		if sample.Account != account || (!since.IsZero() && sample.At.Before(since)) || (!before.IsZero() && !sample.At.Before(before)) {
			return nil
		}
		if len(result) == max {
			copy(result, result[1:])
			result[len(result)-1] = sample
			return nil
		}
		result = append(result, sample)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].At.Before(result[j].At) })
	return result, nil
}

func (s Store) Prune(ctx context.Context, retention time.Duration, now time.Time) (int, error) {
	if retention <= 0 {
		return 0, fmt.Errorf("retention must be positive")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.Add(-retention)
	removed := 0
	err := storage.WithLock(ctx, s.Paths, func() error {
		return s.rewrite(func(sample Sample) (Sample, bool) {
			if sample.At.Before(cutoff) {
				removed++
				return Sample{}, false
			}
			return sample, true
		})
	})
	return removed, err
}

func (s Store) Rename(ctx context.Context, oldName, newName string) (int, error) {
	return s.rename(ctx, oldName, newName, true)
}

// RenameLocked is for a caller that already holds storage.WithLock for the
// same Manager home, such as the account rename transaction.
func (s Store) RenameLocked(oldName, newName string) (int, error) {
	return s.rename(context.Background(), oldName, newName, false)
}

func (s Store) rename(ctx context.Context, oldName, newName string, lock bool) (int, error) {
	if _, err := storage.SanitizeAccountName(oldName); err != nil {
		return 0, err
	}
	if _, err := storage.SanitizeAccountName(newName); err != nil {
		return 0, err
	}
	renamed := 0
	rename := func() error {
		return s.rewrite(func(sample Sample) (Sample, bool) {
			if sample.Account == oldName {
				sample.Account = newName
				renamed++
			}
			return sample, true
		})
	}
	var err error
	if lock {
		err = storage.WithLock(ctx, s.Paths, rename)
	} else {
		err = rename()
	}
	return renamed, err
}

func (s Store) Series(account, window string, since time.Time, maxPoints int) (Series, error) {
	return s.SeriesIn(account, window, since, maxPoints, "UTC")
}

// SeriesIn localizes returned points without changing persisted UTC timestamps.
// Accepted offsets are UTC, local, +03:30 and -07:00.
func (s Store) SeriesIn(account, window string, since time.Time, maxPoints int, offset string) (Series, error) {
	zone, label, err := parseOffset(offset)
	if err != nil {
		return Series{}, err
	}
	samples, err := s.Read(account, since, maxReadSamples)
	if err != nil {
		return Series{}, err
	}
	points := make([]Point, 0, len(samples))
	for _, sample := range samples {
		if value, ok := sample.Windows[window]; ok {
			points = append(points, Point{At: sample.At.In(zone), Value: value})
		}
	}
	if maxPoints > 0 && len(points) > maxPoints {
		points = downsample(points, maxPoints)
	}
	return Series{Account: account, Window: window, Timezone: label, Points: points}, nil
}

func (s Store) rewrite(transform func(Sample) (Sample, bool)) error {
	input, err := os.Open(s.Paths.HistoryFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(s.Paths.HistoryDir(), ".rate-limits.*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	writer := bufio.NewWriter(temporary)
	err = scan(input, func(sample Sample) error {
		next, keep := transform(sample)
		if !keep {
			return nil
		}
		data, err := json.Marshal(next)
		if err != nil {
			return err
		}
		_, err = writer.Write(append(data, '\n'))
		return err
	})
	if flushErr := writer.Flush(); err == nil {
		err = flushErr
	}
	if syncErr := temporary.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryName, s.Paths.HistoryFile())
}

func sampleFromSnapshot(snapshot limits.Snapshot) (Sample, bool) {
	for _, limit := range snapshot.Limits {
		if limit.ID != "codex" {
			continue
		}
		windows := make(map[string]float64, len(limit.Windows))
		for _, window := range limit.Windows {
			windows[window.Label] = window.RemainingPercent
		}
		if len(windows) == 0 || snapshot.Account == "" {
			return Sample{}, false
		}
		return Sample{Account: snapshot.Account, At: snapshot.FetchedAt.UTC(), Plan: snapshot.Plan, Windows: windows}, true
	}
	return Sample{}, false
}

func lastEquals(path string, sample Sample) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	var last Sample
	if err := scan(file, func(current Sample) error { last = current; return nil }); err != nil {
		return false, err
	}
	return last.Account == sample.Account && last.At.Equal(sample.At) && mapsEqual(last.Windows, sample.Windows), nil
}

func scan(reader io.Reader, visit func(Sample) error) error {
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		var sample Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil || sample.Account == "" || sample.At.IsZero() {
			continue
		}
		if err := visit(sample); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func downsample(points []Point, limit int) []Point {
	if limit < 2 {
		return []Point{points[len(points)-1]}
	}
	result := make([]Point, 0, limit)
	for index := 0; index < limit; index++ {
		position := index * (len(points) - 1) / (limit - 1)
		result = append(result, points[position])
	}
	return result
}

func mapsEqual(left, right map[string]float64) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func parseOffset(value string) (*time.Location, string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "UTC") {
		return time.UTC, "UTC+00:00", nil
	}
	if strings.EqualFold(value, "local") {
		_, offset := time.Now().Zone()
		return time.Local, formatOffset(offset), nil
	}
	sign := 1
	if value[0] == '-' {
		sign = -1
		value = value[1:]
	} else if value[0] == '+' {
		value = value[1:]
	} else {
		return nil, "", fmt.Errorf("offset must look like UTC, local, +03:30, or -07:00")
	}
	parts := strings.Split(value, ":")
	if len(parts) > 2 || len(parts) == 0 {
		return nil, "", fmt.Errorf("offset must look like UTC, local, +03:30, or -07:00")
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, "", fmt.Errorf("offset must look like UTC, local, +03:30, or -07:00")
	}
	minutes := 0
	if len(parts) == 2 {
		minutes, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, "", fmt.Errorf("offset must look like UTC, local, +03:30, or -07:00")
		}
	}
	if hours > 23 || minutes > 59 || hours < 0 || minutes < 0 {
		return nil, "", fmt.Errorf("offset is out of range")
	}
	seconds := sign * (hours*3600 + minutes*60)
	return time.FixedZone(formatOffset(seconds), seconds), formatOffset(seconds), nil
}

func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}
