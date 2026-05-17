package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-agent-manager/internal/state"
	"ai-agent-manager/internal/workspace/legacyimport"
	"ai-agent-manager/internal/workspace/readmodels"
)

func createStandaloneViewerDist(t *testing.T) string {
	t.Helper()
	viewerDistDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(viewerDistDir, "assets"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(viewerDistDir, "index.html"), []byte("<!doctype html><html><body><div id=\"root\"></div></body></html>\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(viewerDistDir, "assets", "viewer.js"), []byte("console.log('viewer')\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	return viewerDistDir
}

func createStandaloneMessages(attachmentAbsolutePath string) []readmodels.TranscriptEntry {
	return []readmodels.TranscriptEntry{
		{
			"_id":       "user-1",
			"createdAt": float64(time.Now().UnixMilli()),
			"kind":      "user_prompt",
			"messageId": "message-1",
			"content":   "Please review this attachment.",
			"attachments": []any{
				map[string]any{
					"id":           "attachment-1",
					"kind":         "image",
					"displayName":  "mock.png",
					"absolutePath": attachmentAbsolutePath,
					"relativePath": "./.abolqasem/uploads/mock.png",
					"contentUrl":   "/api/projects/project-1/uploads/mock.png/content",
					"mimeType":     "image/png",
					"size":         float64(4),
				},
			},
		},
		{
			"_id":       "assistant-1",
			"createdAt": float64(time.Now().UnixMilli()),
			"kind":      "assistant_text",
			"messageId": "message-2",
			"text":      "Looks good in " + attachmentAbsolutePath + ".",
		},
	}
}

func TestWriteStandaloneTranscriptExportMetadataMode(t *testing.T) {
	viewerDistDir := createStandaloneViewerDist(t)
	projectDir := t.TempDir()
	uploadsDir := filepath.Join(projectDir, ".abolqasem", "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	attachmentPath := filepath.Join(uploadsDir, "mock.png")
	if err := os.WriteFile(attachmentPath, []byte("mock"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	uploadedRequests := map[string]string{}

	result, err := writeStandaloneTranscriptExport(standaloneExportArgs{
		ChatID:         "chat-1",
		Title:          "Release Review",
		LocalPath:      projectDir,
		Theme:          "dark",
		AttachmentMode: "metadata",
		Messages:       createStandaloneMessages(attachmentPath),
	}, standaloneExportDeps{
		UploadFile: func(targetURL string, body []byte, contentType string, cacheControl string) error {
			uploadedRequests[targetURL] = string(body)
			if contentType != "application/json; charset=utf-8" {
				t.Fatalf("unexpected content type: %q", contentType)
			}
			return nil
		},
		SharePublicBaseURL: "https://share.example.com",
		ShareSlugSuffix:    "ax71234ka",
		ShareUploadBaseURL: "https://upload.example.com/api/share",
		ViewerDistDir:      viewerDistDir,
		Now:                time.Date(2026, 4, 23, 12, 34, 56, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("writeStandaloneTranscriptExport returned error: %v", err)
	}
	success, ok := result.(standaloneExportSuccess)
	if !ok || !success.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if _, err := os.Stat(success.IndexHTMLPath); err != nil {
		t.Fatalf("expected index html: %v", err)
	}
	if _, err := os.Stat(filepath.Join(success.OutputDir, "assets", "viewer.js")); err != nil {
		t.Fatalf("expected viewer asset: %v", err)
	}
	if success.TotalAttachmentCount != 1 || success.BundledAttachmentCount != 0 || success.UploadedFileCount != 1 {
		t.Fatalf("unexpected attachment counts: %#v", success)
	}
	if success.ShareSlug != "release-review-ax71234ka" || success.ShareURL != "https://share.example.com/release-review-ax71234ka" {
		t.Fatalf("unexpected share fields: %#v", success)
	}

	bundle := readStandaloneBundle(t, success.TranscriptJSONPath)
	if bundle["title"] != "Release Review" || bundle["theme"] != "dark" || bundle["attachmentMode"] != "metadata" || bundle["localPath"] != standaloneShareWorkspacePath {
		t.Fatalf("unexpected bundle metadata: %#v", bundle)
	}
	attachment := firstStandaloneAttachment(t, bundle)
	if attachment["contentUrl"] != "" || attachment["absolutePath"] != "" || attachment["relativePath"] != "" {
		t.Fatalf("expected metadata-only attachment, got %#v", attachment)
	}
	bundleJSON, _ := json.Marshal(bundle)
	if strings.Contains(string(bundleJSON), projectDir) {
		t.Fatalf("expected local project path to be rewritten: %s", string(bundleJSON))
	}
	if _, ok := uploadedRequests["https://upload.example.com/api/share/release-review-ax71234ka/transcript.json"]; !ok {
		t.Fatalf("expected transcript upload, got %#v", uploadedRequests)
	}
}

func TestWorkspaceExportStandaloneSupportsLegacyChat(t *testing.T) {
	viewerDistDir := createStandaloneViewerDist(t)
	projectDir := t.TempDir()
	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(strings.Join([]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"legacy question"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"legacy answer"}}`,
	}, "\n")), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	meta := state.SessionMeta{
		Key:            "codex:legacy-export",
		Agent:          "codex",
		SessionID:      "legacy-export",
		TranscriptPath: transcriptPath,
		Cwd:            projectDir,
		ProjectName:    "Legacy Project",
		UpdatedAt:      time.Now(),
	}
	previousLegacyState := workspaceLoadLegacyState
	workspaceLoadLegacyState = func() (*state.AppState, error) {
		return &state.AppState{Sessions: map[string]state.SessionMeta{meta.Key: meta}}, nil
	}
	t.Cleanup(func() {
		workspaceLoadLegacyState = previousLegacyState
	})

	raw, err := json.Marshal(map[string]any{
		"chatId":         legacyimport.LegacyChatID(meta),
		"theme":          "dark",
		"attachmentMode": "metadata",
	})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	result, err := workspaceExportStandaloneWithDeps(raw, standaloneExportDeps{
		UploadFile:         func(string, []byte, string, string) error { return nil },
		SharePublicBaseURL: "https://share.example.com",
		ShareSlugSuffix:    "legacy123",
		ShareUploadBaseURL: "https://upload.example.com/api/share",
		ViewerDistDir:      viewerDistDir,
		Now:                time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("workspaceExportStandaloneWithDeps returned error: %v", err)
	}
	success, ok := result.(standaloneExportSuccess)
	if !ok || !success.OK {
		t.Fatalf("expected success, got %#v", result)
	}
	bundle := readStandaloneBundle(t, success.TranscriptJSONPath)
	if bundle["chatId"] != legacyimport.LegacyChatID(meta) || bundle["localPath"] != standaloneShareWorkspacePath {
		t.Fatalf("unexpected bundle metadata: %#v", bundle)
	}
	messages, ok := bundle["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected legacy transcript messages, got %#v", bundle["messages"])
	}
}

func TestWriteStandaloneTranscriptExportBundleMode(t *testing.T) {
	viewerDistDir := createStandaloneViewerDist(t)
	projectDir := t.TempDir()
	uploadsDir := filepath.Join(projectDir, ".abolqasem", "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	attachmentPath := filepath.Join(uploadsDir, "mock.png")
	if err := os.WriteFile(attachmentPath, []byte("mock"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	uploadedPaths := []string{}

	result, err := writeStandaloneTranscriptExport(standaloneExportArgs{
		ChatID:         "chat-1",
		Title:          "Release Review",
		LocalPath:      projectDir,
		Theme:          "light",
		AttachmentMode: "bundle",
		Messages:       createStandaloneMessages(attachmentPath),
	}, standaloneExportDeps{
		UploadFile: func(targetURL string, body []byte, contentType string, cacheControl string) error {
			uploadedPaths = append(uploadedPaths, targetURL)
			return nil
		},
		SharePublicBaseURL: "https://share.example.com",
		ShareSlugSuffix:    "bundle123",
		ShareUploadBaseURL: "https://upload.example.com/api/share",
		ViewerDistDir:      viewerDistDir,
		Now:                time.Date(2026, 4, 23, 12, 34, 56, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("writeStandaloneTranscriptExport returned error: %v", err)
	}
	success, ok := result.(standaloneExportSuccess)
	if !ok || !success.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if success.TotalAttachmentCount != 1 || success.BundledAttachmentCount != 1 || success.UploadedFileCount != 2 {
		t.Fatalf("unexpected attachment counts: %#v", success)
	}
	bundle := readStandaloneBundle(t, success.TranscriptJSONPath)
	attachment := firstStandaloneAttachment(t, bundle)
	contentURL, _ := attachment["contentUrl"].(string)
	if !strings.HasPrefix(contentURL, "./attachments/") || attachment["absolutePath"] != contentURL || attachment["relativePath"] != contentURL {
		t.Fatalf("unexpected bundled attachment: %#v", attachment)
	}
	content, err := os.ReadFile(filepath.Join(success.OutputDir, strings.TrimPrefix(contentURL, "./")))
	if err != nil {
		t.Fatalf("expected bundled attachment: %v", err)
	}
	if string(content) != "mock" {
		t.Fatalf("unexpected attachment content: %q", string(content))
	}
	if len(uploadedPaths) != 2 || uploadedPaths[0] != "https://upload.example.com/api/share/release-review-bundle123/transcript.json" || !strings.Contains(uploadedPaths[1], "/attachments/") {
		t.Fatalf("unexpected uploaded paths: %#v", uploadedPaths)
	}
}

func TestWriteStandaloneTranscriptExportReturnsFailurePayloadWhenShareUploadFails(t *testing.T) {
	viewerDistDir := createStandaloneViewerDist(t)
	projectDir := t.TempDir()
	uploadsDir := filepath.Join(projectDir, ".abolqasem", "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	attachmentPath := filepath.Join(uploadsDir, "mock.png")
	if err := os.WriteFile(attachmentPath, []byte("mock"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	result, err := writeStandaloneTranscriptExport(standaloneExportArgs{
		ChatID:         "chat-1",
		Title:          "Release Review",
		LocalPath:      projectDir,
		Theme:          "light",
		AttachmentMode: "bundle",
		Messages:       createStandaloneMessages(attachmentPath),
	}, standaloneExportDeps{
		UploadFile: func(targetURL string, body []byte, contentType string, cacheControl string) error {
			return errors.New("No release viewer assets were found")
		},
		SharePublicBaseURL: "https://share.example.com",
		ShareSlugSuffix:    "failed123",
		ShareUploadBaseURL: "https://upload.example.com/api/share",
		ViewerDistDir:      viewerDistDir,
		Now:                time.Date(2026, 4, 23, 12, 34, 56, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("writeStandaloneTranscriptExport returned error: %v", err)
	}
	failure, ok := result.(standaloneExportFailure)
	if !ok || failure.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if !strings.Contains(failure.Error, "Failed to upload shared transcript file transcript.json") {
		t.Fatalf("unexpected error: %q", failure.Error)
	}
	if failure.ShareURL != "https://share.example.com/release-review-failed123" {
		t.Fatalf("unexpected share url: %q", failure.ShareURL)
	}
	if failure.TranscriptFileName != "Release-Review-2026-04-23T12-34-56Z-transcript.json" {
		t.Fatalf("unexpected transcript filename: %q", failure.TranscriptFileName)
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(failure.TranscriptJSON), &bundle); err != nil {
		t.Fatalf("invalid transcript JSON: %v", err)
	}
	if bundle["title"] != "Release Review" {
		t.Fatalf("unexpected failure transcript: %#v", bundle)
	}
}

func readStandaloneBundle(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	var bundle map[string]any
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	return bundle
}

func firstStandaloneAttachment(t *testing.T, bundle map[string]any) map[string]any {
	t.Helper()
	messages := bundle["messages"].([]any)
	firstMessage := messages[0].(map[string]any)
	attachments := firstMessage["attachments"].([]any)
	return attachments[0].(map[string]any)
}
