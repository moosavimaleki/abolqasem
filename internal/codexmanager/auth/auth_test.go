package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func token(claims map[string]any) string {
	body, _ := json.Marshal(claims)
	return "x." + base64.RawURLEncoding.EncodeToString(body) + ".y"
}
func fixture(exp float64) map[string]any {
	return map[string]any{"tokens": map[string]any{"access_token": token(map[string]any{"exp": exp}), "refresh_token": "redacted", "id_token": token(map[string]any{"email": "a@example.invalid", "sub": "one", "chatgpt_account_id": "acct"})}}
}
func TestClaimsIdentityAndRefreshDecision(t *testing.T) {
	raw := fixture(float64(time.Now().Add(24 * time.Hour).Unix()))
	if Metadata(raw)["email"] != "a@example.invalid" {
		t.Fatal("metadata")
	}
	if yes, _ := ShouldRefresh(raw, time.Now()); yes {
		t.Fatal("unexpected refresh")
	}
}

func TestShouldRefreshFallsBackToLastRefreshWhenExpiryIsUnreadable(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	recent := map[string]any{"last_refresh": now.Add(-71 * time.Hour).Format(time.RFC3339), "tokens": map[string]any{"refresh_token": "redacted"}}
	if yes, reason := ShouldRefresh(recent, now); yes || reason != "last refresh is still recent" {
		t.Fatalf("recent unreadable token refresh = %t, %q", yes, reason)
	}
	old := map[string]any{"last_refresh": now.Add(-72 * time.Hour).Format(time.RFC3339), "tokens": map[string]any{"refresh_token": "redacted"}}
	if yes, _ := ShouldRefresh(old, now); !yes {
		t.Fatal("72 hour old fallback token must refresh")
	}
}

func TestShouldPromoteLivePrefersRotatedRefreshToken(t *testing.T) {
	stored := fixture(float64(time.Now().Add(time.Hour).Unix()))
	live := fixture(float64(time.Now().Add(time.Hour).Unix()))
	live["tokens"].(map[string]any)["refresh_token"] = "rotated"
	if yes, reason := ShouldPromoteLive(stored, live); !yes || reason != "live auth has a newer refresh token" {
		t.Fatalf("promotion = %t, %q", yes, reason)
	}
}

func TestRefresherUsesConfiguredProxy(t *testing.T) {
	requests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Host != "auth.invalid" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		identity := token(map[string]any{"email": "a@example.invalid", "sub": "one", "chatgpt_account_id": "acct"})
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"access_token":  token(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())}),
			"id_token":      identity,
			"refresh_token": "rotated",
		})
	}))
	defer proxy.Close()

	_, err := (Refresher{URL: "http://auth.invalid/oauth/token", ProxyURL: proxy.URL}).Refresh(context.Background(), fixture(float64(time.Now().Add(time.Hour).Unix())))
	if err != nil || requests != 1 {
		t.Fatalf("proxy refresh requests=%d err=%v", requests, err)
	}
}

func TestRefreshRejectsChangedIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())}), "id_token": token(map[string]any{"email": "other@example.invalid", "sub": "two", "chatgpt_account_id": "other"})})
	}))
	defer server.Close()
	_, err := (Refresher{URL: server.URL}).Refresh(context.Background(), fixture(0))
	if err == nil {
		t.Fatal("expected identity rejection")
	}
}
