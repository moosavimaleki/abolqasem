package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodexManagerLoginAPIRejectsInvalidRequestsWithoutStartingProcess(t *testing.T) {
	previousDir := codexManagerStateDir
	stateDir := t.TempDir()
	codexManagerStateDir = func() string { return stateDir }
	t.Cleanup(func() { codexManagerStateDir = previousDir })

	recorder := httptest.NewRecorder()
	handleAPICodexManagerLogin(recorder, httptest.NewRequest(http.MethodGet, "/api/codex-manager/login", nil), "")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected method response: %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handleAPICodexManagerLoginStart(recorder, httptest.NewRequest(http.MethodPost, "/api/codex-manager/login", strings.NewReader(`{"name":"../../unsafe"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unsafe account name should be rejected: %d %s", recorder.Code, recorder.Body.String())
	}
}
