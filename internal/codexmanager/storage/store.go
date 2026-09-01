package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const backupCount = 5

func EnsureDirs(paths Paths) error {
	for _, dir := range []string{paths.Home, paths.AccountsDir(), paths.StatusDir(), paths.HistoryDir()} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}

func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	return nil
}

func WriteJSON(paths Paths, path string, value any) error {
	if !isWithin(paths.Home, path) {
		return errors.New("refusing to write outside manager home")
	}
	if err := EnsureDirs(paths); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := os.Stat(path); err == nil {
		if err := rotateBackups(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func rotateBackups(path string) error {
	for index := backupCount; index >= 1; index-- {
		from := fmt.Sprintf("%s.BAK%d", path, index)
		if index == backupCount {
			_ = os.Remove(from)
			continue
		}
		to := fmt.Sprintf("%s.BAK%d", path, index+1)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".BAK1", data, 0600)
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && rel != ""
}
