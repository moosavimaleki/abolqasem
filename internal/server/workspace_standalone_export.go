package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ai-agent-manager/internal/workspace/readmodels"
)

const (
	standaloneTranscriptBundleVersion = 1
	standaloneShareUploadBaseURL      = "https://kanna.sh/api/share"
	standaloneSharePublicBaseURL      = "https://share.kanna.sh"
	standaloneShareWorkspacePath      = "/workspace"
	standaloneAssetCacheControl       = "public, max-age=31536000, immutable"
)

var (
	standaloneFileSegmentRE = regexp.MustCompile(`[^\w.-]+`)
	standaloneSlugPartRE    = regexp.MustCompile(`[^a-z0-9]+`)
	standaloneSuffixRE      = regexp.MustCompile(`[^a-z0-9]+`)
)

type standaloneTranscriptBundle struct {
	Version        int                          `json:"version"`
	ChatID         string                       `json:"chatId"`
	Title          string                       `json:"title"`
	LocalPath      string                       `json:"localPath"`
	ExportedAt     string                       `json:"exportedAt"`
	ViewerVersion  string                       `json:"viewerVersion"`
	Theme          string                       `json:"theme"`
	AttachmentMode string                       `json:"attachmentMode"`
	Messages       []readmodels.TranscriptEntry `json:"messages"`
}

type standaloneExportSuccess struct {
	OK                     bool   `json:"ok"`
	OutputDir              string `json:"outputDir"`
	IndexHTMLPath          string `json:"indexHtmlPath"`
	TranscriptJSONPath     string `json:"transcriptJsonPath"`
	AttachmentMode         string `json:"attachmentMode"`
	TotalAttachmentCount   int    `json:"totalAttachmentCount"`
	BundledAttachmentCount int    `json:"bundledAttachmentCount"`
	ShareSlug              string `json:"shareSlug"`
	ShareURL               string `json:"shareUrl"`
	UploadedFileCount      int    `json:"uploadedFileCount"`
}

type standaloneExportFailure struct {
	OK                 bool   `json:"ok"`
	Error              string `json:"error"`
	OutputDir          string `json:"outputDir"`
	TranscriptJSONPath string `json:"transcriptJsonPath"`
	TranscriptFileName string `json:"transcriptFileName"`
	TranscriptJSON     string `json:"transcriptJson"`
	ShareSlug          string `json:"shareSlug"`
	ShareURL           string `json:"shareUrl"`
}

type standaloneExportArgs struct {
	ChatID         string
	Title          string
	LocalPath      string
	Theme          string
	AttachmentMode string
	Messages       []readmodels.TranscriptEntry
}

type standaloneExportDeps struct {
	ViewerDistDir      string
	Now                time.Time
	ShareSlugSuffix    string
	ShareUploadBaseURL string
	SharePublicBaseURL string
	UploadFile         func(targetURL string, body []byte, contentType string, cacheControl string) error
}

type preparedStandaloneMessages struct {
	Messages               []readmodels.TranscriptEntry
	TotalAttachmentCount   int
	BundledAttachmentCount int
}

func workspaceExportStandalone(raw json.RawMessage) (any, error) {
	var payload struct {
		ChatID         string `json:"chatId"`
		Theme          string `json:"theme"`
		AttachmentMode string `json:"attachmentMode"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.ChatID == "" {
		return nil, errors.New("chatId is required")
	}
	store := workspaceStore()
	state, err := store.LoadState()
	if err != nil {
		return nil, err
	}
	chat, ok := state.ChatsByID[payload.ChatID]
	if !ok || chat.DeletedAt != 0 {
		return nil, errors.New("chat not found")
	}
	project, ok := state.ProjectsByID[chat.ProjectID]
	if !ok || project.DeletedAt != 0 {
		return nil, errors.New("project not found")
	}
	transcript, err := workspaceChatTranscriptSnapshot(store, chat.ID, 0)
	if err != nil {
		return nil, err
	}
	return writeStandaloneTranscriptExport(standaloneExportArgs{
		ChatID:         chat.ID,
		Title:          chat.Title,
		LocalPath:      project.LocalPath,
		Theme:          normalizeStandaloneTheme(payload.Theme),
		AttachmentMode: normalizeStandaloneAttachmentMode(payload.AttachmentMode),
		Messages:       transcript.Messages,
	}, standaloneExportDeps{})
}

func writeStandaloneTranscriptExport(args standaloneExportArgs, deps standaloneExportDeps) (any, error) {
	viewerDistDir := firstNonEmptyString(deps.ViewerDistDir, defaultStandaloneViewerDistDir())
	if !pathExists(viewerDistDir) {
		return nil, errors.New("Standalone viewer bundle not found. Run `npm run build:export-viewer`.")
	}
	now := deps.Now
	if now.IsZero() {
		now = time.Now()
	}
	shareUploadBaseURL := firstNonEmptyString(deps.ShareUploadBaseURL, standaloneShareUploadBaseURL)
	sharePublicBaseURL := firstNonEmptyString(deps.SharePublicBaseURL, standaloneSharePublicBaseURL)
	uploadFile := deps.UploadFile
	if uploadFile == nil {
		uploadFile = uploadStandaloneFile
	}

	exportRootDir := filepath.Join(resolveWorkspaceLocalPath(args.LocalPath), ".kanna", "exports")
	if err := os.MkdirAll(exportRootDir, 0o755); err != nil {
		return nil, err
	}
	outputDir, err := resolveUniqueStandaloneExportDir(exportRootDir, firstNonEmptyString(args.Title, args.ChatID), now)
	if err != nil {
		return nil, err
	}
	if err := copyDirectory(viewerDistDir, outputDir); err != nil {
		return nil, err
	}

	attachmentsDir := filepath.Join(outputDir, "attachments")
	prepared, err := prepareStandaloneMessages(args.Messages, args.AttachmentMode, args.LocalPath, attachmentsDir)
	if err != nil {
		return nil, err
	}
	bundle := standaloneTranscriptBundle{
		Version:        standaloneTranscriptBundleVersion,
		ChatID:         args.ChatID,
		Title:          args.Title,
		LocalPath:      standaloneShareWorkspacePath,
		ExportedAt:     now.UTC().Format(time.RFC3339Nano),
		ViewerVersion:  normalizedAppVersion(),
		Theme:          normalizeStandaloneTheme(args.Theme),
		AttachmentMode: normalizeStandaloneAttachmentMode(args.AttachmentMode),
		Messages:       prepared.Messages,
	}
	transcriptJSONBytes, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, err
	}
	transcriptJSONBytes = append(transcriptJSONBytes, '\n')
	transcriptJSON := string(transcriptJSONBytes)
	transcriptJSONPath := filepath.Join(outputDir, "transcript.json")
	if err := os.WriteFile(transcriptJSONPath, transcriptJSONBytes, 0o644); err != nil {
		return nil, err
	}

	shareSlug := buildStandaloneShareSlug(firstNonEmptyString(args.Title, args.ChatID), deps.ShareSlugSuffix)
	shareURL := buildStandaloneShareURL(sharePublicBaseURL, shareSlug)
	uploadedFileCount, err := uploadStandaloneExportDirectory(outputDir, shareSlug, shareUploadBaseURL, uploadFile)
	if err != nil {
		return standaloneExportFailure{
			OK:                 false,
			Error:              err.Error(),
			OutputDir:          outputDir,
			TranscriptJSONPath: transcriptJSONPath,
			TranscriptFileName: filepath.Base(outputDir) + "-transcript.json",
			TranscriptJSON:     transcriptJSON,
			ShareSlug:          shareSlug,
			ShareURL:           shareURL,
		}, nil
	}

	return standaloneExportSuccess{
		OK:                     true,
		OutputDir:              outputDir,
		IndexHTMLPath:          filepath.Join(outputDir, "index.html"),
		TranscriptJSONPath:     transcriptJSONPath,
		AttachmentMode:         bundle.AttachmentMode,
		TotalAttachmentCount:   prepared.TotalAttachmentCount,
		BundledAttachmentCount: prepared.BundledAttachmentCount,
		ShareSlug:              shareSlug,
		ShareURL:               shareURL,
		UploadedFileCount:      uploadedFileCount,
	}, nil
}

func prepareStandaloneMessages(messages []readmodels.TranscriptEntry, attachmentMode string, localPath string, attachmentsDir string) (preparedStandaloneMessages, error) {
	clone := make([]readmodels.TranscriptEntry, 0, len(messages))
	data, err := json.Marshal(messages)
	if err != nil {
		return preparedStandaloneMessages{}, err
	}
	if err := json.Unmarshal(data, &clone); err != nil {
		return preparedStandaloneMessages{}, err
	}

	totalAttachmentCount := 0
	bundledAttachmentCount := 0
	attachmentsDirCreated := false
	for _, message := range clone {
		if kind, _ := message["kind"].(string); kind != "user_prompt" {
			continue
		}
		attachments, ok := message["attachments"].([]any)
		if !ok || len(attachments) == 0 {
			continue
		}
		totalAttachmentCount += len(attachments)
		for _, rawAttachment := range attachments {
			attachment, ok := rawAttachment.(map[string]any)
			if !ok {
				continue
			}
			if attachmentMode == "metadata" {
				rewriteStandaloneAttachmentAsMetadata(attachment)
				continue
			}
			absolutePath, _ := attachment["absolutePath"].(string)
			if absolutePath == "" || !pathExists(absolutePath) {
				rewriteStandaloneAttachmentAsMetadata(attachment)
				continue
			}
			if !attachmentsDirCreated {
				if err := os.MkdirAll(attachmentsDir, 0o755); err != nil {
					return preparedStandaloneMessages{}, err
				}
				attachmentsDirCreated = true
			}
			displayName, _ := attachment["displayName"].(string)
			attachmentID, _ := attachment["id"].(string)
			exportedFileName := sanitizeFileNameSegment(firstNonEmptyString(attachmentID, "attachment")) + "-" + sanitizeFileNameSegment(filepath.Base(firstNonEmptyString(displayName, absolutePath)))
			destinationPath := filepath.Join(attachmentsDir, exportedFileName)
			if err := copyFile(absolutePath, destinationPath); err != nil {
				rewriteStandaloneAttachmentAsMetadata(attachment)
				continue
			}
			bundledAttachmentCount += 1
			relativeDestinationPath := "./attachments/" + exportedFileName
			attachment["absolutePath"] = relativeDestinationPath
			attachment["relativePath"] = relativeDestinationPath
			attachment["contentUrl"] = relativeDestinationPath
		}
	}
	rewriteLocalPathsForStandaloneShare(clone, resolveWorkspaceLocalPath(localPath))
	return preparedStandaloneMessages{
		Messages:               clone,
		TotalAttachmentCount:   totalAttachmentCount,
		BundledAttachmentCount: bundledAttachmentCount,
	}, nil
}

func uploadStandaloneExportDirectory(outputDir string, shareSlug string, uploadBaseURL string, uploadFile func(string, []byte, string, string) error) (int, error) {
	filePaths, err := listStandaloneUploadFiles(outputDir)
	if err != nil {
		return 0, err
	}
	uploadedFileCount := 0
	for _, filePath := range filePaths {
		relativePath, err := filepath.Rel(outputDir, filePath)
		if err != nil {
			return uploadedFileCount, err
		}
		relativePath = filepath.ToSlash(relativePath)
		body, err := os.ReadFile(filePath)
		if err != nil {
			return uploadedFileCount, err
		}
		targetURL := buildStandaloneShareUploadURL(uploadBaseURL, shareSlug, relativePath)
		if err := uploadFile(targetURL, body, contentTypeForPath(relativePath), standaloneCacheControlForPath(relativePath)); err != nil {
			return uploadedFileCount, fmt.Errorf("Failed to upload shared transcript file %s: %w", relativePath, err)
		}
		uploadedFileCount += 1
	}
	return uploadedFileCount, nil
}

func listStandaloneUploadFiles(outputDir string) ([]string, error) {
	filePaths := []string{filepath.Join(outputDir, "transcript.json")}
	attachmentsDir := filepath.Join(outputDir, "attachments")
	if pathExists(attachmentsDir) {
		attachmentFiles, err := listStandaloneFiles(attachmentsDir)
		if err != nil {
			return nil, err
		}
		filePaths = append(filePaths, attachmentFiles...)
	}
	return filePaths, nil
}

func listStandaloneFiles(rootDir string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func uploadStandaloneFile(targetURL string, body []byte, contentType string, cacheControl string) error {
	request, err := http.NewRequest(http.MethodPut, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Cache-Control", cacheControl)
	request.Header.Set("Content-Type", contentType)
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		if text := strings.TrimSpace(string(detail)); text != "" {
			return errors.New(text)
		}
		return fmt.Errorf("status %d", response.StatusCode)
	}
	return nil
}

func resolveUniqueStandaloneExportDir(exportRootDir string, title string, now time.Time) (string, error) {
	baseName := firstNonEmptyString(sanitizeFileNameSegment(title), "chat") + "-" + formatStandaloneExportTimestamp(now)
	candidate := filepath.Join(exportRootDir, baseName)
	suffix := 2
	for pathExists(candidate) {
		candidate = filepath.Join(exportRootDir, fmt.Sprintf("%s-%d", baseName, suffix))
		suffix += 1
	}
	return candidate, nil
}

func copyDirectory(sourceDir string, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, rel)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if entry.Type().IsRegular() {
			return copyFile(path, targetPath)
		}
		return nil
	})
}

func copyFile(sourcePath string, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
}

func rewriteStandaloneAttachmentAsMetadata(attachment map[string]any) {
	attachment["absolutePath"] = ""
	attachment["relativePath"] = ""
	attachment["contentUrl"] = ""
}

func rewriteLocalPathsForStandaloneShare(value any, localPath string) any {
	if localPath == "" {
		return value
	}
	switch typed := value.(type) {
	case string:
		return strings.ReplaceAll(typed, localPath, standaloneShareWorkspacePath)
	case []readmodels.TranscriptEntry:
		for index := range typed {
			typed[index] = rewriteLocalPathsForStandaloneShare(typed[index], localPath).(readmodels.TranscriptEntry)
		}
	case readmodels.TranscriptEntry:
		for key, nested := range typed {
			typed[key] = rewriteLocalPathsForStandaloneShare(nested, localPath)
		}
	case []any:
		for index := range typed {
			typed[index] = rewriteLocalPathsForStandaloneShare(typed[index], localPath)
		}
	case map[string]any:
		for key, nested := range typed {
			typed[key] = rewriteLocalPathsForStandaloneShare(nested, localPath)
		}
	}
	return value
}

func defaultStandaloneViewerDistDir() string {
	for _, candidate := range []string{
		filepath.Join("web-react", "dist", "export-viewer"),
		filepath.Join("dist", "export-viewer"),
		filepath.Join("web", "export-viewer"),
	} {
		if pathExists(candidate) {
			return candidate
		}
	}
	return filepath.Join("web-react", "dist", "export-viewer")
}

func normalizeStandaloneTheme(theme string) string {
	switch theme {
	case "dark", "light":
		return theme
	default:
		return "dark"
	}
}

func normalizeStandaloneAttachmentMode(mode string) string {
	if mode == "metadata" {
		return "metadata"
	}
	return "bundle"
}

func formatStandaloneExportTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02T15-04-05Z")
}

func sanitizeFileNameSegment(value string) string {
	value = strings.TrimSpace(value)
	value = standaloneFileSegmentRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func buildStandaloneShareSlug(title string, providedSuffix string) string {
	baseSlug := strings.ToLower(strings.TrimSpace(title))
	baseSlug = standaloneSlugPartRE.ReplaceAllString(baseSlug, "-")
	baseSlug = strings.Trim(baseSlug, "-")
	if len(baseSlug) > 64 {
		baseSlug = baseSlug[:64]
	}
	if baseSlug == "" {
		baseSlug = "chat"
	}
	suffix := strings.ToLower(strings.TrimSpace(providedSuffix))
	if suffix == "" {
		suffix = generateStandaloneShareSlugSuffix()
	}
	suffix = standaloneSuffixRE.ReplaceAllString(suffix, "")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	if suffix == "" {
		suffix = "share"
	}
	return baseSlug + "-" + suffix
}

func generateStandaloneShareSlugSuffix() string {
	maxValue := new(big.Int).Lsh(big.NewInt(1), 64)
	value, err := rand.Int(rand.Reader, maxValue)
	if err != nil {
		return fmt.Sprintf("%010d", time.Now().UnixNano()%10_000_000_000)
	}
	suffix := value.Text(36)
	if len(suffix) > 10 {
		suffix = suffix[:10]
	}
	for len(suffix) < 10 {
		suffix = "0" + suffix
	}
	return suffix
}

func buildStandaloneShareURL(baseURL string, shareSlug string) string {
	return strings.TrimRight(baseURL, "/") + "/" + shareSlug
}

func buildStandaloneShareUploadURL(baseURL string, shareSlug string, relativePath string) string {
	segments := append([]string{shareSlug}, strings.Split(relativePath, "/")...)
	for index, segment := range segments {
		segments[index] = urlPathEscape(segment)
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.Join(segments, "/")
}

func contentTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".gif":
		return "image/gif"
	case ".html":
		return "text/html; charset=utf-8"
	case ".ico":
		return "image/x-icon"
	case ".jpeg", ".jpg":
		return "image/jpeg"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".manifest", ".webmanifest":
		return "application/manifest+json; charset=utf-8"
	case ".mp3":
		return "audio/mpeg"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".webp":
		return "image/webp"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

func standaloneCacheControlForPath(string) string {
	return standaloneAssetCacheControl
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func urlPathEscape(segment string) string {
	return url.PathEscape(segment)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
