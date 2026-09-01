package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSessionClientListsAndRevokesExplicitNonCurrentDevice(t *testing.T) {
	revoked := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/auth/session":
			_, _ = writer.Write([]byte(`{"accessToken":"access"}`))
		case "/backend-api/accounts/check/v4-2023-04-27":
			_, _ = writer.Write([]byte(`{"account_ordering":["account"]}`))
		case "/backend-api/accounts/sessions":
			_, _ = writer.Write([]byte(`{"devices":[{"id":"current","is_current":true,"app_sessions":[{"client_name":"Codex"}]},{"id":"old","is_current":false}]}`))
		case "/backend-api/accounts/sessions/revoke":
			revoked = "old"
			_, _ = writer.Write([]byte(`{}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := SessionClient{BaseURL: server.URL, HTTPClient: server.Client()}
	devices, err := client.Devices(context.Background())
	if err != nil || len(devices) != 2 || !devices[0].HasCodex {
		t.Fatalf("devices=%#v err=%v", devices, err)
	}
	if err := client.Revoke(context.Background(), devices[0]); !errors.Is(err, ErrCurrentDevice) {
		t.Fatalf("current revoke err=%v", err)
	}
	if err := client.Revoke(context.Background(), devices[1]); err != nil || revoked != "old" {
		t.Fatalf("revoke err=%v revoked=%q", err, revoked)
	}
}

func TestSessionClientRejectsInvalidProxy(t *testing.T) {
	client := SessionClient{BaseURL: "https://chatgpt.com", ProxyURL: "://bad"}
	if _, err := client.httpClient(&url.URL{Scheme: "https", Host: "chatgpt.com"}); err == nil {
		t.Fatal("expected invalid proxy error")
	}
}

func TestValidCookieValueRejectsQuotedAuxiliaryCookies(t *testing.T) {
	for _, value := range []string{`{"state":true}`, "quoted\"value", "line\nvalue", "semi;colon"} {
		if validCookieValue(value) {
			t.Fatalf("expected cookie value %q to be rejected", value)
		}
	}
	for _, value := range []string{"session-token", "device_id=abc_123", "v1.r.p.xyz"} {
		if !validCookieValue(value) {
			t.Fatalf("expected cookie value %q to be accepted", value)
		}
	}
}

func TestSessionClientOmitsInvalidCookiesBeforeCookieJar(t *testing.T) {
	client := SessionClient{Cookies: []Cookie{
		{Name: "valid", Value: "session-token", Domain: "chatgpt.com", Path: "/"},
		{Name: "aux", Value: `{"state":true}`, Domain: "chatgpt.com", Path: "/"},
	}}
	jarClient, err := client.httpClient(&url.URL{Scheme: "https", Host: "chatgpt.com"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range jarClient.Jar.Cookies(req.URL) {
		if cookie.Name == "aux" {
			t.Fatal("invalid auxiliary cookie was retained")
		}
	}
}
