package sidecar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWaitReadyRequiresVersionedLoopbackHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"status":"ok","version":"0.1.0","configuredAccounts":2,"loopback":true}`))
	}))
	defer server.Close()
	listen := strings.TrimPrefix(server.URL, "http://")
	supervisor := Supervisor{HTTPClient: server.Client()}
	health, err := supervisor.waitReady(context.Background(), listen)
	if err != nil || !health.OK || health.Version != "0.1.0" || health.ConfiguredAccounts != 2 {
		t.Fatalf("health=%#v err=%v", health, err)
	}
}

func TestAcquireLockAllowsOnlyOneSupervisorPerStateDirectory(t *testing.T) {
	home := t.TempDir()
	first, err := acquireLock(home)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unlock()
	if _, err := acquireLock(home); err == nil {
		t.Fatal("expected an already-owned lock error")
	}
}

func TestResolveExecutableUsesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway")
	if err := os.WriteFile(path, []byte(""), 0o700); err != nil {
		t.Fatal(err)
	}
	actual, err := resolveExecutable(path)
	if err != nil || actual != path {
		t.Fatalf("executable=%q err=%v", actual, err)
	}
}

func TestResolveExecutableReportsMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := resolveExecutable("")
	if err == nil || err.Error() != "codex-manager sidecar binary was not found" {
		t.Fatal("expected missing executable error")
	}
}

func TestValidateConfigRejectsNonLoopbackAndSecrets(t *testing.T) {
	config := Config{ListenAddress: "0.0.0.0:8787", ManagerHome: "/tmp/manager", UpstreamBase: "https://example.com", APIKeyEnv: "KEY"}
	if err := validateConfig(config, "secret"); err == nil {
		t.Fatal("expected loopback rejection")
	}
	config.ListenAddress = "127.0.0.1:8787"
	if err := validateConfig(config, ""); err == nil {
		t.Fatal("expected missing key rejection")
	}
}
