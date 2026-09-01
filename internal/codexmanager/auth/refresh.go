package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const RefreshURL = "https://auth.openai.com/oauth/token"
const ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

type Refresher struct {
	URL      string
	ProxyURL string
	Client   *http.Client
	Now      func() time.Time
}

func (r Refresher) Refresh(ctx context.Context, original map[string]any) (map[string]any, error) {
	tokens, _ := original["tokens"].(map[string]any)
	refresh, _ := tokens["refresh_token"].(string)
	if refresh == "" {
		return nil, errors.New("missing refresh token")
	}
	body, err := json.Marshal(map[string]string{"client_id": ClientID, "grant_type": "refresh_token", "refresh_token": refresh})
	if err != nil {
		return nil, err
	}
	url := r.URL
	if url == "" {
		url = RefreshURL
	}
	client := r.Client
	if client == nil {
		client, err = NewHTTPClient(r.ProxyURL, 30*time.Second)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("refresh failed: HTTP %s", resp.Status)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	next := clone(original)
	nextTokens := clone(tokens)
	for _, key := range []string{"id_token", "access_token", "refresh_token"} {
		if value, ok := payload[key]; ok {
			nextTokens[key] = value
		}
	}
	next["tokens"] = nextTokens
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	next["last_refresh"] = now().UTC().Format(time.RFC3339)
	if same, reason := SameIdentity(original, next); !same {
		return nil, errors.New(reason)
	}
	return next, nil
}
func clone(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
