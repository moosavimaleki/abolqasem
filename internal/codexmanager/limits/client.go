package limits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"abolqasem/internal/codexmanager/auth"
)

const DefaultUsageURL = "https://chatgpt.com/backend-api/wham/usage"

type ErrorKind string

const (
	ErrorAuth    ErrorKind = "auth"
	ErrorNetwork ErrorKind = "network"
	ErrorPayload ErrorKind = "payload"
)

type FetchError struct {
	Kind       ErrorKind
	StatusCode int
	Err        error
}

func (e *FetchError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("limits fetch %s error (HTTP %d)", e.Kind, e.StatusCode)
	}
	return fmt.Sprintf("limits fetch %s error", e.Kind)
}

func (e *FetchError) Unwrap() error { return e.Err }

type Client struct {
	URL        string
	ProxyURL   string
	HTTPClient *http.Client
	Now        func() time.Time
}

func (c Client) Fetch(ctx context.Context, account string, credentials map[string]any) (Snapshot, error) {
	tokens, _ := credentials["tokens"].(map[string]any)
	access, _ := tokens["access_token"].(string)
	if access == "" {
		return Snapshot{Account: account}, &FetchError{Kind: ErrorAuth, Err: errors.New("missing access token")}
	}
	url := c.URL
	if url == "" {
		url = DefaultUsageURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{Account: account}, &FetchError{Kind: ErrorNetwork, Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("User-Agent", "codex-cli")
	req.Header.Set("Accept", "application/json")
	if accountID := auth.Metadata(credentials)["account_id"]; accountID != "" {
		req.Header.Set("ChatGPT-Account-ID", accountID)
	}
	httpClient := c.HTTPClient
	if c.ProxyURL != "" {
		proxyClient, proxyErr := auth.NewHTTPClient(c.ProxyURL, 30*time.Second)
		if proxyErr != nil {
			return Snapshot{Account: account}, &FetchError{Kind: ErrorNetwork, Err: proxyErr}
		}
		httpClient = proxyClient
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{Account: account}, &FetchError{Kind: ErrorNetwork, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		kind := ErrorNetwork
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			kind = ErrorAuth
		}
		return Snapshot{Account: account}, &FetchError{Kind: kind, StatusCode: resp.StatusCode}
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return Snapshot{Account: account}, &FetchError{Kind: ErrorPayload, Err: err}
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	snapshot := Normalize(payload, now())
	snapshot.Account = account
	return snapshot, nil
}
