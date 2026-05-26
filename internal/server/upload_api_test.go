package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadServiceStoresMetadataAndServesContent(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "hello.txt")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := io.WriteString(part, "hello upload"); err != nil {
		t.Fatalf("write multipart failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/uploads", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Attachments []uploadAttachment `json:"attachments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(payload.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %#v", payload.Attachments)
	}
	attachment := payload.Attachments[0]
	if attachment.Kind != "file" || attachment.DisplayName != "hello.txt" || attachment.Size != int64(len("hello upload")) {
		t.Fatalf("unexpected attachment: %#v", attachment)
	}
	if !strings.HasPrefix(attachment.AbsolutePath, filepath.Join(os.Getenv("XDG_CACHE_HOME"), "abolqasem", "uploads", "project-1")) {
		t.Fatalf("attachment escaped cache: %#v", attachment.AbsolutePath)
	}
	if _, err := os.Stat(attachment.AbsolutePath + ".json"); err != nil {
		t.Fatalf("expected metadata sidecar: %v", err)
	}

	contentRequest := httptest.NewRequest(http.MethodGet, attachment.ContentURL, nil)
	contentResponse := httptest.NewRecorder()
	handleAPIProjects(contentResponse, contentRequest)
	if contentResponse.Code != http.StatusOK {
		t.Fatalf("expected content 200, got %d", contentResponse.Code)
	}
	if strings.TrimSpace(contentResponse.Body.String()) != "hello upload" {
		t.Fatalf("unexpected content: %q", contentResponse.Body.String())
	}
}

func TestUploadServiceRejectsOversizedRequest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	body := strings.NewReader(strings.Repeat("x", maxUploadBytes+1))
	request := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/uploads", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	response := httptest.NewRecorder()

	handleAPIProjects(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestUploadDeleteRemovesFileAndMetadata(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	attachment, err := saveUploadedFile("project-1", "delete.txt", "text/plain", int64(len("delete")), strings.NewReader("delete"))
	if err != nil {
		t.Fatalf("saveUploadedFile failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodDelete, strings.TrimSuffix(attachment.ContentURL, "/content"), nil)
	response := httptest.NewRecorder()
	handleAPIProjects(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", response.Code)
	}
	if _, err := os.Stat(attachment.AbsolutePath); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, err=%v", err)
	}
	if _, err := os.Stat(attachment.AbsolutePath + ".json"); !os.IsNotExist(err) {
		t.Fatalf("expected metadata removed, err=%v", err)
	}
}
