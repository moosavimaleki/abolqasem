package migrate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"abolqasem/internal/codexmanager/auth"
	"abolqasem/internal/codexmanager/history"
	"abolqasem/internal/codexmanager/storage"
)

// Plan describes a safe, file-level migration from an older codex-manager
// state directory. Only known data files are considered; executables and
// browser databases are never copied automatically.
type Plan struct {
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Files   []string `json:"files"`
	Skipped []string `json:"skipped,omitempty"`
}

func Candidates(home string) []string {
	if strings.TrimSpace(home) == "" {
		home, _ = os.UserHomeDir()
	}
	return []string{filepath.Join(home, ".codex-manager"), filepath.Join(home, ".local", "share", "codex-manager")}
}

func BuildPlan(source string, target storage.Paths) (Plan, error) {
	if strings.TrimSpace(source) == "" {
		return Plan{}, errors.New("migration source is required")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return Plan{}, err
	}
	if source == filepath.Clean(target.Home) {
		return Plan{}, errors.New("migration source and target must differ")
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("source is not a directory")
		}
		return Plan{}, err
	}
	plan := Plan{Source: source, Target: target.Home}
	// Legacy Manager rotates files as "*.json.BAK<n>". They are recovery
	// material, not distinct accounts/statuses, so importing them would bloat
	// the new store and can resurrect stale credentials. Only canonical JSON
	// records are eligible from the two directory trees.
	for _, relativeDir := range []string{"accounts", "status"} {
		if err := plan.addJSONTree(source, relativeDir); err != nil {
			return Plan{}, err
		}
	}
	// config.json is intentionally excluded. The legacy file contains a gateway
	// key and its scheduler fields do not share Abolqasem's settings schema.
	// A migration must never copy a legacy secret into a new managed store.
	// The Python release called this file limits.jsonl while an earlier Go
	// preview used rate-limits.jsonl. Accept either source name, but always
	// merge into the one current HistoryFile below.
	for _, relative := range []string{"history/limits.jsonl", "history/rate-limits.jsonl", "state.json"} {
		path := filepath.Join(source, relative)
		if _, statErr := os.Stat(path); statErr == nil {
			plan.add(relative)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return Plan{}, statErr
		}
	}
	sort.Strings(plan.Files)
	for _, file := range plan.Files {
		if _, statErr := os.Stat(filepath.Join(target.Home, file)); statErr == nil {
			plan.Skipped = append(plan.Skipped, file)
		}
	}
	return plan, nil
}

func (p *Plan) addJSONTree(source, relativeDir string) error {
	root := filepath.Join(source, relativeDir)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil
		}
		p.add(relativePath(source, path))
		return nil
	})
}

func (p *Plan) add(file string) {
	for _, existing := range p.Files {
		if existing == file {
			return
		}
	}
	p.Files = append(p.Files, file)
}
func relativePath(root, path string) string {
	relative, _ := filepath.Rel(root, path)
	return filepath.ToSlash(relative)
}

// Import merges a known legacy tree in one transaction. It deliberately does
// not import config.json (which can contain a gateway key), backup files, or
// browser data. A failed run restores every changed target, including history.
func Import(ctx context.Context, plan Plan, target storage.Paths) (copied []string, err error) {
	if filepath.Clean(plan.Target) != filepath.Clean(target.Home) {
		return nil, errors.New("migration plan target mismatch")
	}
	type fileCopy struct {
		relative    string
		destination string
		data        []byte
	}
	regular := make([]fileCopy, 0, len(plan.Files))
	historySources := make([]string, 0, 2)
	var legacyActive string
	for _, relative := range plan.Files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if filepath.IsAbs(relative) || strings.Contains(relative, "..") {
			return nil, fmt.Errorf("invalid migration path %q", relative)
		}
		source := filepath.Join(plan.Source, filepath.FromSlash(relative))
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(relative, "history/") {
			historySources = append(historySources, source)
			continue
		}
		if relative == "state.json" {
			var document struct {
				Active string `json:"active"`
			}
			if err := json.Unmarshal(data, &document); err != nil {
				return nil, fmt.Errorf("invalid legacy state: %w", err)
			}
			legacyActive = strings.TrimSpace(document.Active)
			continue
		}
		if strings.HasPrefix(relative, "accounts/") {
			if err := validateCredential(data); err != nil {
				return nil, fmt.Errorf("invalid legacy %s: %w", relative, err)
			}
		}
		regular = append(regular, fileCopy{relative: relative, destination: filepath.Join(target.Home, filepath.FromSlash(relative)), data: data})
	}

	return copied, storage.WithLock(ctx, target, func() error {
		before := make(map[string][]byte)
		created := make(map[string]bool)
		changed := make([]string, 0, len(regular)+2)
		rollback := func(cause error) error {
			for index := len(changed) - 1; index >= 0; index-- {
				path := changed[index]
				if created[path] {
					_ = os.Remove(path)
					continue
				}
				_ = writePrivateFile(path, before[path])
			}
			return cause
		}
		remember := func(path string) error {
			if _, seen := before[path]; seen || created[path] {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if errors.Is(readErr, os.ErrNotExist) {
				created[path] = true
				return nil
			}
			if readErr != nil {
				return readErr
			}
			before[path] = data
			return nil
		}
		identities, identityErr := existingAccountIdentities(target)
		if identityErr != nil {
			return identityErr
		}
		importedAccounts := make(map[string]bool)
		for _, copy := range regular {
			if _, statErr := os.Stat(copy.destination); statErr == nil {
				continue
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return rollback(statErr)
			}
			if strings.HasPrefix(copy.relative, "status/") {
				name := strings.TrimSuffix(filepath.Base(copy.relative), ".json")
				// Do not create status-only ghosts when a duplicate legacy
				// credential was intentionally folded into an existing account.
				if !importedAccounts[name] && !accountFileExists(target, name) {
					continue
				}
			}
			if strings.HasPrefix(copy.relative, "accounts/") {
				var credentials map[string]any
				if err := json.Unmarshal(copy.data, &credentials); err != nil {
					return rollback(err)
				}
				duplicate, duplicateErr := sameKnownIdentity(credentials, identities)
				if duplicateErr != nil {
					return rollback(duplicateErr)
				}
				if duplicate {
					// Same credential under a different legacy name is not another
					// account. Skipping it keeps live-auth synchronization unambiguous.
					continue
				}
				identities = append(identities, credentials)
				importedAccounts[strings.TrimSuffix(filepath.Base(copy.relative), ".json")] = true
			}
			if err := remember(copy.destination); err != nil {
				return rollback(err)
			}
			if err := writePrivateFile(copy.destination, copy.data); err != nil {
				return rollback(err)
			}
			changed = append(changed, copy.destination)
			copied = append(copied, copy.relative)
		}
		if len(historySources) > 0 {
			historyPath := target.HistoryFile()
			changedHistory, historyData, mergeErr := mergedLegacyHistorySources(historySources, historyPath)
			if mergeErr != nil {
				return rollback(mergeErr)
			}
			if changedHistory {
				if err := remember(historyPath); err != nil {
					return rollback(err)
				}
				if mergeErr = writePrivateFile(historyPath, historyData); mergeErr != nil {
					return rollback(mergeErr)
				}
				changed = append(changed, historyPath)
				copied = append(copied, "history/rate-limits.jsonl")
			}
		}
		if legacyActive != "" && (importedAccounts[legacyActive] || accountFileExists(target, legacyActive)) {
			statePath := target.StateFile()
			if err := remember(statePath); err != nil {
				return rollback(err)
			}
			if err := writePrivateFile(statePath, []byte("{\"active\":\""+legacyActive+"\"}\n")); err != nil {
				return rollback(err)
			}
			changed = append(changed, statePath)
			copied = append(copied, "state.json")
		}
		return nil
	})
}

func validateCredential(data []byte) error {
	var credentials map[string]any
	if err := json.Unmarshal(data, &credentials); err != nil {
		return err
	}
	tokens, _ := credentials["tokens"].(map[string]any)
	if strings.TrimSpace(stringValue(tokens["refresh_token"])) == "" {
		return errors.New("missing refresh token")
	}
	return nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func existingAccountIdentities(target storage.Paths) ([]map[string]any, error) {
	entries, err := os.ReadDir(target.AccountsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(target.AccountsDir(), entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var credentials map[string]any
		if json.Unmarshal(data, &credentials) == nil {
			result = append(result, credentials)
		}
	}
	return result, nil
}

func sameKnownIdentity(candidate map[string]any, identities []map[string]any) (bool, error) {
	for _, existing := range identities {
		same, _ := auth.SameIdentity(existing, candidate)
		if same {
			return true, nil
		}
		// Very old auth.json files occasionally lack id_token/email metadata.
		// An exact non-empty refresh-token match is still a safe proof that this
		// is the same credential, and avoids creating an ambiguous duplicate.
		if refreshToken(existing) != "" && refreshToken(existing) == refreshToken(candidate) {
			return true, nil
		}
	}
	return false, nil
}

func refreshToken(credentials map[string]any) string {
	tokens, _ := credentials["tokens"].(map[string]any)
	return strings.TrimSpace(stringValue(tokens["refresh_token"]))
}

func accountFileExists(target storage.Paths, name string) bool {
	path, err := target.Account(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".migration-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func mergedLegacyHistorySources(sources []string, destination string) (bool, []byte, error) {
	legacy := make([]history.Sample, 0)
	for _, source := range sources {
		rows, err := readAnyHistory(source)
		if err != nil {
			return false, nil, err
		}
		legacy = append(legacy, rows...)
	}
	existing, err := readCurrentHistory(destination)
	if err != nil {
		return false, nil, err
	}
	byKey := make(map[string]history.Sample, len(existing)+len(legacy))
	for _, sample := range append(existing, legacy...) {
		byKey[historyKey(sample)] = sample
	}
	merged := make([]history.Sample, 0, len(byKey))
	for _, sample := range byKey {
		merged = append(merged, sample)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].At.Equal(merged[j].At) {
			return merged[i].Account < merged[j].Account
		}
		return merged[i].At.Before(merged[j].At)
	})
	if sameHistory(existing, merged) {
		return false, nil, nil
	}
	var payload strings.Builder
	for _, sample := range merged {
		data, marshalErr := json.Marshal(sample)
		if marshalErr != nil {
			return false, nil, marshalErr
		}
		payload.Write(data)
		payload.WriteByte('\n')
	}
	return true, []byte(payload.String()), nil
}

func readAnyHistory(path string) ([]history.Sample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make([]history.Sample, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 2<<20)
	for scanner.Scan() {
		var sample history.Sample
		if json.Unmarshal(scanner.Bytes(), &sample) == nil && sample.Account != "" && !sample.At.IsZero() && len(sample.Windows) > 0 {
			result = append(result, sample)
			continue
		}
		legacy, err := parseLegacyHistorySample(scanner.Bytes())
		if err == nil && legacy.Account != "" {
			result = append(result, legacy)
		}
	}
	return result, scanner.Err()
}

func readCurrentHistory(path string) ([]history.Sample, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	samples := make([]history.Sample, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var sample history.Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			continue
		}
		if sample.Account != "" && !sample.At.IsZero() && len(sample.Windows) > 0 {
			samples = append(samples, sample)
		}
	}
	return samples, scanner.Err()
}

func parseLegacyHistorySample(data []byte) (history.Sample, error) {
	var row struct {
		Account   string   `json:"account"`
		Recorded  string   `json:"recorded_at"`
		Plan      string   `json:"plan_type"`
		Primary   *float64 `json:"primary_remaining_percent"`
		Secondary *float64 `json:"secondary_remaining_percent"`
	}
	if err := json.Unmarshal(data, &row); err != nil {
		return history.Sample{}, err
	}
	at, err := time.Parse(time.RFC3339, row.Recorded)
	if err != nil || strings.TrimSpace(row.Account) == "" {
		if err == nil {
			err = errors.New("legacy history account is missing")
		}
		return history.Sample{}, err
	}
	windows := map[string]float64{}
	if row.Primary != nil {
		windows["primary"] = *row.Primary
	}
	if row.Secondary != nil {
		windows["secondary"] = *row.Secondary
	}
	if len(windows) == 0 {
		return history.Sample{}, errors.New("legacy history has no quota windows")
	}
	return history.Sample{Account: strings.TrimSpace(row.Account), At: at.UTC(), Plan: row.Plan, Windows: windows}, nil
}

func historyKey(sample history.Sample) string {
	return sample.Account + "\x00" + sample.At.UTC().Format(time.RFC3339Nano)
}

func sameHistory(left, right []history.Sample) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if historyKey(left[index]) != historyKey(right[index]) || left[index].Plan != right[index].Plan || !windowsEqual(left[index].Windows, right[index].Windows) {
			return false
		}
	}
	return true
}

func windowsEqual(left, right map[string]float64) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		if other, ok := right[name]; !ok || other != value {
			return false
		}
	}
	return true
}
