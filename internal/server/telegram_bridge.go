package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/workspace/agent"
	"abolqasem/internal/workspace/readmodels"
	"abolqasem/internal/workspace/transcript"

	"golang.org/x/net/proxy"
)

const telegramMessageLimit = 3500

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
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

type telegramAPIResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"description"`
}

type telegramBridge struct {
	mu          sync.Mutex
	cancel      context.CancelFunc
	active      bool
	lastError   string
	fingerprint string
	client      *http.Client
}

var workspaceTelegramBridge = &telegramBridge{client: telegramHTTPClient()}

func telegramHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	for _, raw := range []string{os.Getenv("ALL_PROXY"), os.Getenv("all_proxy")} {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || !strings.HasPrefix(strings.ToLower(parsed.Scheme), "socks") {
			continue
		}
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err == nil {
			transport.Proxy = nil
			transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		}
		break
	}
	return &http.Client{Transport: transport, Timeout: 40 * time.Second}
}

func (b *telegramBridge) Reload() {
	config, err := loadTelegramBridgeConfig()
	b.mu.Lock()
	defer b.mu.Unlock()
	if err != nil {
		b.lastError = err.Error()
		return
	}
	fingerprint := config.BotToken + "|" + strings.Join(config.AllowedUserIDs, ",")
	if config.BotToken == "" || len(config.AllowedUserIDs) == 0 {
		if b.cancel != nil {
			b.cancel()
		}
		b.cancel, b.active, b.fingerprint, b.lastError = nil, false, "", ""
		return
	}
	if b.active && fingerprint == b.fingerprint {
		return
	}
	if b.cancel != nil {
		b.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel, b.active, b.fingerprint, b.lastError = cancel, true, fingerprint, ""
	go b.poll(ctx, config)
}

func (b *telegramBridge) Status(config telegramBridgeConfig) map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	allowAll := false
	for _, value := range config.AllowedUserIDs {
		allowAll = allowAll || value == "*"
	}
	return map[string]any{
		"configured":    config.BotToken != "" && len(config.AllowedUserIDs) > 0,
		"active":        b.active,
		"mappedChats":   len(config.Mappings),
		"mappedThreads": len(config.Mappings),
		"allowedUsers":  len(config.AllowedUserIDs),
		"allowAllUsers": allowAll,
		"lastError":     b.lastError,
	}
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
			}
		}
	}
}

func (b *telegramBridge) getUpdates(ctx context.Context, token string, offset int64) ([]telegramUpdate, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=25&offset=%d", url.PathEscape(token), offset)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	var response telegramAPIResponse
	if err := b.do(request, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", firstNonEmptyString(response.Error, "request failed"))
	}
	var updates []telegramUpdate
	if err := json.Unmarshal(response.Result, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (b *telegramBridge) do(request *http.Request, target *telegramAPIResponse) error {
	response, err := b.client.Do(request)
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
		return
	}
	text := strings.TrimSpace(message.Text)
	if text == "" {
		return
	}
	command, argument := telegramCommand(text)
	switch command {
	case "start", "help":
		b.sendText(ctx, config.BotToken, chatID, "پل Telegram فعال است. /chat <chat-id> برای اتصال، /current، /status و /whoami در دسترس‌اند.")
		return
	case "whoami":
		b.sendText(ctx, config.BotToken, chatID, "userId: "+userID+"\nchatId: "+chatID)
		return
	case "status":
		status := b.Status(config)
		b.sendText(ctx, config.BotToken, chatID, fmt.Sprintf("active: %v\nmapped chats: %v\nlast error: %v", status["active"], status["mappedChats"], status["lastError"]))
		return
	case "current":
		if target := strings.TrimSpace(config.Mappings[chatID]); target != "" {
			b.sendText(ctx, config.BotToken, chatID, "متصل به: "+target)
		} else {
			b.sendText(ctx, config.BotToken, chatID, "هنوز به چتی متصل نیستید. /chat <chat-id>")
		}
		return
	case "chat":
		if _, _, err := workspaceChatProjectRequired(argument); err != nil {
			b.sendText(ctx, config.BotToken, chatID, "chat-id معتبر نیست: "+err.Error())
			return
		}
		config.Mappings[chatID] = argument
		if err := saveTelegramBridgeConfig(config); err != nil {
			b.sendText(ctx, config.BotToken, chatID, "ذخیرهٔ اتصال ناموفق بود: "+err.Error())
			return
		}
		b.sendText(ctx, config.BotToken, chatID, "متصل شد: "+argument)
		return
	case "":
		// A normal message is forwarded below.
	default:
		b.sendText(ctx, config.BotToken, chatID, "دستور ناشناخته است. /help")
		return
	}
	target := strings.TrimSpace(config.Mappings[chatID])
	if target == "" {
		b.sendText(ctx, config.BotToken, chatID, "ابتدا /chat <chat-id> را بفرستید.")
		return
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
	go b.forwardFinal(config.BotToken, chatID, target)
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
	parts := make([]string, 0, 4)
	for index := len(entries) - 1; index >= 0 && len(parts) < 4; index-- {
		kind, role, text := workspaceEntrySearchText(entries[index])
		if text == "" || role == "user" || kind == transcript.KindToolCall || kind == transcript.KindToolResult || kind == transcript.KindStatus {
			continue
		}
		parts = append(parts, text)
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (b *telegramBridge) sendText(ctx context.Context, token string, chatID string, text string) {
	for _, chunk := range splitTelegramText(text) {
		body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": chunk})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+url.PathEscape(token)+"/sendMessage", bytes.NewReader(body))
		if err != nil {
			b.setError(err)
			return
		}
		request.Header.Set("Content-Type", "application/json")
		var response telegramAPIResponse
		if err := b.do(request, &response); err != nil {
			b.setError(err)
			return
		}
		if !response.OK {
			b.setError(fmt.Errorf("telegram sendMessage: %s", firstNonEmptyString(response.Error, "request failed")))
		}
	}
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
