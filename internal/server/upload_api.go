package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxUploadBytes = 25 << 20

type uploadAttachment struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	DisplayName  string `json:"displayName"`
	AbsolutePath string `json:"absolutePath"`
	RelativePath string `json:"relativePath"`
	ContentURL   string `json:"contentUrl"`
	MimeType     string `json:"mimeType"`
	Size         int64  `json:"size"`
}

func handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	projectID, rest, ok := parseProjectAPIPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if rest == "uploads" && r.Method == http.MethodPost {
		handleAPIUpload(w, r, projectID)
		return
	}
	if strings.HasPrefix(rest, "uploads/") {
		handleAPIUploadedFile(w, r, projectID, strings.TrimPrefix(rest, "uploads/"))
		return
	}
	http.NotFound(w, r)
}

func handleAPIUpload(w http.ResponseWriter, r *http.Request, projectID string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "Upload is too large or invalid."})
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "No files uploaded."})
		return
	}
	attachments := make([]uploadAttachment, 0, len(files))
	for _, header := range files {
		if header.Size > maxUploadBytes {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "File exceeds upload limit."})
			return
		}
		file, err := header.Open()
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "Could not open uploaded file."})
			return
		}
		attachment, err := saveUploadedFile(projectID, header.Filename, header.Header.Get("Content-Type"), header.Size, file)
		_ = file.Close()
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		attachments = append(attachments, attachment)
	}
	writeJSON(w, map[string]any{"attachments": attachments})
}

func handleAPIUploadedFile(w http.ResponseWriter, r *http.Request, projectID string, rest string) {
	uploadID := strings.TrimSuffix(rest, "/content")
	uploadID = strings.Trim(uploadID, "/")
	if uploadID == "" || strings.Contains(uploadID, "/") || strings.Contains(uploadID, `\`) {
		http.NotFound(w, r)
		return
	}
	attachment, err := loadUploadMetadata(projectID, uploadID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(rest, "/content"):
		w.Header().Set("Content-Type", attachment.MimeType)
		http.ServeFile(w, r, attachment.AbsolutePath)
	case r.Method == http.MethodDelete && !strings.HasSuffix(rest, "/content"):
		_ = os.Remove(attachment.AbsolutePath)
		_ = os.Remove(metadataPath(projectID, uploadID))
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func saveUploadedFile(projectID string, originalName string, headerMime string, size int64, reader io.Reader) (uploadAttachment, error) {
	projectID = safeSegment(projectID)
	if projectID == "" {
		return uploadAttachment{}, errors.New("invalid project id")
	}
	displayName := filepath.Base(strings.TrimSpace(originalName))
	if displayName == "." || displayName == string(filepath.Separator) || displayName == "" {
		displayName = "upload"
	}
	uploadID := randomID() + "-" + safeFilename(displayName)
	dir := uploadDir(projectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return uploadAttachment{}, err
	}
	absolutePath := filepath.Join(dir, uploadID)
	target, err := os.OpenFile(absolutePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return uploadAttachment{}, err
	}
	defer target.Close()

	limited := io.LimitReader(reader, maxUploadBytes+1)
	written, err := io.Copy(target, limited)
	if err != nil {
		return uploadAttachment{}, err
	}
	if written > maxUploadBytes || (size > 0 && size > maxUploadBytes) {
		_ = os.Remove(absolutePath)
		return uploadAttachment{}, errors.New("file exceeds upload limit")
	}
	mimeType := detectUploadMime(absolutePath, headerMime)
	attachment := uploadAttachment{
		ID:           uploadID,
		Kind:         attachmentKind(mimeType),
		DisplayName:  displayName,
		AbsolutePath: absolutePath,
		RelativePath: "./.kanna/uploads/" + uploadID,
		ContentURL:   "/api/projects/" + projectID + "/uploads/" + uploadID + "/content",
		MimeType:     mimeType,
		Size:         written,
	}
	if err := saveUploadMetadata(projectID, attachment); err != nil {
		_ = os.Remove(absolutePath)
		return uploadAttachment{}, err
	}
	return attachment, nil
}

func parseProjectAPIPath(path string) (string, string, bool) {
	path = strings.TrimPrefix(path, "/api/projects/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	return safeSegment(parts[0]), strings.Trim(parts[1], "/"), true
}

func saveUploadMetadata(projectID string, attachment uploadAttachment) error {
	data, err := json.MarshalIndent(attachment, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath(projectID, attachment.ID), data, 0o600)
}

func loadUploadMetadata(projectID string, uploadID string) (uploadAttachment, error) {
	data, err := os.ReadFile(metadataPath(projectID, uploadID))
	if err != nil {
		return uploadAttachment{}, err
	}
	var attachment uploadAttachment
	if err := json.Unmarshal(data, &attachment); err != nil {
		return uploadAttachment{}, err
	}
	if attachment.AbsolutePath == "" || !strings.HasPrefix(filepath.Clean(attachment.AbsolutePath), uploadDir(safeSegment(projectID))) {
		return uploadAttachment{}, errors.New("invalid attachment path")
	}
	return attachment, nil
}

func metadataPath(projectID string, uploadID string) string {
	return filepath.Join(uploadDir(safeSegment(projectID)), uploadID+".json")
}

func uploadDir(projectID string) string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "ai-agent-manager", "uploads", safeSegment(projectID))
}

func detectUploadMime(path string, headerMime string) string {
	headerMime = strings.TrimSpace(strings.Split(headerMime, ";")[0])
	if headerMime != "" && headerMime != "application/octet-stream" {
		return headerMime
	}
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		buffer := make([]byte, 512)
		n, _ := file.Read(buffer)
		if n > 0 {
			return http.DetectContentType(buffer[:n])
		}
	}
	if mimeType := mime.TypeByExtension(filepath.Ext(path)); mimeType != "" {
		return strings.Split(mimeType, ";")[0]
	}
	return "application/octet-stream"
}

func attachmentKind(mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		return "image"
	}
	return "file"
}

var unsafeSegmentChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeSegment(value string) string {
	value = unsafeSegmentChars.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, ".-_")
	return value
}

func safeFilename(value string) string {
	value = safeSegment(value)
	if value == "" {
		return "upload"
	}
	return value
}

func randomID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "upload"
	}
	return hex.EncodeToString(data[:])
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
