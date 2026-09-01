package auth

import (
	"testing"
	"time"
)

func TestNewHTTPClientValidatesProxyAndTimeout(t *testing.T) {
	if _, err := NewHTTPClient("://bad", time.Second); err == nil {
		t.Fatal("expected invalid proxy error")
	}
	client, err := NewHTTPClient("http://127.0.0.1:8080", 0)
	if err != nil || client.Timeout != 30*time.Second {
		t.Fatalf("client=%#v err=%v", client, err)
	}
}
