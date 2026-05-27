package netproxy

import (
	"ai-agent-manager/internal/state"
	"net/http"
	"net/url"
	"os"
	"time"

	"golang.org/x/net/http/httpproxy"
)

func HTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = ProxyFromSettings
	transport.TLSHandshakeTimeout = timeout
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func ProxyFromSettings(req *http.Request) (*url.URL, error) {
	proxy := currentProviderProxySettings()
	if proxy.Mode == state.ProviderProxyModeCustom && proxy.HTTPProxy != "" {
		config := httpproxy.Config{
			HTTPProxy:  proxy.HTTPProxy,
			HTTPSProxy: proxy.HTTPProxy,
			NoProxy:    proxy.NoProxy,
		}
		proxyFunc := config.ProxyFunc()
		return proxyFunc(req.URL)
	}
	return http.ProxyFromEnvironment(req)
}

func CommandEnv() []string {
	proxy := currentProviderProxySettings()
	if proxy.Mode != state.ProviderProxyModeCustom || proxy.HTTPProxy == "" {
		return os.Environ()
	}
	return state.ApplyProviderProxyEnv(os.Environ(), state.AppSettings{ProviderProxy: proxy})
}

func currentProviderProxySettings() state.ProviderProxySettings {
	settings, err := state.LoadSettings()
	if err != nil {
		settings = state.DefaultAppSettings()
	}
	return state.NormalizeSettings(settings).ProviderProxy
}
