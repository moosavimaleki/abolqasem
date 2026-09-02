// Package boundedlog provides small, disk-safe diagnostic log files.
package boundedlog

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	maxFileBytes  int64 = 768 * 1024
	maxTotalBytes int64 = 16 * 1024 * 1024
	maxFiles            = 48
)

// File accepts writes until its size cap is reached. Further writes are
// intentionally discarded while still reporting success to avoid disrupting
// the process being diagnosed.
type File struct {
	file      *os.File
	mu        sync.Mutex
	written   int64
	capped    bool
	closeOnce sync.Once
}

// Open creates a bounded log in directory and prunes old matching logs before
// returning. Prefix is part of the file name and scopes retention safely.
func Open(directory string, prefix string, name string) (*File, string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, "", err
	}
	prune(directory, prefix)
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}
	return &File{file: file, written: info.Size(), capped: info.Size() >= maxFileBytes}, path, nil
}

func (f *File) Write(data []byte) (int, error) {
	if f == nil || len(data) == 0 {
		return len(data), nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil || f.capped {
		return len(data), nil
	}
	remaining := maxFileBytes - f.written
	if remaining <= 0 {
		f.capped = true
		return len(data), nil
	}
	toWrite := data
	if int64(len(toWrite)) > remaining {
		toWrite = toWrite[:remaining]
		f.capped = true
	}
	written, err := f.file.Write(toWrite)
	f.written += int64(written)
	if err != nil {
		return len(data), err
	}
	return len(data), nil
}

func (f *File) Close() error {
	if f == nil {
		return nil
	}
	var err error
	f.closeOnce.Do(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.file != nil {
			err = f.file.Close()
			f.file = nil
		}
	})
	return err
}

type logEntry struct {
	path    string
	modTime int64
	size    int64
}

func prune(directory string, prefix string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	logs := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logEntry{
			path:    filepath.Join(directory, entry.Name()),
			modTime: info.ModTime().UnixNano(),
			size:    info.Size(),
		})
	}
	sort.Slice(logs, func(left int, right int) bool { return logs[left].modTime < logs[right].modTime })
	var total int64
	for _, entry := range logs {
		total += entry.size
	}
	for len(logs) >= maxFiles || total > maxTotalBytes {
		oldest := logs[0]
		logs = logs[1:]
		if err := os.Remove(oldest.path); err == nil {
			total -= oldest.size
		}
	}
}

var _ io.WriteCloser = (*File)(nil)
