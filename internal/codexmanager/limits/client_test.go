package limits

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetchesAndClassifiesResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))
	}))
	defer server.Close()
	client := Client{URL: server.URL, Now: func() time.Time { return time.Unix(1, 0).UTC() }}
	snapshot, err := client.Fetch(context.Background(), "account", map[string]any{"tokens": map[string]any{"access_token": "access"}})
	if err != nil || snapshot.Account != "account" || len(snapshot.Limits) != 1 || snapshot.Limits[0].Windows[0].RemainingPercent != 75 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	_, err = client.Fetch(context.Background(), "account", map[string]any{})
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) || fetchErr.Kind != ErrorAuth {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestClientFetchUsesConfiguredProxy(t *testing.T) {
	requests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Host != "quota.invalid" || request.Header.Get("Authorization") != "Bearer access" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":25}}}`))
	}))
	defer proxy.Close()

	client := Client{URL: "http://quota.invalid/usage", ProxyURL: proxy.URL}
	snapshot, err := client.Fetch(context.Background(), "account", map[string]any{"tokens": map[string]any{"access_token": "access"}})
	if err != nil || requests != 1 || len(snapshot.Limits) != 1 {
		t.Fatalf("snapshot=%#v requests=%d err=%v", snapshot, requests, err)
	}
}
