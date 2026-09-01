package browser

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoverProfilesAndCopyCookieDBWithoutMutatingChrome(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Local State"), []byte(`{"profile":{"info_cache":{"Default":{"name":"Personal"}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	profileDir := filepath.Join(root, "Default", "Network")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(profileDir, "Cookies")
	if err := os.WriteFile(db, []byte("sqlite contents"), 0600); err != nil {
		t.Fatal(err)
	}
	profiles, err := DiscoverProfiles(root)
	if err != nil || len(profiles) != 1 || profiles[0].Name != "Personal" || profiles[0].Directory != "Default" {
		t.Fatalf("profiles=%#v err=%v", profiles, err)
	}
	copyPath, cleanup, err := CopyCookieDB(context.Background(), profiles[0])
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	copied, err := os.ReadFile(copyPath)
	if err != nil || string(copied) != "sqlite contents" {
		t.Fatalf("copy=%q err=%v", copied, err)
	}
	original, _ := os.ReadFile(db)
	if string(original) != "sqlite contents" {
		t.Fatal("original browser DB was modified")
	}
}

func TestValidateProfileRejectsOutsideCookieDB(t *testing.T) {
	profile := Profile{Path: t.TempDir(), CookieDB: filepath.Join(t.TempDir(), "Cookies")}
	if err := ValidateProfile(profile); err == nil {
		t.Fatal("expected unsafe path rejection")
	}
}

func TestLoadChatGPTCookiesFiltersExpiredAndKeepsValuesOutOfJSON(t *testing.T) {
	profilePath := t.TempDir()
	dbDir := filepath.Join(profilePath, "Network")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "Cookies")
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE cookies (host_key TEXT, name TEXT, value TEXT, encrypted_value BLOB, path TEXT, is_secure INTEGER, expires_utc INTEGER)`); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).UnixMicro() + 11_644_473_600_000_000
	past := time.Now().Add(-time.Hour).UnixMicro() + 11_644_473_600_000_000
	if _, err := database.Exec(`INSERT INTO cookies VALUES ('.chatgpt.com','session','secret',X'', '/', 1, ?), ('.chatgpt.com','old','expired',X'', '/', 1, ?)`, future, past); err != nil {
		t.Fatal(err)
	}
	database.Close()
	cookies, err := LoadChatGPTCookies(context.Background(), Profile{Path: profilePath, CookieDB: dbPath}, nil)
	if err != nil || len(cookies) != 1 || cookies[0].Name != "session" || cookies[0].Value != "secret" {
		t.Fatalf("cookies=%#v err=%v", cookies, err)
	}
	encoded, err := json.Marshal(cookies[0])
	if err != nil || strings.Contains(string(encoded), "secret") {
		t.Fatalf("encoded=%s err=%v", encoded, err)
	}
}
