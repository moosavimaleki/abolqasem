package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"abolqasem/internal/providers/catalog"
	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"

	"golang.org/x/net/proxy"
)

const telegramCustomCommandOutputLimit = 24 * 1024

const telegramMessageLimit = 3500

const telegramTakeOverLockCallbackPrefix = "lock:takeover:"
const telegramProjectCallbackPrefix = "project:"
const telegramProjectsPageCallbackPrefix = "ps:"
const telegramProjectChatsPageCallbackPrefix = "pc:"
const telegramPickerPageSize = 5
const telegramTelegramPickerCallback = "picker"
const telegramModelCallbackPrefix = "tm:"
const telegramEffortCallbackPrefix = "te:"

type telegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *telegramMessage       `json:"message"`
	CallbackQuery *telegramCallbackQuery `json:"callback_query"`
}

type telegramMessage struct {
	Text string `json:"text"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

type telegramCallbackQuery struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *telegramMessage `json:"message"`
}

type telegramAPIResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"description"`
}

type telegramBridge struct {
	mu                  sync.Mutex
	cancel              context.CancelFunc
	active              bool
	lastError           string
	fingerprint         string
	client              *http.Client
	apiBaseURL          string
	lastForwardedByChat map[string]string
	previewByToken      map[string]telegramPreviewItem
}

const telegramAPIBaseURL = "https://api.telegram.org"

var workspaceTelegramBridge = &telegramBridge{
	client:              &http.Client{Timeout: 40 * time.Second},
	apiBaseURL:          telegramAPIBaseURL,
	lastForwardedByChat: map[string]string{},
	previewByToken:      map[string]telegramPreviewItem{},
}

var workspaceTakeOverCodexSessionForTelegram = workspaceTakeOverCodexSession

func telegramHTTPClient(proxyURL string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	for _, raw := range telegramProxyCandidates(proxyURL) {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" {
			return nil, fmt.Errorf("invalid Telegram proxy URL")
		}
		if strings.HasPrefix(strings.ToLower(parsed.Scheme), "socks") {
			dialer, err := proxy.FromURL(parsed, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("invalid Telegram SOCKS proxy: %w", err)
			}
			transport.Proxy = nil
			transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		} else if parsed.Scheme == "http" || parsed.Scheme == "https" {
			transport.Proxy = http.ProxyURL(parsed)
		} else {
			return nil, fmt.Errorf("unsupported Telegram proxy scheme %q", parsed.Scheme)
		}
		return &http.Client{Transport: transport, Timeout: 40 * time.Second}, nil
	}
	return nil, fmt.Errorf("Telegram proxy is required")
}

func telegramProxyCandidates(configured string) []string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return []string{configured}
	}
	return []string{
		os.Getenv("ABOLQASEM_TELEGRAM_PROXY"),
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("https_proxy"),
		os.Getenv("ALL_PROXY"),
		os.Getenv("all_proxy"),
	}
}

func (b *telegramBridge) Reload() {
	config, err := loadTelegramBridgeConfig()
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.lastError = err.Error()
		return
	}
	client, proxyErr := telegramHTTPClient(config.ProxyURL)
	fingerprint := telegramBridgeConfigFingerprint(config)
	if config.BotToken == "" || len(config.AllowedUserIDs) == 0 {
		if b.cancel != nil {
			b.cancel()
		}
		b.cancel, b.active, b.fingerprint, b.lastError = nil, false, "", ""
		return
	}
	if proxyErr != nil {
		if b.cancel != nil {
			b.cancel()
		}
		b.cancel, b.active, b.fingerprint, b.lastError = nil, false, "", proxyErr.Error()
		return
	}
	if b.active && fingerprint == b.fingerprint {
		return
	}
	if b.cancel != nil {
		b.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.client = client
	b.cancel, b.active, b.fingerprint, b.lastError = cancel, true, fingerprint, ""
	go b.poll(ctx, config)
	go b.initializeBot(ctx, config)
}

func telegramBridgeConfigFingerprint(config telegramBridgeConfig) string {
	commands, _ := json.Marshal(config.CustomCommands)
	return config.BotToken + "|" + config.ProxyURL + "|" + strings.Join(config.AllowedUserIDs, ",") + "|" + string(commands)
}

func (b *telegramBridge) initializeBot(ctx context.Context, config telegramBridgeConfig) {
	b.syncBotCommands(ctx, config.BotToken)
	for _, chatID := range config.ChatIDs {
		b.sendText(ctx, config.BotToken, chatID, "پل ابوالقاسم دوباره آنلاین شد.")
	}
}

func (b *telegramBridge) Status(config telegramBridgeConfig) map[string]any {
	_, proxyErr := telegramHTTPClient(config.ProxyURL)
	b.mu.Lock()
	defer b.mu.Unlock()
	allowAll := false
	for _, value := range config.AllowedUserIDs {
		allowAll = allowAll || value == "*"
	}
	return map[string]any{
		"configured":      config.BotToken != "" && len(config.AllowedUserIDs) > 0,
		"active":          b.active,
		"mappedChats":     len(config.Mappings),
		"mappedThreads":   len(config.Mappings),
		"allowedUsers":    len(config.AllowedUserIDs),
		"knownChats":      len(config.ChatIDs),
		"allowAllUsers":   allowAll,
		"lastError":       b.lastError,
		"proxyConfigured": proxyErr == nil,
	}
}

func (b *telegramBridge) SendTest(ctx context.Context, config telegramBridgeConfig) error {
	if config.BotToken == "" {
		return errors.New("ربات Telegram هنوز تنظیم نشده است")
	}
	if len(config.ChatIDs) == 0 {
		return errors.New("هنوز چت Telegram شناخته‌شده‌ای وجود ندارد؛ ابتدا در ربات /start را بزنید")
	}
	message := "# پیام آزمایشی ابوالقاسم\n\nاتصال ربات، پروکسی، Rich Markdown و نمایش راست‌به‌چپ سالم است."
	payload := map[string]any{
		"chat_id": config.ChatIDs[0],
		"rich_message": map[string]any{
			"markdown": message,
			"is_rtl":   true,
		},
	}
	if err := b.callTelegram(ctx, config.BotToken, "sendRichMessage", payload); err != nil {
		b.setError(err)
		return err
	}
	b.setError(nil)
	return nil
}

func (b *telegramBridge) setError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.lastError = ""
		return
	}
	b.lastError = err.Error()
}

func (b *telegramBridge) poll(ctx context.Context, config telegramBridgeConfig) {
	var offset int64
	for ctx.Err() == nil {
		updates, err := b.getUpdates(ctx, config.BotToken, offset)
		if err != nil {
			b.setError(err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		b.setError(nil)
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message != nil {
				b.handleMessage(ctx, config, *update.Message)
			} else if update.CallbackQuery != nil {
				b.handleCallbackQuery(ctx, config, *update.CallbackQuery)
			}
		}
	}
}

func (b *telegramBridge) getUpdates(ctx context.Context, token string, offset int64) ([]telegramUpdate, error) {
	body, _ := json.Marshal(map[string]any{
		"timeout":         25,
		"offset":          offset,
		"allowed_updates": []string{"message", "callback_query"},
	})
	endpoint := strings.TrimRight(b.apiBaseURL, "/") + "/bot" + url.PathEscape(token) + "/getUpdates"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	var response telegramAPIResponse
	if err := b.do(request, &response); err != nil {
		return nil, redactTelegramToken(err, token)
	}
	if !response.OK {
		return nil, redactTelegramToken(fmt.Errorf("telegram getUpdates: %s", firstNonEmptyString(response.Error, "request failed")), token)
	}
	var updates []telegramUpdate
	if err := json.Unmarshal(response.Result, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (b *telegramBridge) do(request *http.Request, target *telegramAPIResponse) error {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("telegram http status %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(target)
}

func (b *telegramBridge) handleMessage(ctx context.Context, config telegramBridgeConfig, message telegramMessage) {
	// Per-chat preferences can change from inline buttons while the poller keeps
	// its network configuration snapshot. Refresh the persisted state per prompt.
	if latest, err := loadTelegramBridgeConfig(); err == nil {
		config = latest
	}
	chatID, userID := fmt.Sprintf("%d", message.Chat.ID), fmt.Sprintf("%d", message.From.ID)
	if !telegramUserAllowed(config, userID) {
		b.sendText(ctx, config.BotToken, chatID, "این کاربر اجازهٔ استفاده از ربات را ندارد.\n\nشناسهٔ کاربر شما: `"+userID+"`\n\nاین شناسه را در allowlist تنظیمات Telegram اضافه کنید.")
		return
	}
	b.rememberChatID(&config, chatID)
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return
	}
	command, argument := telegramCommand(text)
	if strings.HasPrefix(command, "chat_") {
		argument = strings.TrimPrefix(command, "chat_")
		command = "chat"
	}
	switch command {
	case "start":
		b.sendTextWithMarkup(ctx, config.BotToken, chatID, telegramHelpMarkdown(), telegramDashboardMarkup())
		return
	case "help":
		b.sendText(ctx, config.BotToken, chatID, telegramHelpMarkdown())
		return
	case "whoami":
		b.sendText(ctx, config.BotToken, chatID, "شناسه‌های تلگرام:\n\n- کاربر: `"+userID+"`\n- چت: `"+chatID+"`")
		return
	case "status":
		status := b.Status(config)
		b.sendText(ctx, config.BotToken, chatID, fmt.Sprintf("وضعیت پل تلگرام:\n\n- فعال: `%v`\n- اتصال‌ها: `%v`\n- آخرین خطا: `%v`", status["active"], status["mappedChats"], status["lastError"]))
		return
	case "commands":
		b.sendText(ctx, config.BotToken, chatID, telegramCustomCommandHelpMarkdown(config.CustomCommands))
		return
	case "settings", "model", "thinking":
		b.sendTelegramSessionSettings(ctx, config, chatID)
		return
	case "run":
		name := strings.ToLower(strings.TrimSpace(argument))
		if strings.ContainsAny(name, " \t\n") || name == "" {
			b.sendText(ctx, config.BotToken, chatID, "استفاده: `/run <name>`\n\nبرای دیدن فرمان‌های مجاز: /commands")
			return
		}
		customCommand, ok := telegramCustomCommandByName(config.CustomCommands, name)
		if !ok {
			b.sendText(ctx, config.BotToken, chatID, "این فرمان در allowlist نیست. برای دیدن فرمان‌های مجاز: /commands")
			return
		}
		b.sendText(ctx, config.BotToken, chatID, "در حال اجرای `/run "+telegramMarkdownInline(customCommand.Name)+"`…")
		go func(command telegramCustomCommand) {
			output, err := runTelegramCustomCommand(context.Background(), command)
			if err != nil {
				b.sendText(context.Background(), config.BotToken, chatID, "اجرای `/run "+telegramMarkdownInline(command.Name)+"` ناموفق بود:\n\n```text\n"+telegramCodeBlock(outputOrError(output, err))+"\n```")
				return
			}
			b.sendText(context.Background(), config.BotToken, chatID, "خروجی `/run "+telegramMarkdownInline(command.Name)+"`:\n\n```text\n"+telegramCodeBlock(output)+"\n```")
		}(customCommand)
		return
	case "chats", "threads", "projects":
		b.sendProjectPicker(ctx, config, chatID)
		return
	case "newchat", "newthread":
		_, err := createTelegramChat(&config, chatID)
		if err != nil {
			b.sendText(ctx, config.BotToken, chatID, "ساخت نشست تازه ناموفق بود: "+err.Error())
			return
		}
		b.sendTelegramSessionSettings(ctx, config, chatID)
		return
	case "current":
		if strings.TrimSpace(config.Mappings[chatID]) != "" {
			b.sendTelegramSessionSettings(ctx, config, chatID)
		} else {
			b.sendText(ctx, config.BotToken, chatID, "هنوز به چتی متصل نیستید. /chat <chat-id>")
		}
		return
	case "chat", "thread":
		target := argument
		if index, err := strconv.Atoi(argument); err == nil {
			choices := telegramChatChoices(0)
			if index < 1 || index > len(choices) {
				b.sendText(ctx, config.BotToken, chatID, "شمارهٔ نشست معتبر نیست. برای دیدن فهرست تازه /chats را بزنید.")
				return
			}
			target = choices[index-1].ChatID
		}
		if _, _, err := workspaceChatProjectRequired(target); err != nil {
			b.sendText(ctx, config.BotToken, chatID, "chat-id معتبر نیست: "+err.Error())
			return
		}
		config.Mappings[chatID] = target
		if err := saveTelegramBridgeRuntimeState(config); err != nil {
			b.sendText(ctx, config.BotToken, chatID, "ذخیرهٔ اتصال ناموفق بود: "+err.Error())
			return
		}
		b.sendTelegramSessionSettings(ctx, config, chatID)
		return
	case "history":
		target := strings.TrimSpace(config.Mappings[chatID])
		if target == "" {
			b.sendText(ctx, config.BotToken, chatID, "هنوز نشستی انتخاب نشده است. برای انتخاب /chats را بزنید.")
			return
		}
		b.sendText(ctx, config.BotToken, chatID, telegramChatHistoryMarkdown(target))
		return
	case "":
		// A normal message is forwarded below.
	default:
		b.sendText(ctx, config.BotToken, chatID, "دستور ناشناخته است. /help")
		return
	}
	target := strings.TrimSpace(config.Mappings[chatID])
	if target == "" {
		var err error
		target, err = createTelegramChat(&config, chatID)
		if err != nil {
			b.sendText(ctx, config.BotToken, chatID, "ساخت خودکار نشست ناموفق بود: "+err.Error()+"\n\nبرای انتخاب نشست موجود /chats را بزنید.")
			return
		}
	}
	if err := workspaceEnsureCodexChatWritable(target); err != nil {
		b.sendTextWithMarkup(ctx, config.BotToken, chatID, "این chat قفل است: "+err.Error(), telegramTakeOverLockMarkup(target))
		return
	}
	pref := config.Preferences[chatID]
	model, effort := telegramCodexSelection(pref)
	result, err := workspaceAgentCoordinator().Send(ctx, agent.SendCommand{
		ChatID:       target,
		Content:      text,
		Provider:     "codex",
		Model:        model,
		Effort:       effort,
		ModelOptions: &catalog.ModelOptions{Codex: &catalog.CodexModelOptionsPatch{ReasoningEffort: effort}},
	})
	if err != nil {
		b.sendText(ctx, config.BotToken, chatID, "ارسال ناموفق بود: "+err.Error())
		return
	}
	if result.Queued {
		b.sendText(ctx, config.BotToken, chatID, "پیام در صف قرار گرفت.")
		return
	}
	b.sendText(ctx, config.BotToken, chatID, "در حال دریافت پاسخ…")
}

func (b *telegramBridge) handleCallbackQuery(ctx context.Context, config telegramBridgeConfig, callback telegramCallbackQuery) {
	if latest, err := loadTelegramBridgeConfig(); err == nil {
		config = latest
	}
	chatID := ""
	if callback.Message != nil {
		chatID = fmt.Sprintf("%d", callback.Message.Chat.ID)
	}
	userID := fmt.Sprintf("%d", callback.From.ID)
	if !telegramUserAllowed(config, userID) {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "اجازهٔ دسترسی ندارید")
		if chatID != "" {
			b.sendText(ctx, config.BotToken, chatID, "این کاربر اجازهٔ استفاده از ربات را ندارد. شناسهٔ شما: `"+userID+"`")
		}
		return
	}
	if chatID == "" {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "انتخاب نامعتبر است")
		return
	}
	if preview, ok := b.telegramPreviewForCallback(callback.Data, chatID, config.Mappings[chatID]); ok {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "در حال آماده‌سازی پیش‌نمایش…")
		go b.sendTelegramPreview(context.Background(), config.BotToken, chatID, preview)
		return
	}
	if callback.Data == telegramTelegramPickerCallback {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "فهرست نشست‌ها")
		b.sendProjectPicker(ctx, config, chatID)
		return
	}
	if callback.Data == "settings" || callback.Data == "model" || callback.Data == "thinking" {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "تنظیمات نشست")
		b.sendTelegramSessionSettings(ctx, config, chatID)
		return
	}
	if callback.Data == "history" {
		target := strings.TrimSpace(config.Mappings[chatID])
		if target == "" {
			b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "ابتدا یک نشست انتخاب کنید")
			b.sendTextWithMarkup(ctx, config.BotToken, chatID, "هنوز نشستی انتخاب نشده است.", telegramDashboardMarkup())
			return
		}
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "تاریخچهٔ اخیر")
		b.sendTextWithMarkup(ctx, config.BotToken, chatID, telegramChatHistoryMarkdown(target), telegramSessionSettingsMarkup(config, chatID))
		return
	}
	if callback.Data == "tm" {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "انتخاب مدل")
		b.sendTelegramModelPicker(ctx, config, chatID)
		return
	}
	if callback.Data == "te" {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "انتخاب سطح فکر")
		b.sendTelegramEffortPicker(ctx, config, chatID)
		return
	}
	if modelIndex, ok := telegramModelCallback(callback.Data); ok {
		b.selectTelegramModel(ctx, config, callback.ID, chatID, modelIndex)
		return
	}
	if effort, ok := telegramEffortCallback(callback.Data); ok {
		b.selectTelegramEffort(ctx, config, callback.ID, chatID, effort)
		return
	}
	if target, ok := telegramTakeOverLockChatID(callback.Data); ok {
		if _, _, err := workspaceChatProjectRequired(target); err != nil {
			b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "نشست پیدا نشد")
			return
		}
		b.startCodexTakeOverFromTelegram(ctx, config, callback.ID, chatID, target)
		return
	}
	if page, ok := telegramProjectsPage(callback.Data); ok {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "صفحه تغییر کرد")
		b.sendProjectPickerPage(ctx, config, chatID, page)
		return
	}
	if projectID, page, ok := telegramProjectChatsPage(callback.Data); ok {
		if len(telegramChatChoicesForProject(projectID)) == 0 {
			b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "پروژه یا چت‌های آن پیدا نشد")
			return
		}
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "صفحه تغییر کرد")
		b.sendProjectChatPickerPage(ctx, config, chatID, projectID, page)
		return
	}
	if projectID, ok := telegramProjectCallbackProjectID(callback.Data); ok {
		if len(telegramChatChoicesForProject(projectID)) == 0 {
			b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "پروژه یا چت‌های آن پیدا نشد")
			return
		}
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "پروژه انتخاب شد")
		b.sendProjectChatPicker(ctx, config, chatID, projectID)
		return
	}
	if !strings.HasPrefix(callback.Data, "chat:") {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "انتخاب نامعتبر است")
		return
	}
	target := strings.TrimSpace(strings.TrimPrefix(callback.Data, "chat:"))
	if _, _, err := workspaceChatProjectRequired(target); err != nil {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "نشست پیدا نشد")
		return
	}
	b.rememberChatID(&config, chatID)
	config.Mappings[chatID] = target
	if err := saveTelegramBridgeRuntimeState(config); err != nil {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "ذخیره ناموفق بود")
		b.setError(err)
		return
	}
	b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "نشست انتخاب شد")
	b.sendText(ctx, config.BotToken, chatID, "نشست انتخاب شد: `"+target+"`")
	b.sendText(ctx, config.BotToken, chatID, telegramChatHistoryMarkdown(target))
}

func (b *telegramBridge) startCodexTakeOverFromTelegram(ctx context.Context, config telegramBridgeConfig, callbackID string, telegramChatID string, targetChatID string) {
	// Telegram expects callback queries to be acknowledged quickly. Claiming a
	// Codex app-server can take several seconds, so keep it off the polling loop.
	b.answerCallbackQuery(ctx, config.BotToken, callbackID, "در حال گرفتن نشست…")
	go func() {
		status, err := workspaceTakeOverCodexSessionForTelegram(targetChatID, true, "dangerous")
		if err != nil {
			b.sendTextWithMarkup(context.Background(), config.BotToken, telegramChatID, "گرفتن قفل نشست ناموفق بود: "+err.Error(), telegramTakeOverLockMarkupForStatus(targetChatID, status))
			return
		}
		b.sendText(context.Background(), config.BotToken, telegramChatID, "قفل نشست توسط ابوالقاسم گرفته شد. پیام بعدی شما ارسال می‌شود.")
	}()
}

func telegramTakeOverLockMarkup(chatID string) any {
	chat, err := workspaceChatRequired(chatID)
	if err != nil {
		return nil
	}
	return telegramTakeOverLockMarkupForStatus(chatID, workspaceCodexLockStatus(chat))
}

func telegramTakeOverLockMarkupForStatus(chatID string, status readmodels.CodexLockStatus) any {
	if status.State != codexLockOwnedElsewhere || !status.CanTakeOver || strings.TrimSpace(chatID) == "" {
		return nil
	}
	return map[string]any{
		"inline_keyboard": [][]map[string]string{{{
			"text":          "🔓 گرفتن نشست",
			"callback_data": telegramTakeOverLockCallbackPrefix + chatID,
			"style":         "danger",
		}}},
	}
}

func telegramTakeOverLockChatID(data string) (string, bool) {
	chatID := strings.TrimSpace(strings.TrimPrefix(data, telegramTakeOverLockCallbackPrefix))
	if !strings.HasPrefix(data, telegramTakeOverLockCallbackPrefix) || chatID == "" || len([]byte(data)) > 64 {
		return "", false
	}
	return chatID, true
}

func (b *telegramBridge) rememberChatID(config *telegramBridgeConfig, chatID string) {
	for _, existing := range config.ChatIDs {
		if existing == chatID {
			return
		}
	}
	config.ChatIDs = append([]string{chatID}, config.ChatIDs...)
	if len(config.ChatIDs) > 50 {
		config.ChatIDs = config.ChatIDs[:50]
	}
	if err := saveTelegramBridgeRuntimeState(*config); err != nil {
		b.setError(err)
	}
}

type telegramChatChoice struct {
	ChatID       string
	ChatTitle    string
	ProjectID    string
	ProjectTitle string
	UpdatedAt    int64
}

type telegramProjectChoice struct {
	ProjectID string
	Title     string
	UpdatedAt int64
	ChatCount int
}

func telegramChatChoices(limit int) []telegramChatChoice {
	sidebar, _ := workspaceSidebarSnapshot().(readmodels.SidebarData)
	return telegramChatChoicesFromSidebar(sidebar, limit)
}

func telegramChatChoicesFromSidebar(sidebar readmodels.SidebarData, limit int) []telegramChatChoice {
	choices := make([]telegramChatChoice, 0)
	seen := map[string]bool{}
	for _, group := range sidebar.ProjectGroups {
		for _, chat := range group.Chats {
			if chat.ChatID == "" || seen[chat.ChatID] {
				continue
			}
			seen[chat.ChatID] = true
			choices = append(choices, telegramChatChoice{
				ChatID:       chat.ChatID,
				ChatTitle:    firstNonEmptyString(chat.Title, "بدون عنوان"),
				ProjectID:    group.GroupKey,
				ProjectTitle: firstNonEmptyString(group.SidebarTitle, group.Title, group.RealTitle, "بدون پروژه"),
				UpdatedAt:    sidebarChatTimestamp(chat),
			})
		}
	}
	sort.SliceStable(choices, func(i, j int) bool { return choices[i].UpdatedAt > choices[j].UpdatedAt })
	if limit > 0 && len(choices) > limit {
		choices = choices[:limit]
	}
	return choices
}

func telegramProjectChoices(limit int) []telegramProjectChoice {
	sidebar, _ := workspaceSidebarSnapshot().(readmodels.SidebarData)
	return telegramProjectChoicesFromSidebar(sidebar, limit)
}

func telegramProjectChoicesFromSidebar(sidebar readmodels.SidebarData, limit int) []telegramProjectChoice {
	choices := make([]telegramProjectChoice, 0, len(sidebar.ProjectGroups))
	for _, group := range sidebar.ProjectGroups {
		if strings.TrimSpace(group.GroupKey) == "" || len(group.Chats) == 0 {
			continue
		}
		choice := telegramProjectChoice{
			ProjectID: group.GroupKey,
			Title:     firstNonEmptyString(group.SidebarTitle, group.Title, group.RealTitle, "بدون پروژه"),
			ChatCount: len(group.Chats),
		}
		for _, chat := range group.Chats {
			choice.UpdatedAt = max(choice.UpdatedAt, sidebarChatTimestamp(chat))
		}
		choices = append(choices, choice)
	}
	sort.SliceStable(choices, func(i, j int) bool { return choices[i].UpdatedAt > choices[j].UpdatedAt })
	if limit > 0 && len(choices) > limit {
		choices = choices[:limit]
	}
	return choices
}

func telegramChatChoicesForProject(projectID string) []telegramChatChoice {
	return telegramChatChoicesForProjectFromChoices(projectID, telegramChatChoices(0))
}

func telegramChatChoicesForProjectFromChoices(projectID string, choices []telegramChatChoice) []telegramChatChoice {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	filtered := make([]telegramChatChoice, 0, len(choices))
	for _, choice := range choices {
		if choice.ProjectID == projectID {
			filtered = append(filtered, choice)
		}
	}
	return filtered
}

func telegramChatListMarkdown(currentChatID string, page int) string {
	const pageSize = 10
	choices := telegramChatChoices(0)
	if len(choices) == 0 {
		return "هنوز نشستی برای انتخاب وجود ندارد. ابتدا در ابوالقاسم یک چت بسازید."
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start >= len(choices) {
		return "این صفحه وجود ندارد. برای برگشت به ابتدای فهرست /chats را بزنید."
	}
	end := min(start+pageSize, len(choices))
	var result strings.Builder
	fmt.Fprintf(&result, "# انتخاب نشست\n\nبرای اتصال، دکمهٔ نشست یا فرمان آن را بزنید. صفحهٔ %d از %d:\n\n", page, (len(choices)+pageSize-1)/pageSize)
	for index := start; index < end; index++ {
		choice := choices[index]
		marker := ""
		if choice.ChatID == currentChatID {
			marker = " ← نشست فعلی"
		}
		fmt.Fprintf(&result, "%d. /chat_%d — **%s** · %s%s\n", index+1, index+1, telegramMarkdownInline(choice.ProjectTitle), telegramMarkdownInline(choice.ChatTitle), marker)
	}
	if end < len(choices) {
		fmt.Fprintf(&result, "\nبرای صفحهٔ بعد /chats %d را بزنید.", page+1)
	}
	return result.String()
}

func (b *telegramBridge) sendChatPicker(ctx context.Context, config telegramBridgeConfig, telegramChatID string) {
	b.sendProjectPicker(ctx, config, telegramChatID)
}

func (b *telegramBridge) sendProjectPicker(ctx context.Context, config telegramBridgeConfig, telegramChatID string) {
	b.sendProjectPickerPage(ctx, config, telegramChatID, 1)
}

func (b *telegramBridge) sendProjectPickerPage(ctx context.Context, config telegramBridgeConfig, telegramChatID string, page int) {
	choices := telegramProjectChoices(0)
	markdown := telegramProjectListMarkdownForChoices(choices, page)
	markup := telegramProjectPickerMarkupForChoices(choices, page)
	b.sendTextWithMarkup(ctx, config.BotToken, telegramChatID, markdown, markup)
}

func telegramProjectListMarkdownForChoices(choices []telegramProjectChoice, page int) string {
	if len(choices) == 0 {
		return "هنوز پروژه‌ای با chat قابل انتخاب وجود ندارد. ابتدا در ابوالقاسم یک chat بسازید."
	}
	start, end, page, pageCount, ok := telegramPageBounds(len(choices), page)
	if !ok {
		return "این صفحه وجود ندارد. برای برگشت به ابتدای فهرست /chats را بزنید."
	}
	_ = start
	_ = end
	return fmt.Sprintf("# انتخاب پروژه\n\nاز دکمه‌های زیر یک پروژه را انتخاب کنید تا chatهای آن نمایش داده شوند. صفحهٔ %d از %d.", page, pageCount)
}

func telegramProjectPickerMarkupForChoices(choices []telegramProjectChoice, page int) any {
	if len(choices) == 0 {
		return nil
	}
	start, end, page, pageCount, ok := telegramPageBounds(len(choices), page)
	if !ok {
		return nil
	}
	rows := make([][]map[string]string, 0, telegramPickerPageSize+1)
	for _, choice := range choices[start:end] {
		callbackData := telegramProjectCallbackPrefix + choice.ProjectID
		if len([]byte(callbackData)) > 64 {
			continue
		}
		rows = append(rows, []map[string]string{{
			"text":          truncateTelegramButtonLabel(choice.Title+" · "+strconv.Itoa(choice.ChatCount), 60),
			"callback_data": callbackData,
		}})
	}
	rows = appendTelegramPaginationRow(rows, page, pageCount, func(target int) string {
		return telegramProjectsPageCallbackPrefix + strconv.Itoa(target)
	})
	if len(rows) == 0 {
		return nil
	}
	return map[string]any{"inline_keyboard": rows}
}

func (b *telegramBridge) sendProjectChatPicker(ctx context.Context, config telegramBridgeConfig, telegramChatID string, projectID string) {
	b.sendProjectChatPickerPage(ctx, config, telegramChatID, projectID, 1)
}

func (b *telegramBridge) sendProjectChatPickerPage(ctx context.Context, config telegramBridgeConfig, telegramChatID string, projectID string, page int) {
	choices := telegramChatChoicesForProject(projectID)
	if len(choices) == 0 {
		b.sendText(ctx, config.BotToken, telegramChatID, "برای این پروژه chat قابل انتخابی پیدا نشد.")
		return
	}
	markdown := telegramProjectChatListMarkdown(config.Mappings[telegramChatID], choices, page)
	markup := telegramProjectChatPickerMarkup(config.Mappings[telegramChatID], choices, page)
	b.sendTextWithMarkup(ctx, config.BotToken, telegramChatID, markdown, markup)
}

func telegramProjectChatListMarkdown(currentChatID string, choices []telegramChatChoice, page int) string {
	start, end, page, pageCount, ok := telegramPageBounds(len(choices), page)
	if !ok {
		return "این صفحه وجود ندارد. برای برگشت به فهرست پروژه‌ها /chats را بزنید."
	}
	_ = start
	_ = end
	current := ""
	if currentChatID != "" {
		current = " نشست فعلی با علامت ✓ مشخص است."
	}
	return fmt.Sprintf("# انتخاب chat\n\nاز دکمه‌های زیر chat موردنظر را انتخاب کنید. صفحهٔ %d از %d.%s", page, pageCount, current)
}

func telegramProjectChatPickerMarkup(currentChatID string, choices []telegramChatChoice, page int) any {
	start, end, page, pageCount, ok := telegramPageBounds(len(choices), page)
	if !ok {
		return nil
	}
	rows := make([][]map[string]string, 0, telegramPickerPageSize+1)
	for _, choice := range choices[start:end] {
		label := choice.ChatTitle
		if choice.ChatID == currentChatID {
			label = "✓ " + label
		}
		button := map[string]string{
			"text":          truncateTelegramButtonLabel(label, 60),
			"callback_data": "chat:" + choice.ChatID,
		}
		if choice.ChatID == currentChatID {
			button["style"] = "success"
		}
		rows = append(rows, []map[string]string{button})
	}
	rows = appendTelegramPaginationRow(rows, page, pageCount, func(target int) string {
		return telegramProjectChatsPageCallbackPrefix + strconv.Itoa(target) + ":" + choices[0].ProjectID
	})
	return map[string]any{"inline_keyboard": rows}
}

func telegramPageBounds(total int, page int) (start int, end int, normalizedPage int, pageCount int, ok bool) {
	if total == 0 || page < 1 {
		return 0, 0, page, 0, false
	}
	pageCount = (total + telegramPickerPageSize - 1) / telegramPickerPageSize
	if page > pageCount {
		return 0, 0, page, pageCount, false
	}
	start = (page - 1) * telegramPickerPageSize
	return start, min(start+telegramPickerPageSize, total), page, pageCount, true
}

func appendTelegramPaginationRow(rows [][]map[string]string, page int, pageCount int, callback func(int) string) [][]map[string]string {
	buttons := make([]map[string]string, 0, 2)
	if page > 1 {
		buttons = append(buttons, map[string]string{"text": "« قبلی", "callback_data": callback(page - 1), "style": "link"})
	}
	if page < pageCount {
		buttons = append(buttons, map[string]string{"text": "بعدی »", "callback_data": callback(page + 1), "style": "link"})
	}
	if len(buttons) > 0 {
		rows = append(rows, buttons)
	}
	return rows
}

func telegramProjectsPage(data string) (int, bool) {
	return telegramCallbackPage(data, telegramProjectsPageCallbackPrefix)
}

func telegramProjectChatsPage(data string) (string, int, bool) {
	if !strings.HasPrefix(data, telegramProjectChatsPageCallbackPrefix) || len([]byte(data)) > 64 {
		return "", 0, false
	}
	parts := strings.SplitN(strings.TrimPrefix(data, telegramProjectChatsPageCallbackPrefix), ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", 0, false
	}
	page, err := strconv.Atoi(parts[0])
	if err != nil || page < 1 {
		return "", 0, false
	}
	return strings.TrimSpace(parts[1]), page, true
}

func telegramCallbackPage(data string, prefix string) (int, bool) {
	if !strings.HasPrefix(data, prefix) || len([]byte(data)) > 64 {
		return 0, false
	}
	page, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(data, prefix)))
	return page, err == nil && page > 0
}

func telegramProjectCallbackProjectID(data string) (string, bool) {
	projectID := strings.TrimSpace(strings.TrimPrefix(data, telegramProjectCallbackPrefix))
	if !strings.HasPrefix(data, telegramProjectCallbackPrefix) || projectID == "" || len([]byte(data)) > 64 {
		return "", false
	}
	return projectID, true
}

func truncateTelegramButtonLabel(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func telegramHelpMarkdown() string {
	return "# راهنمای پل تلگرام\n\n" +
		"پیام عادی به نشست انتخاب‌شده فرستاده می‌شود و اگر هنوز نشستی انتخاب نشده باشد، یک نشست تازه به‌صورت خودکار ساخته می‌شود.\n\n" +
		"- `/chats` یا `/projects` — انتخاب پروژه و سپس chat\n" +
		"- `/newchat` — ساخت و انتخاب نشست تازه\n" +
		"- `/chat <id>` — انتخاب مستقیم یک نشست\n" +
		"- `/current` — نمایش نشست فعلی\n" +
		"- `/settings` — انتخاب مدل و سطح فکر برای پیام‌های بعدی\n" +
		"- `/history` — نمایش تاریخچهٔ اخیر\n" +
		"- `/status` — وضعیت پل و اتصال\n" +
		"- `/commands` — فهرست فرمان‌های سیستمی مجاز\n" +
		"- `/run <name>` — اجرای یک فرمان از پیش‌تعریف‌شده\n" +
		"- `/whoami` — شناسه‌های Telegram\n" +
		"- `/help` — همین راهنما\n\n" +
		"فرمان‌های سازگار با Codex Mobile یعنی `/threads`، `/newthread` و `/thread <id>` نیز پشتیبانی می‌شوند."
}

func telegramCustomCommandHelpMarkdown(commands []telegramCustomCommand) string {
	if len(commands) == 0 {
		return "هیچ فرمان سیستمی مجازی تعریف نشده است. آن را از Settings → Telegram اضافه کنید."
	}
	var result strings.Builder
	result.WriteString("# فرمان‌های سیستمی مجاز\n\nفقط این فرمان‌های از پیش‌تعریف‌شده، بدون آرگومان، اجرا می‌شوند.\n\n")
	for _, command := range commands {
		fmt.Fprintf(&result, "- `/run %s`", telegramMarkdownInline(command.Name))
		if command.Description != "" {
			fmt.Fprintf(&result, " — %s", telegramMarkdownInline(command.Description))
		}
		result.WriteString("\n")
	}
	return result.String()
}

func telegramCustomCommandByName(commands []telegramCustomCommand, name string) (telegramCustomCommand, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return telegramCustomCommand{}, false
}

func runTelegramCustomCommand(ctx context.Context, command telegramCustomCommand) (string, error) {
	timeout := time.Duration(command.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = 30 * time.Second
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runContext, "sh", "-lc", command.Command)
	if command.WorkingDirectory != "" {
		cmd.Dir = command.WorkingDirectory
	}
	output := &telegramCappedOutput{limit: telegramCustomCommandOutputLimit}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	result := strings.TrimSpace(output.String())
	if output.truncated {
		result += "\n\n[output truncated]"
	}
	if errors.Is(runContext.Err(), context.DeadlineExceeded) {
		return result, fmt.Errorf("timed out after %s", timeout)
	}
	return result, err
}

type telegramCappedOutput struct {
	limit     int
	buffer    strings.Builder
	truncated bool
}

func (output *telegramCappedOutput) Write(value []byte) (int, error) {
	remaining := output.limit - output.buffer.Len()
	if remaining <= 0 {
		output.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = output.buffer.Write(value[:remaining])
		output.truncated = true
		return len(value), nil
	}
	_, _ = output.buffer.Write(value)
	return len(value), nil
}

func (output *telegramCappedOutput) String() string { return output.buffer.String() }

func outputOrError(output string, err error) string {
	if strings.TrimSpace(output) != "" {
		return output + "\n\n" + err.Error()
	}
	return err.Error()
}

func telegramCodexProviderCatalog() catalog.ProviderCatalogEntry {
	for _, provider := range workspaceAvailableProviders() {
		if provider.ID == "codex" {
			return provider
		}
	}
	return catalog.GetOrDefault("codex")
}

func telegramCodexModelChoices() []catalog.ProviderModelOption {
	provider := telegramCodexProviderCatalog()
	models := append([]catalog.ProviderModelOption(nil), provider.Models...)
	if len(models) == 0 {
		models = []catalog.ProviderModelOption{{ID: catalog.DefaultCodexModel, Label: catalog.DefaultCodexModel}}
	}
	return models
}

func telegramCodexEffortChoices() []catalog.ProviderEffortOption {
	return []catalog.ProviderEffortOption{
		{ID: "minimal", Label: "Minimal"},
		{ID: "low", Label: "Low"},
		{ID: "medium", Label: "Medium"},
		{ID: "high", Label: "High"},
		{ID: "xhigh", Label: "XHigh"},
	}
}

func telegramCodexSelection(preference telegramChatPreference) (string, string) {
	model := strings.TrimSpace(preference.Model)
	models := telegramCodexModelChoices()
	validModel := false
	for _, candidate := range models {
		if candidate.ID == model {
			validModel = true
			break
		}
	}
	if !validModel {
		model = telegramCodexProviderCatalog().DefaultModel
		if model == "" {
			model = models[0].ID
		}
	}
	effort := strings.TrimSpace(preference.ReasoningEffort)
	if !catalog.IsCodexReasoningEffort(effort) {
		effort = catalog.CodexRuntimeDefaultReasoningEffort()
	}
	return model, effort
}

func telegramModelCallback(data string) (int, bool) {
	if !strings.HasPrefix(data, telegramModelCallbackPrefix) || len([]byte(data)) > 64 {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(data, telegramModelCallbackPrefix))
	return index, err == nil && index >= 0
}

func telegramEffortCallback(data string) (string, bool) {
	if !strings.HasPrefix(data, telegramEffortCallbackPrefix) || len([]byte(data)) > 64 {
		return "", false
	}
	effort := strings.TrimPrefix(data, telegramEffortCallbackPrefix)
	return effort, catalog.IsCodexReasoningEffort(effort)
}

func telegramSessionSettingsMarkdown(config telegramBridgeConfig, telegramChatID string) string {
	target := strings.TrimSpace(config.Mappings[telegramChatID])
	if target == "" {
		return "# تنظیمات نشست\n\nهنوز نشستی انتخاب نشده است. ابتدا از دکمهٔ انتخاب نشست استفاده کنید."
	}
	model, effort := telegramCodexSelection(config.Preferences[telegramChatID])
	return fmt.Sprintf("# تنظیمات نشست\n\nنشست فعلی: `%s`\n\n**مدل:** `%s`\n\n**سطح فکر:** `%s`\n\nانتخاب‌ها فقط برای پیام‌های بعدی همین چت تلگرام ذخیره می‌شوند.", telegramMarkdownInline(target), telegramMarkdownInline(model), telegramMarkdownInline(effort))
}

func telegramSessionSettingsMarkup(config telegramBridgeConfig, telegramChatID string) any {
	model, effort := telegramCodexSelection(config.Preferences[telegramChatID])
	return map[string]any{"inline_keyboard": [][]map[string]string{
		{{"text": "🧠 مدل: " + truncateTelegramButtonLabel(model, 28), "callback_data": "tm"}, {"text": "🎯 سطح فکر: " + effort, "callback_data": "te"}},
		{{"text": "🗂 نشست‌ها", "callback_data": telegramTelegramPickerCallback}, {"text": "📜 تاریخچه", "callback_data": "history"}},
	}}
}

func telegramDashboardMarkup() any {
	return map[string]any{"inline_keyboard": [][]map[string]string{
		{{"text": "🗂 انتخاب نشست", "callback_data": telegramTelegramPickerCallback, "style": "primary"}, {"text": "⚙️ تنظیمات نشست", "callback_data": "settings"}},
	}}
}

func (b *telegramBridge) sendTelegramSessionSettings(ctx context.Context, config telegramBridgeConfig, telegramChatID string) {
	b.sendTextWithMarkup(ctx, config.BotToken, telegramChatID, telegramSessionSettingsMarkdown(config, telegramChatID), telegramSessionSettingsMarkup(config, telegramChatID))
}

func (b *telegramBridge) sendTelegramModelPicker(ctx context.Context, config telegramBridgeConfig, telegramChatID string) {
	models := telegramCodexModelChoices()
	selectedModel, _ := telegramCodexSelection(config.Preferences[telegramChatID])
	rows := make([][]map[string]string, 0, len(models)+1)
	for index, model := range models {
		label := firstNonEmptyString(model.Label, model.ID)
		button := map[string]string{"text": truncateTelegramButtonLabel(label, 50), "callback_data": telegramModelCallbackPrefix + strconv.Itoa(index)}
		if model.ID == selectedModel {
			button["text"] = "✓ " + button["text"]
			button["style"] = "success"
		}
		rows = append(rows, []map[string]string{button})
	}
	rows = append(rows, []map[string]string{{"text": "↩ بازگشت", "callback_data": "settings", "style": "link"}})
	b.sendTextWithMarkup(ctx, config.BotToken, telegramChatID, "# انتخاب مدل\n\nمدل موردنظر را برای پیام‌های بعدی انتخاب کنید:", map[string]any{"inline_keyboard": rows})
}

func (b *telegramBridge) sendTelegramEffortPicker(ctx context.Context, config telegramBridgeConfig, telegramChatID string) {
	efforts := telegramCodexEffortChoices()
	_, selectedEffort := telegramCodexSelection(config.Preferences[telegramChatID])
	rows := make([][]map[string]string, 0, len(efforts)+1)
	for _, effort := range efforts {
		button := map[string]string{"text": effort.Label, "callback_data": telegramEffortCallbackPrefix + effort.ID}
		if effort.ID == selectedEffort {
			button["text"] = "✓ " + button["text"]
			button["style"] = "success"
		}
		rows = append(rows, []map[string]string{button})
	}
	rows = append(rows, []map[string]string{{"text": "↩ بازگشت", "callback_data": "settings", "style": "link"}})
	b.sendTextWithMarkup(ctx, config.BotToken, telegramChatID, "# انتخاب سطح فکر\n\nسطح reasoning را برای پیام‌های بعدی انتخاب کنید:", map[string]any{"inline_keyboard": rows})
}

func (b *telegramBridge) selectTelegramModel(ctx context.Context, config telegramBridgeConfig, callbackID, telegramChatID string, index int) {
	models := telegramCodexModelChoices()
	if index >= len(models) {
		b.answerCallbackQuery(ctx, config.BotToken, callbackID, "مدل پیدا نشد")
		return
	}
	preference := config.Preferences[telegramChatID]
	preference.Model = models[index].ID
	if err := saveTelegramBridgePreference(telegramChatID, preference); err != nil {
		b.answerCallbackQuery(ctx, config.BotToken, callbackID, "ذخیره ناموفق بود")
		b.setError(err)
		return
	}
	config.Preferences[telegramChatID] = preference
	b.answerCallbackQuery(ctx, config.BotToken, callbackID, "مدل ذخیره شد")
	b.sendTelegramSessionSettings(ctx, config, telegramChatID)
}

func (b *telegramBridge) selectTelegramEffort(ctx context.Context, config telegramBridgeConfig, callbackID, telegramChatID, effort string) {
	if !catalog.IsCodexReasoningEffort(effort) {
		b.answerCallbackQuery(ctx, config.BotToken, callbackID, "سطح فکر نامعتبر است")
		return
	}
	preference := config.Preferences[telegramChatID]
	preference.ReasoningEffort = effort
	if err := saveTelegramBridgePreference(telegramChatID, preference); err != nil {
		b.answerCallbackQuery(ctx, config.BotToken, callbackID, "ذخیره ناموفق بود")
		b.setError(err)
		return
	}
	config.Preferences[telegramChatID] = preference
	b.answerCallbackQuery(ctx, config.BotToken, callbackID, "سطح فکر ذخیره شد")
	b.sendTelegramSessionSettings(ctx, config, telegramChatID)
}

func telegramCodeBlock(value string) string {
	value = strings.ReplaceAll(value, "```", "''' ")
	if strings.TrimSpace(value) == "" {
		return "(no output)"
	}
	return value
}

func createTelegramChat(config *telegramBridgeConfig, telegramChatID string) (string, error) {
	var project readmodels.ProjectRecord
	if current := strings.TrimSpace(config.Mappings[telegramChatID]); current != "" {
		_, currentProject, err := workspaceChatProjectRequired(current)
		if err == nil {
			project = currentProject
		}
	}
	if project.ID == "" {
		if choices := telegramChatChoices(1); len(choices) > 0 {
			_, recentProject, err := workspaceChatProjectRequired(choices[0].ChatID)
			if err == nil {
				project = recentProject
			}
		}
	}
	if project.ID == "" {
		defaultCWD := strings.TrimSpace(os.Getenv("ABOLQASEM_TELEGRAM_DEFAULT_CWD"))
		if defaultCWD == "" {
			return "", errors.New("هیچ پروژه‌ای وجود ندارد؛ ابتدا یک پروژه در ابوالقاسم باز کنید یا ABOLQASEM_TELEGRAM_DEFAULT_CWD را تنظیم کنید")
		}
		var err error
		project, err = workspaceOpenProject(defaultCWD, "")
		if err != nil {
			return "", err
		}
	}
	chat, err := workspaceCreateChatWithOptions(project.ID, "codex", "")
	if err != nil {
		return "", err
	}
	if config.Mappings == nil {
		config.Mappings = map[string]string{}
	}
	config.Mappings[telegramChatID] = chat.ID
	if err := saveTelegramBridgeRuntimeState(*config); err != nil {
		return "", err
	}
	workspaceConnections.broadcast(chat.ID)
	return chat.ID, nil
}

func telegramChatHistoryMarkdown(chatID string) string {
	snapshot, _ := workspaceChatSnapshot(chatID, 80).(*readmodels.ChatSnapshot)
	if snapshot == nil {
		return "تاریخچهٔ این نشست پیدا نشد."
	}
	rows := make([]string, 0, 12)
	for index := len(snapshot.Messages) - 1; index >= 0 && len(rows) < 12; index-- {
		kind, role, text := workspaceEntrySearchText(snapshot.Messages[index])
		if text == "" || kind == transcript.KindToolCall || kind == transcript.KindToolResult || kind == transcript.KindStatus {
			continue
		}
		switch role {
		case "user":
			rows = append(rows, "## شما\n\n"+text)
		case "assistant":
			rows = append(rows, "## دستیار\n\n"+text)
		}
	}
	if len(rows) == 0 {
		return "این نشست هنوز تاریخچهٔ پیامی ندارد."
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	return "# تاریخچهٔ اخیر\n\n" + strings.Join(rows, "\n\n")
}

func telegramMarkdownInline(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	replacer := strings.NewReplacer(
		"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>", "|", "\\|",
	)
	return replacer.Replace(value)
}

func telegramUserAllowed(config telegramBridgeConfig, userID string) bool {
	for _, candidate := range config.AllowedUserIDs {
		if candidate == "*" || candidate == userID {
			return true
		}
	}
	return false
}

func telegramCommand(text string) (string, string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	command := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	command = strings.Split(command, "@")[0]
	return command, strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
}

func (b *telegramBridge) forwardFinal(token string, telegramChatID string, workspaceChatID string) {
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			b.sendText(context.Background(), token, telegramChatID, "پاسخ هنوز در حال اجراست؛ وضعیت را در وب ببینید.")
			return
		case <-ticker.C:
			if workspaceAgentCoordinator().ActiveStatuses()[workspaceChatID] != "" {
				continue
			}
			snapshot, _ := workspaceChatSnapshot(workspaceChatID, 80).(*readmodels.ChatSnapshot)
			if snapshot == nil {
				return
			}
			if text := telegramFinalText(snapshot.Messages); text != "" {
				b.sendTranscript(context.Background(), token, telegramChatID, workspaceChatID, text)
			}
			return
		}
	}
}

func telegramFinalText(entries []readmodels.TranscriptEntry) string {
	_, text := telegramFinalMessage(entries)
	return text
}

func telegramFinalMessage(entries []readmodels.TranscriptEntry) (string, string) {
	parts := make([]string, 0, 4)
	ids := make([]string, 0, 4)
	for index := len(entries) - 1; index >= 0 && len(parts) < 4; index-- {
		kind, role, text := workspaceEntrySearchText(entries[index])
		if role == "user" {
			break
		}
		if text == "" || role != "assistant" || kind == transcript.KindCompactSummary {
			continue
		}
		parts = append(parts, text)
		ids = append(ids, firstNonEmptyString(workspaceEntryString(entries[index], "_id"), text))
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
		ids[left], ids[right] = ids[right], ids[left]
	}
	return strings.Join(ids, "|"), strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (b *telegramBridge) chatStateChanged(chatID string) {
	if strings.TrimSpace(chatID) == "" || workspaceAgentCoordinator().ActiveStatuses()[chatID] != "" {
		return
	}
	snapshot, _ := workspaceChatSnapshot(chatID, 80).(*readmodels.ChatSnapshot)
	if snapshot == nil {
		return
	}
	fingerprint, text := telegramFinalMessage(snapshot.Messages)
	if fingerprint == "" || text == "" {
		return
	}
	config, err := loadTelegramBridgeConfig()
	if err != nil || config.BotToken == "" {
		return
	}
	recipients := make([]string, 0)
	for telegramChatID, mappedChatID := range config.Mappings {
		if mappedChatID == chatID {
			recipients = append(recipients, telegramChatID)
		}
	}
	if len(recipients) == 0 {
		return
	}
	b.mu.Lock()
	if b.lastForwardedByChat == nil {
		b.lastForwardedByChat = map[string]string{}
	}
	if b.lastForwardedByChat[chatID] == fingerprint {
		b.mu.Unlock()
		return
	}
	b.lastForwardedByChat[chatID] = fingerprint
	b.mu.Unlock()
	go func() {
		for _, telegramChatID := range recipients {
			b.sendTranscript(context.Background(), config.BotToken, telegramChatID, chatID, text)
		}
	}()
}

func (b *telegramBridge) sendText(ctx context.Context, token string, chatID string, text string) {
	b.sendTextWithMarkup(ctx, token, chatID, text, nil)
}

func (b *telegramBridge) sendTextWithMarkup(ctx context.Context, token string, chatID string, text string, replyMarkup any) {
	for _, chunk := range splitTelegramText(text) {
		content := map[string]any{
			"markdown": chunk,
			"is_rtl":   telegramMarkdownIsRTL(chunk),
		}
		// Telegram Bot API 10.3 models these actions as InputRichBlockButtons.
		// Send the concrete block payload instead of silently falling back to an
		// InlineKeyboardMarkup attached outside the rich message.
		if blocks, ok := telegramRichMessageBlocks(chunk, replyMarkup); ok {
			content = map[string]any{
				"blocks": blocks,
				"is_rtl": telegramMarkdownIsRTL(chunk),
			}
		}
		payload := map[string]any{
			"chat_id":      chatID,
			"rich_message": content,
		}
		// Unknown markup remains backwards compatible. All of our inline keyboard
		// maps are converted above to native rich button rows.
		if replyMarkup != nil && !telegramRichMarkupSupported(replyMarkup) {
			payload["reply_markup"] = replyMarkup
		}
		replyMarkup = nil
		if err := b.callTelegram(ctx, token, "sendRichMessage", payload); err != nil {
			b.setError(err)
			return
		}
	}
}

func telegramRichMarkupSupported(markup any) bool {
	_, ok := telegramInputRichBlockButtons(markup)
	return ok
}

// telegramInputRichBlockButtons converts the internal row builder into the
// exact Bot API InputRichBlockButtons wire shape.
func telegramInputRichBlockButtons(markup any) ([]map[string]any, bool) {
	container, ok := markup.(map[string]any)
	if !ok {
		return nil, false
	}
	rows, ok := container["inline_keyboard"].([][]map[string]string)
	if !ok || len(rows) == 0 {
		return nil, false
	}
	blocks := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		buttons := make([]map[string]any, 0, len(row))
		for _, button := range row {
			text := strings.TrimSpace(button["text"])
			callbackData := strings.TrimSpace(button["callback_data"])
			if text == "" || callbackData == "" || len([]byte(callbackData)) > 64 {
				continue
			}
			richButton := map[string]any{
				"text":          text,
				"callback_data": callbackData,
			}
			if style := strings.TrimSpace(button["style"]); style != "" {
				richButton["style"] = style
			}
			buttons = append(buttons, richButton)
		}
		if len(buttons) == 0 {
			continue
		}
		blocks = append(blocks, map[string]any{
			"type":    "buttons",
			"buttons": buttons,
			"align":   "right",
		})
	}
	if len(blocks) == 0 {
		return nil, false
	}
	return blocks, true
}

func telegramRichMessageBlocks(markdown string, markup any) ([]map[string]any, bool) {
	buttonBlocks, ok := telegramInputRichBlockButtons(markup)
	if !ok {
		return nil, false
	}
	blocks := telegramTextRichBlocks(markdown)
	blocks = append(blocks, buttonBlocks...)
	return blocks, true
}

func telegramTextRichBlocks(markdown string) []map[string]any {
	paragraphs := strings.Split(strings.TrimSpace(markdown), "\n\n")
	blocks := make([]map[string]any, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if strings.HasPrefix(paragraph, "# ") && !strings.Contains(strings.TrimPrefix(paragraph, "# "), "\n") {
			blocks = append(blocks, map[string]any{
				"type": "heading",
				"text": telegramRichPlainText(strings.TrimPrefix(paragraph, "# ")),
				"size": 2,
			})
			continue
		}
		blocks = append(blocks, map[string]any{
			"type": "paragraph",
			"text": telegramRichPlainText(paragraph),
		})
	}
	return blocks
}

func telegramRichPlainText(markdown string) string {
	replacer := strings.NewReplacer("**", "", "__", "", "`", "", `\*`, "*", `\_`, "_", `\[`, "[", `\]`, "]")
	return replacer.Replace(markdown)
}

func (b *telegramBridge) callTelegram(ctx context.Context, token string, method string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(b.apiBaseURL, "/") + "/bot" + url.PathEscape(token) + "/" + method
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return redactTelegramToken(err, token)
	}
	request.Header.Set("Content-Type", "application/json")
	var response telegramAPIResponse
	if err := b.do(request, &response); err != nil {
		return redactTelegramToken(err, token)
	}
	if !response.OK {
		return redactTelegramToken(fmt.Errorf("telegram %s: %s", method, firstNonEmptyString(response.Error, "request failed")), token)
	}
	return nil
}

func (b *telegramBridge) syncBotCommands(ctx context.Context, token string) {
	commands := []map[string]string{
		{"command": "start", "description": "راهنما و انتخاب نشست"},
		{"command": "chats", "description": "فهرست نشست‌های اخیر"},
		{"command": "newchat", "description": "ساخت و انتخاب نشست تازه"},
		{"command": "chat", "description": "انتخاب نشست با شناسه"},
		{"command": "current", "description": "نمایش نشست فعلی"},
		{"command": "settings", "description": "تنظیم مدل و سطح فکر"},
		{"command": "history", "description": "نمایش تاریخچهٔ اخیر"},
		{"command": "status", "description": "نمایش وضعیت پل"},
		{"command": "whoami", "description": "نمایش شناسه‌های تلگرام"},
		{"command": "help", "description": "نمایش راهنما"},
	}
	if err := b.callTelegram(ctx, token, "setMyCommands", map[string]any{"commands": commands}); err != nil {
		b.setError(err)
	}
}

func (b *telegramBridge) answerCallbackQuery(ctx context.Context, token string, callbackID string, text string) {
	if strings.TrimSpace(callbackID) == "" {
		return
	}
	if err := b.callTelegram(ctx, token, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
	}); err != nil {
		b.setError(err)
	}
}

func telegramMarkdownIsRTL(markdown string) bool {
	for _, char := range markdown {
		if unicode.Is(unicode.Arabic, char) || unicode.Is(unicode.Hebrew, char) {
			return true
		}
	}
	return false
}

func redactTelegramToken(err error, token string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if token = strings.TrimSpace(token); token != "" {
		message = strings.ReplaceAll(message, token, "[redacted-token]")
		message = strings.ReplaceAll(message, url.PathEscape(token), "[redacted-token]")
	}
	return fmt.Errorf("%s", message)
}

func splitTelegramText(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	chunks := []string{}
	for len([]rune(value)) > telegramMessageLimit {
		chunk, rest := splitTelegramMarkdownChunk(value)
		if chunk == "" || rest == "" {
			break
		}
		chunks = append(chunks, chunk)
		value = rest
	}
	return append(chunks, value)
}

type telegramMarkdownFence struct {
	opening string
	marker  string
}

// splitTelegramMarkdownChunk keeps rich Markdown valid at the Telegram message
// boundary. A code fence is closed before the split and reopened in the next
// message; a long GFM table gets its header and separator repeated. This keeps
// the custom Rich Markdown renderer from treating half a block as prose.
func splitTelegramMarkdownChunk(value string) (string, string) {
	runes := []rune(value)
	maxCut := min(telegramMessageLimit, len(runes))
	minCut := maxCut / 2
	for cut := maxCut; cut >= minCut; cut-- {
		if cut < len(runes) && !unicode.IsSpace(runes[cut-1]) {
			continue
		}
		if chunk, rest, ok := renderTelegramMarkdownSplit(runes, cut); ok {
			return chunk, rest
		}
	}

	// A single unbroken token is rare but must still respect Telegram's limit.
	// Reserve room for a closing fence when its content has no whitespace.
	cut := maxCut
	if fence, open := telegramMarkdownOpenFence(string(runes[:cut])); open {
		cut = min(cut, telegramMessageLimit-len([]rune(fence.marker))-1)
	}
	if cut <= 0 {
		cut = maxCut
	}
	chunk, rest, ok := renderTelegramMarkdownSplit(runes, cut)
	if ok {
		return chunk, rest
	}
	return strings.TrimSpace(string(runes[:cut])), strings.TrimSpace(string(runes[cut:]))
}

func renderTelegramMarkdownSplit(runes []rune, cut int) (string, string, bool) {
	if cut <= 0 || cut >= len(runes) {
		return "", "", false
	}
	chunk := strings.TrimRightFunc(string(runes[:cut]), unicode.IsSpace)
	rest := strings.TrimLeftFunc(string(runes[cut:]), unicode.IsSpace)
	if chunk == "" || rest == "" {
		return "", "", false
	}
	if fence, open := telegramMarkdownOpenFence(chunk); open {
		chunk += "\n" + fence.marker
		rest = fence.opening + "\n" + rest
	} else if header := telegramMarkdownTableHeader(chunk); header != "" {
		rest = header + "\n" + rest
	}
	if len([]rune(chunk)) > telegramMessageLimit {
		return "", "", false
	}
	return chunk, rest, true
}

func telegramMarkdownOpenFence(markdown string) (telegramMarkdownFence, bool) {
	var current telegramMarkdownFence
	for _, line := range strings.Split(markdown, "\n") {
		marker, opening, ok := telegramMarkdownFenceLine(line)
		if !ok {
			continue
		}
		if current.marker == "" {
			current = telegramMarkdownFence{opening: opening, marker: marker}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), current.marker) {
			current = telegramMarkdownFence{}
		}
	}
	return current, current.marker != ""
}

func telegramMarkdownFenceLine(line string) (marker string, opening string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	first := rune(trimmed[0])
	if first != '`' && first != '~' {
		return "", "", false
	}
	count := 0
	for _, char := range trimmed {
		if char != first {
			break
		}
		count++
	}
	if count < 3 {
		return "", "", false
	}
	return strings.Repeat(string(first), count), trimmed, true
}

func telegramMarkdownTableHeader(markdown string) string {
	lines := strings.Split(strings.TrimRightFunc(markdown, unicode.IsSpace), "\n")
	start := len(lines) - 1
	for start >= 0 && strings.Contains(lines[start], "|") {
		start--
	}
	table := lines[start+1:]
	if len(table) < 2 || !telegramMarkdownTableDivider(table[1]) {
		return ""
	}
	return strings.TrimSpace(table[0]) + "\n" + strings.TrimSpace(table[1])
}

func telegramMarkdownTableDivider(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.Count(trimmed, "|") < 2 || !strings.Contains(trimmed, "-") {
		return false
	}
	for _, char := range trimmed {
		if char == '|' || char == '-' || char == ':' || unicode.IsSpace(char) {
			continue
		}
		return false
	}
	return true
}

func sortedTelegramMappingIDs(mappings map[string]string) []string {
	ids := make([]string, 0, len(mappings))
	for id := range mappings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
