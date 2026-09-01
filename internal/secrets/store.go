package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"abolqasem/internal/state"
)

var mu sync.Mutex
var rootDir = state.GetStateDir

func path(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") {
		return "", errors.New("invalid secret name")
	}
	return filepath.Join(rootDir(), "secrets", name), nil
}

func Put(name, value string) error {
	file, err := path(name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("secret value is empty")
	}
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte(value), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func Get(name string) (string, error) {
	file, err := path(name)
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()
	value, err := os.ReadFile(file)
	return strings.TrimSpace(string(value)), err
}

func Configured(name string) bool {
	value, err := Get(name)
	return err == nil && value != ""
}
