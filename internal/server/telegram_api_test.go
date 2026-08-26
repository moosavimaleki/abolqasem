package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"abolqasem/internal/workspace/readmodels"
)

func TestTelegramConfigAPIStoresPrivateAllowlistedConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	mux := http.NewServeMux()
	setupRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/telegram/configure", bytes.NewBufferString(`{"botToken":" 123:token ","proxyUrl":"socks5://127.0.0.1:10810","allowedUserIds":[" tg:42 ","*","42"],"customCommands":[{"name":" Git_Status ","description":" Repository status ","command":" git status --short ","workingDirectory":" /tmp ","timeoutSeconds":999}]}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("configure status=%d body=%s", response.Code, response.Body.String())
	}
	info, err := os.Stat(telegramBridgeConfigPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("expected private Telegram config, info=%#v err=%v", info, err)
	}
	config, err := loadTelegramBridgeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.CustomCommands, []telegramCustomCommand{{Name: "git_status", Description: "Repository status", Command: "git status --short", WorkingDirectory: "/tmp", TimeoutSeconds: 120}}) {
		t.Fatalf("unexpected custom commands: %#v", config.CustomCommands)
	}

	status := httptest.NewRecorder()
	mux.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/telegram/status", nil))
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"configured":true`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"proxyConfigured":true`)) || !bytes.Contains(status.Body.Bytes(), []byte(`"allowAllUsers":true`)) {
		t.Fatalf("unexpected Telegram status: %d %s", status.Code, status.Body.String())
	}
}

func TestTelegramRuntimeStateSavePreservesNewerCustomCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	command := telegramCustomCommand{Name: "status", Command: "git status --short", TimeoutSeconds: 30}
	latest := telegramBridgeConfig{
		BotToken:       "123:token",
		ProxyURL:       "socks5://127.0.0.1:10810",
		AllowedUserIDs: []string{"42"},
		CustomCommands: []telegramCustomCommand{command},
	}
	if err := saveTelegramBridgeConfig(latest); err != nil {
		t.Fatal(err)
	}

	staleWorkerConfig := telegramBridgeConfig{
		BotToken:       latest.BotToken,
		ProxyURL:       latest.ProxyURL,
		AllowedUserIDs: latest.AllowedUserIDs,
		ChatIDs:        []string{"99"},
		Mappings:       map[string]string{"99": "chat-1"},
	}
	if err := saveTelegramBridgeRuntimeState(staleWorkerConfig); err != nil {
		t.Fatal(err)
	}

	stored, err := loadTelegramBridgeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.CustomCommands, []telegramCustomCommand{command}) {
		t.Fatalf("runtime state save removed custom commands: %#v", stored.CustomCommands)
	}
	if !reflect.DeepEqual(stored.ChatIDs, []string{"99"}) || stored.Mappings["99"] != "chat-1" {
		t.Fatalf("runtime state was not saved: %#v", stored)
	}
}

func TestTelegramBridgeFingerprintIncludesCustomCommands(t *testing.T) {
	base := telegramBridgeConfig{BotToken: "123:token", ProxyURL: "socks5://127.0.0.1:10810", AllowedUserIDs: []string{"42"}}
	withCommand := base
	withCommand.CustomCommands = []telegramCustomCommand{{Name: "status", Command: "git status --short"}}
	if telegramBridgeConfigFingerprint(base) == telegramBridgeConfigFingerprint(withCommand) {
		t.Fatal("custom command changes must restart the Telegram worker")
	}
}

func TestTelegramCustomCommandRunsOnlyConfiguredCommand(t *testing.T) {
	output, err := runTelegramCustomCommand(context.Background(), telegramCustomCommand{
		Name:           "report",
		Command:        "printf 'ready'",
		TimeoutSeconds: 1,
	})
	if err != nil || output != "ready" {
		t.Fatalf("run command = %q, %v", output, err)
	}
	if _, ok := telegramCustomCommandByName([]telegramCustomCommand{{Name: "report"}}, "missing"); ok {
		t.Fatal("unknown command must not resolve")
	}
	if !strings.Contains(telegramCustomCommandHelpMarkdown([]telegramCustomCommand{{Name: "report", Description: "show report"}}), "/run report") {
		t.Fatal("custom command help should advertise the safe invocation")
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

func TestTelegramConfigAPIDoesNotSilentlyDropInvalidCustomCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := saveTelegramBridgeConfig(telegramBridgeConfig{
		BotToken: "123:token", ProxyURL: "socks5://127.0.0.1:10810", AllowedUserIDs: []string{"42"},
		CustomCommands: []telegramCustomCommand{{Name: "status", Command: "git status --short"}},
	}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	setupRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/telegram/configure", bytes.NewBufferString(`{"botToken":"123:token","proxyUrl":"socks5://127.0.0.1:10810","allowedUserIds":["42"],"customCommands":[{"name":"bad/name","command":"echo no"}]}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "name must use") {
		t.Fatalf("expected actionable custom command validation error, got %d %s", response.Code, response.Body.String())
	}
	stored, err := loadTelegramBridgeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.CustomCommands, []telegramCustomCommand{{Name: "status", Command: "git status --short", TimeoutSeconds: 30}}) {
		t.Fatalf("invalid save must preserve existing commands: %#v", stored.CustomCommands)
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

func TestTelegramMarkdownSplitterClosesFencesAndRepeatsTableHeaders(t *testing.T) {
	code := "```go\n" + strings.Repeat("fmt.Println(\"safe boundary\")\n", 180) + "```"
	chunks := splitTelegramText(code)
	if len(chunks) < 2 {
		t.Fatalf("expected code to split, got %#v", chunks)
	}
	for index, chunk := range chunks {
		if len([]rune(chunk)) > telegramMessageLimit || strings.Count(chunk, "```")%2 != 0 {
			t.Fatalf("invalid code chunk %d: %q", index, chunk)
		}
	}
	joined := strings.Join(chunks, "\n")
	if strings.Count(joined, "fmt.Println") != 180 {
		t.Fatalf("code lines were lost across chunks: %d", strings.Count(joined, "fmt.Println"))
	}

	table := "| file | change |\n|---|---|\n" + strings.Repeat("| internal/server/very-long-name.go | edited |\n", 120)
	chunks = splitTelegramText(table)
	if len(chunks) < 2 {
		t.Fatalf("expected table to split, got %#v", chunks)
	}
	for index, chunk := range chunks[1:] {
		if !strings.HasPrefix(chunk, "| file | change |\n|---|---|") {
			t.Fatalf("table header was not repeated for chunk %d: %q", index+1, chunk[:min(80, len(chunk))])
		}
	}
}

func TestTelegramTranscriptPreviewsUseOpaqueCallbacksAndVectorDocuments(t *testing.T) {
	bridge := &telegramBridge{previewByToken: map[string]telegramPreviewItem{}}
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main() {\n\tprintln(\"ready\")\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := telegramPreviewItem{Kind: telegramPreviewFile, TelegramChatID: "99", WorkspaceChatID: "chat-1", FilePath: file, Line: 3, CreatedAt: time.Now()}
	token := bridge.rememberTelegramPreview(item)
	if token == "" || len(token) > 64 {
		t.Fatalf("invalid preview token: %q", token)
	}
	if _, ok := bridge.telegramPreviewForCallback(telegramPreviewCallbackPrefix+token, "99", "wrong-chat"); ok {
		t.Fatal("preview token must be bound to its workspace chat")
	}
	preview, ok := bridge.telegramPreviewForCallback(telegramPreviewCallbackPrefix+token, "99", "chat-1")
	if !ok || preview.FilePath != file {
		t.Fatalf("preview callback = %#v %v", preview, ok)
	}
	response, err := buildFilePreview([]string{filepath.Dir(file)}, file, 3, filePreviewOptions{Full: true})
	if err != nil {
		t.Fatalf("full code preview err=%v", err)
	}
	if !response.Full || len(response.Lines) != 5 {
		t.Fatalf("Telegram code preview must include the full file: full=%v lines=%d", response.Full, len(response.Lines))
	}
	codeSVG := telegramCodePreviewSVG(response)
	if !strings.Contains(codeSVG, "<svg") || !strings.Contains(codeSVG, "main.go") || !strings.Contains(codeSVG, `width="720" height="`) {
		t.Fatalf("code preview svg err=%v", err)
	}
	if strings.Contains(codeSVG, `width="100%"`) {
		t.Fatal("code preview must use explicit background dimensions for Telegram SVG viewer")
	}
	chartSVG := telegramMermaidSVG("flowchart TD\nA --> B")
	if !strings.Contains(chartSVG, `<rect x="0" y="0" width="780"`) {
		t.Fatal("Mermaid preview must include an opaque explicit-size background")
	}
	chart := telegramMermaidSVG("flowchart TD\nA --> B\nB --> C")
	if !strings.Contains(chart, "<svg") || !strings.Contains(chart, "marker-end") {
		t.Fatalf("Mermaid SVG was not rendered: %s", chart)
	}
	markdown, buttons := bridge.telegramTranscriptPreviews("99", "chat-1", "```mermaid\nflowchart TD\nA --> B\n```")
	if !strings.Contains(markdown, "برای دیدن") || len(buttons) != 1 || !strings.Contains(buttons[0].Label, "Mermaid") {
		t.Fatalf("Mermaid transcript preview = %q %#v", markdown, buttons)
	}
}

func TestTelegramCodePreviewCapsHugeFilesWithDownloadHint(t *testing.T) {
	lines := make([]filePreviewLine, telegramPreviewMaxCodeLines+25)
	for index := range lines {
		lines[index] = filePreviewLine{Number: index + 1, Text: fmt.Sprintf("line %d", index+1)}
	}

	svg := telegramCodePreviewSVG(filePreviewResponse{Path: "/tmp/large.py", Language: "python", Lines: lines})
	if !strings.Contains(svg, "line 2400") {
		t.Fatal("expected the capped preview to retain the last included source line")
	}
	if !strings.Contains(svg, "remaining 25 lines") {
		t.Fatal("expected a download hint for omitted lines")
	}
	if strings.Contains(svg, "line 2425") {
		t.Fatal("capped preview must not render lines beyond the safety limit")
	}
}

func TestTelegramSendDocumentUploadsVectorPreview(t *testing.T) {
	var filename, chatID, caption string
	var payload []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/bot123:test/sendDocument" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if err := request.ParseMultipartForm(1024 * 1024); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		chatID, caption = request.FormValue("chat_id"), request.FormValue("caption")
		file, header, err := request.FormFile("document")
		if err != nil {
			t.Errorf("FormFile: %v", err)
		} else {
			defer file.Close()
			filename = header.Filename
			payload, _ = io.ReadAll(file)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()
	bridge := &telegramBridge{client: server.Client(), apiBaseURL: server.URL}
	if err := bridge.sendDocument(context.Background(), "123:test", "99", "preview.svg", []byte("<svg/>"), "preview"); err != nil {
		t.Fatal(err)
	}
	if filename != "preview.svg" || chatID != "99" || caption != "preview" || string(payload) != "<svg/>" {
		t.Fatalf("unexpected document request: %q %q %q %q", filename, chatID, caption, payload)
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

func TestTelegramTakeOverAcknowledgesBeforeClaimCompletes(t *testing.T) {
	paths := make(chan string, 2)
	telegramServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer telegramServer.Close()

	claimStarted := make(chan struct{})
	releaseClaim := make(chan struct{})
	claimFinished := make(chan struct{})
	originalTakeOver := workspaceTakeOverCodexSessionForTelegram
	workspaceTakeOverCodexSessionForTelegram = func(chatID string, confirmed bool, executionMode string) (readmodels.CodexLockStatus, error) {
		close(claimStarted)
		<-releaseClaim
		close(claimFinished)
		return readmodels.CodexLockStatus{State: codexLockOwnedByUs}, nil
	}
	t.Cleanup(func() {
		select {
		case <-claimFinished:
		default:
			close(releaseClaim)
			<-claimFinished
		}
		workspaceTakeOverCodexSessionForTelegram = originalTakeOver
	})

	bridge := &telegramBridge{client: telegramServer.Client(), apiBaseURL: telegramServer.URL}
	bridge.startCodexTakeOverFromTelegram(context.Background(), telegramBridgeConfig{BotToken: "123:test"}, "callback-1", "99", "chat-1")

	select {
	case path := <-paths:
		if path != "/bot123:test/answerCallbackQuery" {
			t.Fatalf("first Telegram request = %q", path)
		}
	case <-time.After(time.Second):
		t.Fatal("callback query was not acknowledged promptly")
	}
	select {
	case <-claimStarted:
	case <-time.After(time.Second):
		t.Fatal("takeover worker did not start")
	}
	select {
	case path := <-paths:
		t.Fatalf("sent %q before takeover completed", path)
	default:
	}

	close(releaseClaim)
	select {
	case path := <-paths:
		if path != "/bot123:test/sendRichMessage" {
			t.Fatalf("completion Telegram request = %q", path)
		}
	case <-time.After(time.Second):
		t.Fatal("takeover completion was not reported")
	}
}

func TestTelegramChatChoicesAreNewestFirstAndRenderable(t *testing.T) {
	sidebar := readmodels.SidebarData{ProjectGroups: []readmodels.SidebarProjectGroup{
		{GroupKey: "project-one", Title: "پروژه *یک*", Chats: []readmodels.SidebarChatRow{{ChatID: "chat-old", Title: "قدیمی", CreationTime: 10}}},
		{GroupKey: "project-two", Title: "پروژه دو", Chats: []readmodels.SidebarChatRow{{ChatID: "chat-new", Title: "جدید", CreationTime: 20}}},
	}}
	choices := telegramChatChoicesFromSidebar(sidebar, 0)
	if len(choices) != 2 || choices[0].ChatID != "chat-new" || choices[1].ChatID != "chat-old" {
		t.Fatalf("unexpected Telegram choices: %#v", choices)
	}
	if choices[0].ProjectID != "project-two" {
		t.Fatalf("chat project id = %q", choices[0].ProjectID)
	}
	if got := telegramMarkdownInline("پروژه *یک*"); got != `پروژه \*یک\*` {
		t.Fatalf("escaped title = %q", got)
	}
}

func TestTelegramProjectPickerSeparatesChatsByProject(t *testing.T) {
	sidebar := readmodels.SidebarData{ProjectGroups: []readmodels.SidebarProjectGroup{
		{GroupKey: "project-old", Title: "قدیمی", Chats: []readmodels.SidebarChatRow{{ChatID: "chat-old", Title: "کهنه", CreationTime: 10}}},
		{GroupKey: "project-active", Title: "فعال", Chats: []readmodels.SidebarChatRow{
			{ChatID: "chat-new", Title: "جدید", CreationTime: 30},
			{ChatID: "chat-middle", Title: "میانی", CreationTime: 20},
		}},
	}}
	projects := telegramProjectChoicesFromSidebar(sidebar, 0)
	if len(projects) != 2 || projects[0].ProjectID != "project-active" || projects[0].ChatCount != 2 {
		t.Fatalf("unexpected project choices: %#v", projects)
	}
	allChats := telegramChatChoicesFromSidebar(sidebar, 0)
	projectChats := telegramChatChoicesForProjectFromChoices("project-active", allChats)
	if len(projectChats) != 2 || projectChats[0].ChatID != "chat-new" || projectChats[1].ChatID != "chat-middle" {
		t.Fatalf("project chats = %#v", projectChats)
	}
	if markdown := telegramProjectChatListMarkdown("chat-middle", projectChats, 1); !strings.Contains(markdown, "دکمه") || !strings.Contains(markdown, "نشست فعلی") || strings.Contains(markdown, "کهنه") || strings.Contains(markdown, "فعال") {
		t.Fatalf("project chat markdown = %q", markdown)
	}
	markup, err := json.Marshal(telegramProjectChatPickerMarkup("chat-middle", projectChats, 1))
	if err != nil || !strings.Contains(string(markup), "chat:chat-new") || !strings.Contains(string(markup), "chat:chat-middle") || strings.Contains(string(markup), "chat:chat-old") {
		t.Fatalf("project chat markup = %s err=%v", markup, err)
	}
	if projectID, ok := telegramProjectCallbackProjectID("project:project-active"); !ok || projectID != "project-active" {
		t.Fatalf("project callback = %q %v", projectID, ok)
	}
	if _, ok := telegramProjectCallbackProjectID("project:" + strings.Repeat("x", 65)); ok {
		t.Fatal("oversized project callback must be rejected")
	}
}

func TestTelegramProjectAndChatPickersPaginateFiveAtATime(t *testing.T) {
	projects := make([]telegramProjectChoice, 0, 7)
	chats := make([]telegramChatChoice, 0, 7)
	for index := 1; index <= 7; index++ {
		projects = append(projects, telegramProjectChoice{ProjectID: fmt.Sprintf("project-%d", index), Title: fmt.Sprintf("پروژه %d", index), ChatCount: index})
		chats = append(chats, telegramChatChoice{ProjectID: "project-1", ProjectTitle: "پروژه ۱", ChatID: fmt.Sprintf("chat-%d", index), ChatTitle: fmt.Sprintf("چت %d", index)})
	}

	projectMarkup, err := json.Marshal(telegramProjectPickerMarkupForChoices(projects, 1))
	if err != nil || strings.Count(string(projectMarkup), `"callback_data":"project:`) != 5 || !strings.Contains(string(projectMarkup), `"callback_data":"ps:2"`) || strings.Contains(string(projectMarkup), `"callback_data":"ps:0"`) {
		t.Fatalf("first project page markup = %s err=%v", projectMarkup, err)
	}
	secondProjectMarkup, _ := json.Marshal(telegramProjectPickerMarkupForChoices(projects, 2))
	if strings.Count(string(secondProjectMarkup), `"callback_data":"project:`) != 2 || !strings.Contains(string(secondProjectMarkup), `"callback_data":"ps:1"`) || strings.Contains(string(secondProjectMarkup), `"callback_data":"ps:3"`) {
		t.Fatalf("second project page markup = %s", secondProjectMarkup)
	}

	chatMarkup, err := json.Marshal(telegramProjectChatPickerMarkup("", chats, 1))
	if err != nil || strings.Count(string(chatMarkup), `"callback_data":"chat:`) != 5 || !strings.Contains(string(chatMarkup), `"callback_data":"pc:2:project-1"`) {
		t.Fatalf("first chat page markup = %s err=%v", chatMarkup, err)
	}
	secondChatMarkup, _ := json.Marshal(telegramProjectChatPickerMarkup("", chats, 2))
	if strings.Count(string(secondChatMarkup), `"callback_data":"chat:`) != 2 || !strings.Contains(string(secondChatMarkup), `"callback_data":"pc:1:project-1"`) {
		t.Fatalf("second chat page markup = %s", secondChatMarkup)
	}
	if projectID, page, ok := telegramProjectChatsPage("pc:2:project-1"); !ok || projectID != "project-1" || page != 2 {
		t.Fatalf("chat page callback = %q %d %v", projectID, page, ok)
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

func TestTelegramFinalMessageStripsInternalMemoryCitation(t *testing.T) {
	entries := []readmodels.TranscriptEntry{
		{"_id": "user", "kind": "user_prompt", "content": "گزارش بده"},
		{"_id": "answer", "kind": "assistant_text", "text": "گزارش آماده شد.\n\n<oai-mem-citation>\n<citation_entries>MEMORY.md:1-2</citation_entries>\n<rollout_ids>abc</rollout_ids>\n</oai-mem-citation>"},
	}
	fingerprint, text := telegramFinalMessage(entries)
	if fingerprint != "answer" || text != "گزارش آماده شد." {
		t.Fatalf("sanitized Telegram message = %q %q", fingerprint, text)
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
