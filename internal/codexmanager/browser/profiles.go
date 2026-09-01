package browser

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// DiscoverProfiles only reads profile metadata. The browser's profile and its
// SQLite databases are never opened writable by Abolqasem.
func DiscoverProfiles(roots ...string) ([]Profile, error) {
	if len(roots) == 0 {
		roots = DefaultRoots()
	}
	profiles := make([]Profile, 0)
	seen := make(map[string]struct{})
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		cache := profileInfoCache(filepath.Join(root, "Local State"))
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || (entry.Name() != "Default" && !strings.HasPrefix(entry.Name(), "Profile ")) {
				continue
			}
			directory := entry.Name()
			profilePath := filepath.Join(root, directory)
			cookieDB := findCookieDB(profilePath)
			if cookieDB == "" {
				continue
			}
			canonical, err := filepath.EvalSymlinks(cookieDB)
			if err != nil {
				canonical = cookieDB
			}
			if _, exists := seen[canonical]; exists {
				continue
			}
			seen[canonical] = struct{}{}
			name := strings.TrimSpace(cache[directory])
			if name == "" {
				name = profileName(filepath.Join(profilePath, "Preferences"), directory)
			}
			profiles = append(profiles, Profile{
				ID:          filepath.Base(root) + "/" + directory,
				Name:        name,
				Directory:   directory,
				Root:        root,
				Path:        profilePath,
				CookieDB:    cookieDB,
				LocalState:  filepath.Join(root, "Local State"),
				Preferences: filepath.Join(profilePath, "Preferences"),
			})
		}
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library/Application Support/Google/Chrome"), filepath.Join(home, "Library/Application Support/Chromium")}
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		return []string{filepath.Join(local, "Google/Chrome/User Data"), filepath.Join(local, "Chromium/User Data")}
	default:
		return []string{filepath.Join(home, ".config/google-chrome"), filepath.Join(home, ".config/google-chrome-beta"), filepath.Join(home, ".config/chromium")}
	}
}

func findCookieDB(profilePath string) string {
	for _, candidate := range []string{filepath.Join(profilePath, "Cookies"), filepath.Join(profilePath, "Network", "Cookies")} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func profileInfoCache(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var document struct {
		Profile struct {
			InfoCache map[string]struct {
				Name string `json:"name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if json.Unmarshal(data, &document) != nil {
		return nil
	}
	result := make(map[string]string, len(document.Profile.InfoCache))
	for directory, info := range document.Profile.InfoCache {
		result[directory] = strings.TrimSpace(info.Name)
	}
	return result
}

func profileName(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	var document struct {
		Profile struct {
			Name string `json:"name"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(data, &document); err != nil || strings.TrimSpace(document.Profile.Name) == "" {
		return fallback
	}
	return strings.TrimSpace(document.Profile.Name)
}

var ErrUnsafeCookiePath = errors.New("cookie database is outside the selected profile")

func ValidateProfile(profile Profile) error {
	if profile.Path == "" || profile.CookieDB == "" {
		return ErrUnsafeCookiePath
	}
	relative, err := filepath.Rel(profile.Path, profile.CookieDB)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return ErrUnsafeCookiePath
	}
	return nil
}
