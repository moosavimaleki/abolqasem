package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/login"
)

type codexManagerLoginFlow struct {
	cancel context.CancelFunc
	done   chan codexManagerLoginResult
}

type codexManagerLoginResult struct {
	result login.Result
	err    error
}

var codexManagerLoginFlows = struct {
	sync.Mutex
	byID map[string]codexManagerLoginFlow
}{byID: map[string]codexManagerLoginFlow{}}

func handleAPICodexManagerLogin(w http.ResponseWriter, r *http.Request, loginID string) {
	loginID = strings.TrimSpace(loginID)
	if loginID == "" {
		handleAPICodexManagerLoginStart(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		handleAPICodexManagerLoginStatus(w, loginID)
	case http.MethodDelete:
		if !cancelCodexManagerLogin(loginID) {
			http.Error(w, "device login was not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"cancelled": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPICodexManagerLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Name          string `json:"name"`
		Replace       bool   `json:"replace"`
		ExpectedEmail string `json:"expectedEmail"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&request); err != nil {
		http.Error(w, "Invalid device login request", http.StatusBadRequest)
		return
	}
	if _, err := (account.Repository{Paths: codexManagerPaths()}).Paths.Account(request.Name); err != nil {
		http.Error(w, "Invalid account name", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	codeCh := make(chan login.Code, 1)
	done := make(chan codexManagerLoginResult, 1)
	tempRoot := filepath.Join(codexManagerPaths().Home, "login")
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		http.Error(w, "Could not prepare the private device-login directory", http.StatusInternalServerError)
		cancel()
		return
	}
	client := &login.AppServerClient{TempRoot: tempRoot}
	service := login.Service{Accounts: account.Repository{Paths: codexManagerPaths()}, Client: client, Timeout: 15 * time.Minute}
	go func() {
		result, err := service.Login(ctx, request.Name, request.Replace, request.ExpectedEmail, login.Callbacks{OnCode: func(code login.Code) { codeCh <- code }})
		done <- codexManagerLoginResult{result: result, err: err}
	}()
	select {
	case code := <-codeCh:
		codexManagerLoginFlows.Lock()
		codexManagerLoginFlows.byID[code.LoginID] = codexManagerLoginFlow{cancel: cancel, done: done}
		codexManagerLoginFlows.Unlock()
		writeJSON(w, map[string]any{"loginId": code.LoginID, "verificationUrl": code.VerificationURL, "userCode": code.UserCode, "expiresInSeconds": 900})
	case <-time.After(12 * time.Second):
		cancel()
		http.Error(w, "Codex did not return a device code in time; verify the Codex executable and retry.", http.StatusGatewayTimeout)
	}
}

func handleAPICodexManagerLoginStatus(w http.ResponseWriter, loginID string) {
	codexManagerLoginFlows.Lock()
	flow, exists := codexManagerLoginFlows.byID[loginID]
	codexManagerLoginFlows.Unlock()
	if !exists {
		http.Error(w, "device login was not found", http.StatusNotFound)
		return
	}
	select {
	case result := <-flow.done:
		codexManagerLoginFlows.Lock()
		delete(codexManagerLoginFlows.byID, loginID)
		codexManagerLoginFlows.Unlock()
		if result.err != nil {
			writeJSON(w, map[string]any{"status": "failed", "error": safeCodexManagerLoginError(result.err)})
			return
		}
		writeJSON(w, map[string]any{"status": "completed", "account": result.result})
	default:
		writeJSON(w, map[string]any{"status": "pending"})
	}
}

func cancelCodexManagerLogin(loginID string) bool {
	codexManagerLoginFlows.Lock()
	flow, exists := codexManagerLoginFlows.byID[loginID]
	if exists {
		delete(codexManagerLoginFlows.byID, loginID)
	}
	codexManagerLoginFlows.Unlock()
	if exists {
		flow.cancel()
	}
	return exists
}

func safeCodexManagerLoginError(err error) string {
	if errors.Is(err, login.ErrLoginTimeout) {
		return "Device login timed out. Start a new login attempt."
	}
	if errors.Is(err, login.ErrLoginCancelled) {
		return "Device login was cancelled."
	}
	return "Device login failed. Retry after checking the Codex executable and sign-in page."
}
