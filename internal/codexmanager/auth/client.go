package auth

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// NewHTTPClient builds a bounded client suitable for account maintenance.
// Callers pass an empty proxy for the normal direct path. Credentials are never
// embedded in errors returned from this helper.
func NewHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil || proxy.Scheme == "" || proxy.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}
