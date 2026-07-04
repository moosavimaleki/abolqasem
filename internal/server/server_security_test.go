package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalServerAddressBindsLoopbackOnly(t *testing.T) {
	if got := localServerAddress(9092); got != "127.0.0.1:9092" {
		t.Fatalf("expected loopback server address, got %q", got)
	}
}

func TestWorkspaceWSOriginAllowedOnlyForLoopbackHTTP(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		allowed bool
	}{
		{name: "empty non-browser origin", origin: "", allowed: true},
		{name: "localhost", origin: "http://localhost:9092", allowed: true},
		{name: "loopback", origin: "http://127.0.0.1:9092", allowed: true},
		{name: "remote host", origin: "http://example.com:9092", allowed: false},
		{name: "localhost subdomain", origin: "http://localhost.example.com:9092", allowed: false},
		{name: "credentials", origin: "http://localhost:9092@evil.example", allowed: false},
		{name: "https remote exposure", origin: "https://localhost:9092", allowed: false},
		{name: "pathful origin", origin: "http://localhost:9092/app", allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isAllowedWorkspaceWSOrigin(test.origin); got != test.allowed {
				t.Fatalf("isAllowedWorkspaceWSOrigin(%q) = %v, expected %v", test.origin, got, test.allowed)
			}
		})
	}
}

func TestRoutesDoNotEmitCORSForForeignOrigin(t *testing.T) {
	mux := http.NewServeMux()
	setupRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	request.Header.Set("Origin", "http://example.com")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected CORS to stay closed, got Access-Control-Allow-Origin %q", got)
	}
}
