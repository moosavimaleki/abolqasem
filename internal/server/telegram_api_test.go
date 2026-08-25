package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"abolqasem/internal/workspace/readmodels"
)

func TestTelegramConfigAPIStoresPrivateAllowlistedConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	mux := http.NewServeMux()
	setupRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/telegram/configure", bytes.NewBufferString(`{"botToken":" 123:token ","proxyUrl":"socks5://127.0.0.1:10810","allowedUserIds":[" tg:42 ","*","42"]}`))
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
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"configured":true`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"proxyConfigured":true`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"allowAllUsers":true`)) {
		t.Fatalf("unexpected Telegram status: %d %s", status.Code, status.Body.String())
	}
}

func TestTelegramConfigAcceptsLegacyScalarNumericIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(filepath.Dir(telegramBridgeConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"botToken":"123:token","allowedUserIds":42,"chatIds":-100123,"mappings":{}}`)
	if err := os.WriteFile(telegramBridgeConfigPath(), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := loadTelegramBridgeConfig()
	if err != nil {
		t.Fatalf("load legacy Telegram config: %v", err)
	}
	if !reflect.DeepEqual(config.AllowedUserIDs, []string{"42"}) || !reflect.DeepEqual(config.ChatIDs, []string{"-100123"}) {
		t.Fatalf("legacy IDs were not normalized: %#v", config)
	}
}

func TestTelegramBridgeSendsRichMarkdownWithRTLMetadata(t *testing.T) {
	var path string
	var payload struct {
		ChatID      string `json:"chat_id"`
		RichMessage struct {
			Markdown string `json:"markdown"`
			IsRTL    bool   `json:"is_rtl"`
		} `json:"rich_message"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()

	bridge := &telegramBridge{client: server.Client(), apiBaseURL: server.URL}
	markdown := "# گزارش بازار\n\n| Coin | Change |\n|---|---|\n| BTC | +5% |\n\n<details>\n<summary>جزئیات</summary>\n\nمتن بیشتر\n\n</details>"
	bridge.sendText(context.Background(), "123:test", "42", markdown)

	if path != "/bot123:test/sendRichMessage" {
		t.Fatalf("path = %q", path)
	}
	if payload.ChatID != "42" || payload.RichMessage.Markdown != markdown || !payload.RichMessage.IsRTL {
		t.Fatalf("unexpected rich message payload: %#v", payload)
	}
	if telegramMarkdownIsRTL("English-only response") {
		t.Fatal("expected English-only Markdown to remain LTR")
	}
}

func TestTelegramTestEndpointUsesKnownChatAndRichRTLMessage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(filepath.Dir(telegramBridgeConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{"botToken":"123:test","allowedUserIds":["42"],"chatIds":["99"],"mappings":{}}`)
	if err := os.WriteFile(telegramBridgeConfigPath(), config, 0o600); err != nil {
		t.Fatal(err)
	}

	var path string
	var payload struct {
		ChatID      string `json:"chat_id"`
		RichMessage struct {
			Markdown string `json:"markdown"`
			IsRTL    bool   `json:"is_rtl"`
		} `json:"rich_message"`
	}
	telegramServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode test payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer telegramServer.Close()

	originalBridge := workspaceTelegramBridge
	workspaceTelegramBridge = &telegramBridge{client: telegramServer.Client(), apiBaseURL: telegramServer.URL, lastForwardedByChat: map[string]string{}}
	t.Cleanup(func() { workspaceTelegramBridge = originalBridge })
	mux := http.NewServeMux()
	setupRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/telegram/test", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("test status=%d body=%s", response.Code, response.Body.String())
	}
	if path != "/bot123:test/sendRichMessage" || payload.ChatID != "99" || !payload.RichMessage.IsRTL || !strings.Contains(payload.RichMessage.Markdown, "پیام آزمایشی") {
		t.Fatalf("unexpected Telegram test request: path=%q payload=%#v", path, payload)
	}
}

func TestTelegramBridgeRequiresAndSupportsExplicitProxy(t *testing.T) {
	for _, name := range []string{"ABOLQASEM_TELEGRAM_PROXY", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(name, "")
	}
	if _, err := telegramHTTPClient(""); err == nil {
		t.Fatal("expected Telegram bridge to require a proxy")
	}
	client, err := telegramHTTPClient("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("telegramHTTPClient returned error: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatal("expected explicit HTTP proxy transport")
	}
	request := httptest.NewRequest(http.MethodGet, "https://api.telegram.org", nil)
	proxyURL, err := transport.Proxy(request)
	if err != nil || proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxy URL = %#v, err=%v", proxyURL, err)
	}
	if _, err := telegramHTTPClient("socks5h://127.0.0.1:10808"); err != nil {
		t.Fatalf("expected SOCKS5H proxy support: %v", err)
	}
}

func TestTelegramGetUpdatesRequestsAndParsesCallbackQueries(t *testing.T) {
	var method string
	var payload struct {
		Offset         int64    `json:"offset"`
		AllowedUpdates []string `json:"allowed_updates"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode getUpdates payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":8,"callback_query":{"id":"cb-1","data":"chat:chat-1","from":{"id":42},"message":{"chat":{"id":99}}}}]}`))
	}))
	defer server.Close()
	bridge := &telegramBridge{client: server.Client(), apiBaseURL: server.URL}
	updates, err := bridge.getUpdates(context.Background(), "123:test", 7)
	if err != nil {
		t.Fatalf("getUpdates: %v", err)
	}
	if method != http.MethodPost || payload.Offset != 7 || !reflect.DeepEqual(payload.AllowedUpdates, []string{"message", "callback_query"}) {
		t.Fatalf("unexpected getUpdates request: %s %#v", method, payload)
	}
	if len(updates) != 1 || updates[0].CallbackQuery == nil || updates[0].CallbackQuery.Data != "chat:chat-1" {
		t.Fatalf("callback query was not parsed: %#v", updates)
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

func TestTelegramTakeOverLockCallbackIsCompactAndValidated(t *testing.T) {
	data := telegramTakeOverLockCallbackPrefix + "chat-123"
	if chatID, ok := telegramTakeOverLockChatID(data); !ok || chatID != "chat-123" {
		t.Fatalf("takeover callback = %q %v", chatID, ok)
	}
	if _, ok := telegramTakeOverLockChatID("chat:chat-123"); ok {
		t.Fatal("ordinary chat selection must not be treated as a takeover")
	}
	if _, ok := telegramTakeOverLockChatID(telegramTakeOverLockCallbackPrefix + strings.Repeat("x", 65)); ok {
		t.Fatal("oversized callback data must be rejected")
	}
	markup := telegramTakeOverLockMarkupForStatus("chat-123", readmodels.CodexLockStatus{State: codexLockOwnedElsewhere, CanTakeOver: true})
	encoded, err := json.Marshal(markup)
	if err != nil || !strings.Contains(string(encoded), "گرفتن نشست") || !strings.Contains(string(encoded), data) {
		t.Fatalf("takeover markup = %s, err=%v", encoded, err)
	}
	if markup := telegramTakeOverLockMarkupForStatus("chat-123", readmodels.CodexLockStatus{State: codexLockOwnedByUs, CanTakeOver: true}); markup != nil {
		t.Fatalf("unexpected takeover markup for owned session: %#v", markup)
	}
}

func TestTelegramChatChoicesAreNewestFirstAndRenderable(t *testing.T) {
	sidebar := readmodels.SidebarData{ProjectGroups: []readmodels.SidebarProjectGroup{
		{Title: "پروژه *یک*", Chats: []readmodels.SidebarChatRow{{ChatID: "chat-old", Title: "قدیمی", CreationTime: 10}}},
		{Title: "پروژه دو", Chats: []readmodels.SidebarChatRow{{ChatID: "chat-new", Title: "جدید", CreationTime: 20}}},
	}}
	choices := telegramChatChoicesFromSidebar(sidebar, 0)
	if len(choices) != 2 || choices[0].ChatID != "chat-new" || choices[1].ChatID != "chat-old" {
		t.Fatalf("unexpected Telegram choices: %#v", choices)
	}
	if got := telegramMarkdownInline("پروژه *یک*"); got != `پروژه \*یک\*` {
		t.Fatalf("escaped title = %q", got)
	}
}

func TestTelegramFinalMessageDoesNotRepeatAnOlderTurn(t *testing.T) {
	entries := []readmodels.TranscriptEntry{
		{"_id": "old-user", "kind": "user_prompt", "content": "قدیمی"},
		{"_id": "old-answer", "kind": "assistant_text", "text": "پاسخ قدیمی"},
		{"_id": "new-user", "kind": "user_prompt", "content": "جدید"},
		{"_id": "new-answer", "kind": "assistant_text", "text": "پاسخ جدید"},
	}
	fingerprint, text := telegramFinalMessage(entries)
	if fingerprint != "new-answer" || text != "پاسخ جدید" {
		t.Fatalf("final Telegram message = %q %q", fingerprint, text)
	}
}

func TestTelegramFinalMessageUsesCleanProposedPlanMarkdown(t *testing.T) {
	entries := []readmodels.TranscriptEntry{
		{"_id": "user", "kind": "user_prompt", "content": "پلن کن"},
		{"_id": "plan", "kind": "proposed_plan", "turnId": "turn-1", "plan": "# پلن\n\n- تست"},
	}
	fingerprint, text := telegramFinalMessage(entries)
	if fingerprint != "plan" || text != "# پلن\n\n- تست" || strings.Contains(text, "proposed_plan") {
		t.Fatalf("final Telegram plan = %q %q", fingerprint, text)
	}
}

func TestNormalizeTelegramMappingsRejectsInvalidIDs(t *testing.T) {
	got := normalizeTelegramMappings(map[string]string{"20": "chat-a", "bad": "chat-b", "30": ""})
	want := map[string]string{"20": "chat-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mappings = %#v, want %#v", got, want)
	}
}
