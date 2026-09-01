package storage

import (
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var accountNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)

type Paths struct{ Home string }

func (p Paths) AccountsDir() string { return filepath.Join(p.Home, "accounts") }
func (p Paths) StatusDir() string   { return filepath.Join(p.Home, "status") }
func (p Paths) HistoryDir() string  { return filepath.Join(p.Home, "history") }
func (p Paths) HistoryFile() string { return filepath.Join(p.HistoryDir(), "rate-limits.jsonl") }
func (p Paths) StateFile() string   { return filepath.Join(p.Home, "state.json") }
func (p Paths) LockFile() string    { return filepath.Join(p.Home, "lock") }
func (p Paths) Account(name string) (string, error) {
	name, err := SanitizeAccountName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(p.AccountsDir(), name+".json"), nil
}
func (p Paths) Status(name string) (string, error) {
	name, err := SanitizeAccountName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(p.StatusDir(), name+".json"), nil
}

func SanitizeAccountName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !accountNamePattern.MatchString(name) {
		return "", errors.New("account name must match [A-Za-z0-9._-] and be <= 80 chars")
	}
	return name, nil
}
func SortedAccountNames(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}
