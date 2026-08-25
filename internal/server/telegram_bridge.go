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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"

	"golang.org/x/net/proxy"
)

const telegramMessageLimit = 3500

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
}

const telegramAPIBaseURL = "https://api.telegram.org"

var workspaceTelegramBridge = &telegramBridge{
	client:              &http.Client{Timeout: 40 * time.Second},
	apiBaseURL:          telegramAPIBaseURL,
	lastForwardedByChat: map[string]string{},
}

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
	fingerprint := config.BotToken + "|" + config.ProxyURL + "|" + strings.Join(config.AllowedUserIDs, ",")
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
		b.sendText(ctx, config.BotToken, chatID, telegramHelpMarkdown())
		b.sendChatPicker(ctx, config, chatID)
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
	case "chats", "threads":
		page, _ := strconv.Atoi(argument)
		b.sendChatPickerPage(ctx, config, chatID, page)
		return
	case "newchat", "newthread":
		target, err := createTelegramChat(&config, chatID)
		if err != nil {
			b.sendText(ctx, config.BotToken, chatID, "ساخت نشست تازه ناموفق بود: "+err.Error())
			return
		}
		b.sendText(ctx, config.BotToken, chatID, "نشست تازه ساخته و انتخاب شد: `"+target+"`\n\nپیام بعدی شما به همین نشست فرستاده می‌شود.")
		return
	case "current":
		if target := strings.TrimSpace(config.Mappings[chatID]); target != "" {
			b.sendText(ctx, config.BotToken, chatID, "متصل به: "+target)
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
		if err := saveTelegramBridgeConfig(config); err != nil {
			b.sendText(ctx, config.BotToken, chatID, "ذخیرهٔ اتصال ناموفق بود: "+err.Error())
			return
		}
		b.sendText(ctx, config.BotToken, chatID, "نشست انتخاب شد: `"+target+"`\n\nپیام بعدی شما به همین نشست فرستاده می‌شود.")
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
		b.sendText(ctx, config.BotToken, chatID, "این chat قفل است: "+err.Error())
		return
	}
	result, err := workspaceAgentCoordinator().Send(ctx, agent.SendCommand{ChatID: target, Content: text})
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
	if chatID == "" || !strings.HasPrefix(callback.Data, "chat:") {
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
	if err := saveTelegramBridgeConfig(config); err != nil {
		b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "ذخیره ناموفق بود")
		b.setError(err)
		return
	}
	b.answerCallbackQuery(ctx, config.BotToken, callback.ID, "نشست انتخاب شد")
	b.sendText(ctx, config.BotToken, chatID, "نشست انتخاب شد: `"+target+"`")
	b.sendText(ctx, config.BotToken, chatID, telegramChatHistoryMarkdown(target))
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
	if err := saveTelegramBridgeConfig(*config); err != nil {
		b.setError(err)
	}
}

type telegramChatChoice struct {
	ChatID       string
	ChatTitle    string
	ProjectTitle string
	UpdatedAt    int64
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
	b.sendChatPickerPage(ctx, config, telegramChatID, 1)
}

func (b *telegramBridge) sendChatPickerPage(ctx context.Context, config telegramBridgeConfig, telegramChatID string, page int) {
	if page < 1 {
		page = 1
	}
	markdown := telegramChatListMarkdown(config.Mappings[telegramChatID], page)
	markup := telegramChatPickerMarkup(config.Mappings[telegramChatID], page)
	b.sendTextWithMarkup(ctx, config.BotToken, telegramChatID, markdown, markup)
}

func telegramChatPickerMarkup(currentChatID string, page int) any {
	const pageSize = 10
	choices := telegramChatChoices(0)
	if page < 1 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start >= len(choices) {
		return nil
	}
	end := min(start+pageSize, len(choices))
	rows := make([][]map[string]string, 0, end-start)
	for _, choice := range choices[start:end] {
		label := choice.ProjectTitle + " / " + choice.ChatTitle
		if choice.ChatID == currentChatID {
			label = "✓ " + label
		}
		rows = append(rows, []map[string]string{{
			"text":          truncateTelegramButtonLabel(label, 60),
			"callback_data": "chat:" + choice.ChatID,
		}})
	}
	return map[string]any{"inline_keyboard": rows}
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
		"- `/chats` — انتخاب از نشست‌های اخیر\n" +
		"- `/newchat` — ساخت و انتخاب نشست تازه\n" +
		"- `/chat <id>` — انتخاب مستقیم یک نشست\n" +
		"- `/current` — نمایش نشست فعلی\n" +
		"- `/history` — نمایش تاریخچهٔ اخیر\n" +
		"- `/status` — وضعیت پل و اتصال\n" +
		"- `/whoami` — شناسه‌های Telegram\n" +
		"- `/help` — همین راهنما\n\n" +
		"فرمان‌های سازگار با Codex Mobile یعنی `/threads`، `/newthread` و `/thread <id>` نیز پشتیبانی می‌شوند."
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
	if err := saveTelegramBridgeConfig(*config); err != nil {
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
				b.sendText(context.Background(), token, telegramChatID, text)
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
			b.sendText(context.Background(), config.BotToken, telegramChatID, text)
		}
	}()
}

func (b *telegramBridge) sendText(ctx context.Context, token string, chatID string, text string) {
	b.sendTextWithMarkup(ctx, token, chatID, text, nil)
}

func (b *telegramBridge) sendTextWithMarkup(ctx context.Context, token string, chatID string, text string, replyMarkup any) {
	for _, chunk := range splitTelegramText(text) {
		payload := map[string]any{
			"chat_id": chatID,
			"rich_message": map[string]any{
				"markdown": chunk,
				"is_rtl":   telegramMarkdownIsRTL(chunk),
			},
		}
		if replyMarkup != nil {
			payload["reply_markup"] = replyMarkup
			replyMarkup = nil
		}
		if err := b.callTelegram(ctx, token, "sendRichMessage", payload); err != nil {
			b.setError(err)
			return
		}
	}
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
		runes := []rune(value)
		cut := telegramMessageLimit
		for index := cut; index > telegramMessageLimit/2; index-- {
			if runes[index] == '\n' || runes[index] == ' ' {
				cut = index
				break
			}
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[:cut])))
		value = strings.TrimSpace(string(runes[cut:]))
	}
	return append(chunks, value)
}

func sortedTelegramMappingIDs(mappings map[string]string) []string {
	ids := make([]string, 0, len(mappings))
	for id := range mappings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
