package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var accountSwitchKey = []byte("oai/apps/accountSwitchSessions")

// DiscoverSwitchAccounts reads LevelDB files as immutable byte streams. It
// extracts only email addresses from the account-switcher value and retains no
// bearer token or session material.
func DiscoverSwitchAccounts(profile Profile) ([]string, error) {
	if profile.Path == "" {
		return nil, ErrUnsafeCookiePath
	}
	locations := []string{
		filepath.Join(profile.Path, "Local Storage", "leveldb"),
		filepath.Join(profile.Path, "IndexedDB", "https_chatgpt.com_0.indexeddb.leveldb"),
	}
	var best accountCandidate
	for _, location := range locations {
		entries, err := os.ReadDir(location)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(location, entry.Name()))
			if err != nil {
				continue
			}
			for offset := 0; ; {
				index := indexOf(data, accountSwitchKey, offset)
				if index < 0 {
					break
				}
				offset = index + len(accountSwitchKey)
				candidate := parseSwitchAccounts(data[offset:])
				if candidate.latest > best.latest || (candidate.latest == best.latest && len(candidate.emails) > len(best.emails)) {
					best = candidate
				}
			}
		}
	}
	return best.emails, nil
}

type accountCandidate struct {
	latest int64
	emails []string
}

func parseSwitchAccounts(data []byte) accountCandidate {
	start := indexOf(data, []byte("["), 0)
	if start < 0 {
		return accountCandidate{}
	}
	decoder := json.NewDecoder(strings.NewReader(string(data[start:])))
	var values []struct {
		Email        string `json:"email"`
		LastLoggedIn int64  `json:"lastLoggedInAt"`
	}
	if decoder.Decode(&values) != nil {
		return accountCandidate{}
	}
	seen := make(map[string]struct{})
	result := accountCandidate{}
	for _, value := range values {
		email := strings.ToLower(strings.TrimSpace(value.Email))
		if email == "" {
			continue
		}
		seen[email] = struct{}{}
		if value.LastLoggedIn > result.latest {
			result.latest = value.LastLoggedIn
		}
	}
	for email := range seen {
		result.emails = append(result.emails, email)
	}
	sort.Strings(result.emails)
	return result
}

func AssociateManagedAccounts(emails []string, managed map[string]string) map[string]string {
	result := make(map[string]string)
	byEmail := make(map[string]string, len(managed))
	for accountName, email := range managed {
		if normalized := strings.ToLower(strings.TrimSpace(email)); normalized != "" {
			byEmail[normalized] = accountName
		}
	}
	for _, email := range emails {
		if accountName, ok := byEmail[strings.ToLower(strings.TrimSpace(email))]; ok {
			result[email] = accountName
		}
	}
	return result
}

func indexOf(data, needle []byte, start int) int {
	for index := start; index+len(needle) <= len(data); index++ {
		matched := true
		for offset := range needle {
			if data[index+offset] != needle[offset] {
				matched = false
				break
			}
		}
		if matched {
			return index
		}
	}
	return -1
}
