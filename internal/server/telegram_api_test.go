package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTelegramConfigAPIStoresPrivateAllowlistedConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	mux := http.NewServeMux()
	setupRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/telegram/configure", bytes.NewBufferString(`{"botToken":" 123:token ","allowedUserIds":[" tg:42 ","*","42"]}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("configure status=%d body=%s", response.Code, response.Body.String())
	}
	info, err := os.Stat(telegramBridgeConfigPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected private Telegram config, info=%#v err=%v", info, err)
	}

	status := httptest.NewRecorder()
	mux.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/telegram/status", nil))
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"configured":true`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"allowAllUsers":true`)) {
		t.Fatalf("unexpected Telegram status: %d %s", status.Code, status.Body.String())
	}
}

func TestTelegramBridgeCommandAndTextHelpers(t *testing.T) {
	command, argument := telegramCommand("/chat@my_bot chat-123")
	if command != "chat" || argument != "chat-123" {
		t.Fatalf("telegram command = %q %q", command, argument)
	}
	config := telegramBridgeConfig{AllowedUserIDs: []string{"42", "*"}, Mappings: map[string]string{"20": "chat-b", "10": "chat-a"}}
	if !telegramUserAllowed(config, "999") || !telegramUserAllowed(config, "42") {
		t.Fatal("allowlist did not accept configured users")
	}
	if got := sortedTelegramMappingIDs(config.Mappings); !reflect.DeepEqual(got, []string{"10", "20"}) {
		t.Fatalf("sorted mappings = %#v", got)
	}
	long := string(bytes.Repeat([]byte("a"), telegramMessageLimit+20))
	chunks := splitTelegramText(long)
	if len(chunks) != 2 || len([]rune(chunks[0])) > telegramMessageLimit || chunks[0]+chunks[1] != long {
		t.Fatalf("unexpected Telegram chunks: %#v", chunks)
	}
}

func TestNormalizeTelegramMappingsRejectsInvalidIDs(t *testing.T) {
	got := normalizeTelegramMappings(map[string]string{"20": "chat-a", "bad": "chat-b", "30": ""})
	want := map[string]string{"20": "chat-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mappings = %#v, want %#v", got, want)
	}
}
