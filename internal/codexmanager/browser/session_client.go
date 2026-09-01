package browser

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultChatGPTURL = "https://chatgpt.com"

var (
	ErrNotSignedIn   = errors.New("not signed in to ChatGPT")
	ErrCurrentDevice = errors.New("refusing to revoke current device")
)

type SessionClient struct {
	BaseURL    string
	ProxyURL   string
	HTTPClient *http.Client
	Cookies    []Cookie
}

type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform,omitempty"`
	Current  bool   `json:"current"`
	LastSeen string `json:"lastSeen,omitempty"`
	HasCodex bool   `json:"hasCodex"`
}

// AccountEmail confirms the account selected by the currently signed-in
// Chrome profile. It returns only an email address; tokens and cookie values
// remain inside this short-lived client.
func (c SessionClient) AccountEmail(ctx context.Context) (string, error) {
	token, accountID, err := c.access(ctx)
	if err != nil {
		return "", err
	}
	payload, err := c.request(ctx, http.MethodGet, "/backend-api/me", token, accountID, nil)
	if err != nil {
		return "", err
	}
	email := strings.ToLower(strings.TrimSpace(firstText(payload, "email")))
	if email == "" {
		return "", ErrNotSignedIn
	}
	return email, nil
}

func (c SessionClient) Devices(ctx context.Context) ([]Device, error) {
	token, accountID, err := c.access(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := c.request(ctx, http.MethodGet, "/backend-api/accounts/sessions", token, accountID, nil)
	if err != nil {
		return nil, err
	}
	items, _ := payload["devices"].([]any)
	devices := make([]Device, 0, len(items))
	for _, item := range items {
		device, _ := item.(map[string]any)
		if device == nil {
			continue
		}
		id := firstText(device, "id", "session_id")
		if id == "" {
			continue
		}
		applications, _ := device["app_sessions"].([]any)
		hasCodex := false
		for _, app := range applications {
			entry, _ := app.(map[string]any)
			if strings.EqualFold(firstText(entry, "client_name"), "Codex") {
				hasCodex = true
			}
		}
		current, _ := device["is_current"].(bool)
		if !current {
			current, _ = device["is_current_device"].(bool)
		}
		devices = append(devices, Device{ID: id, Name: firstText(device, "device_name", "name"), Platform: strings.ToLower(firstText(device, "platform", "os")), Current: current, HasCodex: hasCodex, LastSeen: firstText(device, "last_signed_in_timestamp_second")})
	}
	return devices, nil
}

func (c SessionClient) Revoke(ctx context.Context, target Device) error {
	if target.ID == "" {
		return errors.New("revoke target is required")
	}
	if target.Current {
		return ErrCurrentDevice
	}
	token, accountID, err := c.access(ctx)
	if err != nil {
		return err
	}
	_, err = c.request(ctx, http.MethodPost, "/backend-api/accounts/sessions/revoke", token, accountID, map[string]string{"session_id": target.ID})
	return err
}

func (c SessionClient) access(ctx context.Context) (string, string, error) {
	session, err := c.request(ctx, http.MethodGet, "/api/auth/session", "", "", nil)
	if err != nil {
		return "", "", err
	}
	token := firstText(session, "accessToken", "access_token")
	if token == "" {
		return "", "", ErrNotSignedIn
	}
	accounts, err := c.request(ctx, http.MethodGet, "/backend-api/accounts/check/v4-2023-04-27", "", "", nil)
	if err != nil {
		return "", "", err
	}
	ordering, _ := accounts["account_ordering"].([]any)
	if len(ordering) == 0 {
		return "", "", ErrNotSignedIn
	}
	accountID, _ := ordering[0].(string)
	if accountID == "" {
		return "", "", ErrNotSignedIn
	}
	return token, accountID, nil
}

func (c SessionClient) request(ctx context.Context, method, path, token, accountID string, body any) (map[string]any, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = defaultChatGPTURL
	}
	parsed, err := url.Parse(base + path)
	if err != nil || parsed.Scheme != "https" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		return nil, errors.New("invalid ChatGPT endpoint")
	}
	var reader *bytes.Reader
	if body != nil {
		data, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, marshalErr
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Oai-Language", "en-US")
	req.Header.Set("Oai-Session-Id", browserSessionID())
	req.Header.Set("Origin", base)
	req.Header.Set("Referer", base+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36")
	req.Header.Set("X-OpenAI-Target-Path", path)
	req.Header.Set("X-OpenAI-Target-Route", path)
	if deviceID := c.cookieValue("oai-did"); validHeaderValue(deviceID) {
		req.Header.Set("Oai-Device-Id", deviceID)
	}
	if integrityState := c.cookieValue("__Secure-oai-is"); integrityState != "" {
		if parts := strings.Split(integrityState, "."); len(parts) >= 3 {
			value := "v1.r.p." + parts[2]
			if validHeaderValue(value) {
				req.Header.Set("X-Oai-Is-Client-Observation", value)
			}
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	client, err := c.httpClient(parsed)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT session API request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrNotSignedIn
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ChatGPT sessions API returned HTTP %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil || payload == nil {
		return nil, errors.New("ChatGPT sessions API returned invalid JSON")
	}
	return payload, nil
}

func (c SessionClient) cookieValue(name string) string {
	for _, cookie := range c.Cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func validHeaderValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, runeValue := range value {
		if runeValue < 0x20 || runeValue == 0x7f || runeValue > 0x7e {
			return false
		}
	}
	return true
}

// validCookieValue applies the stricter RFC cookie-octet rules used by
// net/http. Chromium occasionally stores auxiliary ChatGPT cookies as quoted
// JSON values; passing those values to cookiejar causes net/http to log
// "invalid byte '\"' in Cookie.Value" for every request and can stall the
// account/session checks while the jar repeatedly drops them. Invalid or
// non-authentication cookies are safe to omit; the session and device cookies
// used by this client are plain ASCII token values.
func validCookieValue(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, runeValue := range value {
		if runeValue < 0x21 || runeValue > 0x7e || runeValue == '"' || runeValue == ',' || runeValue == ';' || runeValue == '\\' {
			return false
		}
	}
	return true
}

func browserSessionID() string {
	var value [16]byte
	if _, err := cryptorand.Read(value[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func (c SessionClient) httpClient(base *url.URL) (*http.Client, error) {
	if c.HTTPClient != nil {
		return c.HTTPClient, nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	httpCookies := make([]*http.Cookie, 0, len(c.Cookies))
	for _, cookie := range c.Cookies {
		if !validCookieValue(cookie.Value) {
			continue
		}
		httpCookies = append(httpCookies, &http.Cookie{Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path, Secure: cookie.Secure})
	}
	jar.SetCookies(base, httpCookies)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if c.ProxyURL != "" {
		proxy, err := url.Parse(c.ProxyURL)
		if err != nil || proxy.Scheme == "" || proxy.Host == "" {
			return nil, errors.New("invalid proxy URL")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &http.Client{Timeout: 20 * time.Second, Jar: jar, Transport: transport}, nil
}

func firstText(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
