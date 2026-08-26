package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	telegramPreviewCallbackPrefix = "preview:"
	telegramPreviewLifetime       = 30 * time.Minute
	telegramPreviewButtonLimit    = 8
	telegramPreviewMaxCodeLines   = 2400
)

type telegramPreviewKind string

const (
	telegramPreviewFile    telegramPreviewKind = "file"
	telegramPreviewMermaid telegramPreviewKind = "mermaid"
)

type telegramPreviewItem struct {
	Kind            telegramPreviewKind
	TelegramChatID  string
	WorkspaceChatID string
	FilePath        string
	Line            int
	MermaidSource   string
	CreatedAt       time.Time
}

type telegramPreviewButton struct {
	Label string
	Token string
}

var telegramMarkdownFileLink = regexp.MustCompile(`\[([^\]]+)\]\((/[^)\s]+)\)`)

// Codex commonly writes source references as plain `path/to/file:line` text.
// Keep this deliberately path-shaped and validate every match against the
// selected project before creating a preview callback.
var telegramPlainFileReference = regexp.MustCompile(`(?m)(^|[^[:alnum:]_])((?:/|[[:alnum:]_.~-]+/)[^:\n]*:[1-9][0-9]*(?::[1-9][0-9]*)?)([[:space:],;)}\]]|$)`)

// sendTranscript keeps the readable transcript in Rich Markdown and places
// heavyweight previews behind explicit callback buttons. Telegram callback data
// has a 64-byte limit, so the file path/source is kept server-side only.
func (b *telegramBridge) sendTranscript(ctx context.Context, token, telegramChatID, workspaceChatID, markdown string) {
	markdown, buttons := b.telegramTranscriptPreviews(telegramChatID, workspaceChatID, markdown)
	if len(buttons) == 0 {
		b.sendText(ctx, token, telegramChatID, markdown)
		return
	}
	rows := make([][]map[string]string, 0, len(buttons))
	for _, button := range buttons {
		rows = append(rows, []map[string]string{{
			"text":          truncateTelegramButtonLabel(button.Label, 60),
			"callback_data": telegramPreviewCallbackPrefix + button.Token,
		}})
	}
	// Keep the buttons attached to the transcript itself. Sending a second
	// message made the controls easy to miss (and could leave only the plain
	// path visible when Telegram delayed the follow-up request).
	b.sendTextWithMarkup(ctx, token, telegramChatID, markdown+"\n\nبرای باز کردن فایل‌ها یا نمودارها، گزینهٔ مربوط را انتخاب کنید.", map[string]any{"inline_keyboard": rows})
}

func (b *telegramBridge) telegramTranscriptPreviews(telegramChatID, workspaceChatID, markdown string) (string, []telegramPreviewButton) {
	buttons := make([]telegramPreviewButton, 0, telegramPreviewButtonLimit)
	markdown = b.telegramReplaceMermaidBlocks(telegramChatID, workspaceChatID, markdown, &buttons)
	markdown = telegramMarkdownFileLink.ReplaceAllStringFunc(markdown, func(raw string) string {
		if len(buttons) >= telegramPreviewButtonLimit {
			return raw
		}
		parts := telegramMarkdownFileLink.FindStringSubmatch(raw)
		if len(parts) != 3 {
			return raw
		}
		path, line, ok := telegramResolvePreviewFilePath(workspaceChatID, parts[2])
		if !ok {
			return raw
		}
		token := b.rememberTelegramPreview(telegramPreviewItem{
			Kind: telegramPreviewFile, TelegramChatID: telegramChatID, WorkspaceChatID: workspaceChatID, FilePath: path, Line: line, CreatedAt: time.Now(),
		})
		if token == "" {
			return raw
		}
		buttons = append(buttons, telegramPreviewButton{Label: "📄 " + firstNonEmptyString(parts[1], filepath.Base(path)), Token: token})
		return "`" + telegramMarkdownInline(parts[1]) + "`"
	})
	markdown = telegramPlainFileReference.ReplaceAllStringFunc(markdown, func(raw string) string {
		if len(buttons) >= telegramPreviewButtonLimit {
			return raw
		}
		parts := telegramPlainFileReference.FindStringSubmatch(raw)
		if len(parts) != 4 {
			return raw
		}
		// A Markdown link target has already been handled above. Do not create a
		// second callback for the same path after it was rendered inline.
		if parts[1] == "(" {
			return raw
		}
		path, line, ok := telegramResolvePreviewFilePath(workspaceChatID, parts[2])
		if !ok {
			return raw
		}
		token := b.rememberTelegramPreview(telegramPreviewItem{
			Kind: telegramPreviewFile, TelegramChatID: telegramChatID, WorkspaceChatID: workspaceChatID, FilePath: path, Line: line, CreatedAt: time.Now(),
		})
		if token == "" {
			return raw
		}
		buttons = append(buttons, telegramPreviewButton{Label: "📄 " + filepath.Base(path), Token: token})
		return parts[1] + "`" + telegramMarkdownInline(parts[2]) + "`" + parts[3]
	})
	return markdown, buttons
}

func (b *telegramBridge) telegramReplaceMermaidBlocks(telegramChatID, workspaceChatID, markdown string, buttons *[]telegramPreviewButton) string {
	lines := strings.Split(markdown, "\n")
	result := make([]string, 0, len(lines))
	for index := 0; index < len(lines); index++ {
		if !strings.EqualFold(strings.TrimSpace(lines[index]), "```mermaid") {
			result = append(result, lines[index])
			continue
		}
		end := index + 1
		for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "```") {
			end++
		}
		if end >= len(lines) || len(*buttons) >= telegramPreviewButtonLimit {
			result = append(result, lines[index])
			continue
		}
		source := strings.TrimSpace(strings.Join(lines[index+1:end], "\n"))
		if source == "" {
			result = append(result, lines[index:end+1]...)
			index = end
			continue
		}
		token := b.rememberTelegramPreview(telegramPreviewItem{Kind: telegramPreviewMermaid, TelegramChatID: telegramChatID, WorkspaceChatID: workspaceChatID, MermaidSource: source, CreatedAt: time.Now()})
		if token == "" {
			result = append(result, lines[index:end+1]...)
			index = end
			continue
		}
		*buttons = append(*buttons, telegramPreviewButton{Label: "📈 چارت Mermaid", Token: token})
		result = append(result, "📈 چارت Mermaid — برای دیدن، دکمهٔ زیر را بزنید.")
		index = end
	}
	return strings.Join(result, "\n")
}

func telegramPreviewFilePath(raw string) (string, int, bool) {
	decoded, line, ok := telegramPreviewReferenceParts(raw)
	if !ok || !filepath.IsAbs(decoded) {
		return "", 0, false
	}
	return filepath.Clean(decoded), line, true
}

func telegramPreviewReferenceParts(raw string) (string, int, bool) {
	decoded, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil {
		return "", 0, false
	}
	line := 0
	if colon := strings.LastIndex(decoded, ":"); colon > strings.LastIndex(decoded, "/") {
		linePart := decoded[colon+1:]
		// Source references may include a column (`file.py:29:4`). The
		// preview is line-oriented, so discard the column while retaining 29.
		if previousColon := strings.LastIndex(decoded[:colon], ":"); previousColon > strings.LastIndex(decoded[:colon], "/") {
			linePart = decoded[previousColon+1 : colon]
			decoded = decoded[:previousColon]
		} else {
			decoded = decoded[:colon]
		}
		if parsed, err := strconv.Atoi(linePart); err == nil && parsed > 0 {
			line = parsed
		}
	}
	if decoded == "" || filepath.Clean(decoded) == "." {
		return "", 0, false
	}
	return filepath.Clean(decoded), line, true
}

func telegramResolvePreviewFilePath(workspaceChatID, raw string) (string, int, bool) {
	decoded, line, ok := telegramPreviewReferenceParts(raw)
	if !ok {
		return "", 0, false
	}
	_, project, err := workspaceChatProjectRequired(workspaceChatID)
	if err != nil {
		return "", 0, false
	}
	var path string
	if filepath.IsAbs(decoded) {
		path, err = resolvePreviewPath([]string{project.LocalPath}, decoded)
		if err != nil {
			return "", 0, false
		}
		return path, line, true
	}

	// Codex often prefixes a project-relative path with the project directory
	// name (for example `eitaa-apk/svg.py`) even when that directory is already
	// the selected workspace root. Try both spellings while keeping resolution
	// strictly inside the project root.
	candidates := []string{filepath.Join(project.LocalPath, decoded)}
	projectName := filepath.Base(filepath.Clean(project.LocalPath))
	prefix := projectName + string(filepath.Separator)
	if projectName != "." && strings.HasPrefix(decoded, prefix) {
		candidates = append(candidates, filepath.Join(project.LocalPath, strings.TrimPrefix(decoded, prefix)))
	}
	for _, candidate := range candidates {
		path, err = resolvePreviewPath([]string{project.LocalPath}, candidate)
		if err == nil {
			return path, line, true
		}
	}
	return "", 0, false
}

func telegramPreviewPathAllowed(workspaceChatID, path string) bool {
	_, project, err := workspaceChatProjectRequired(workspaceChatID)
	if err != nil {
		return false
	}
	_, err = resolvePreviewPath([]string{project.LocalPath}, path)
	return err == nil
}

func (b *telegramBridge) rememberTelegramPreview(item telegramPreviewItem) string {
	bytes := make([]byte, 9)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	token := hex.EncodeToString(bytes)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.previewByToken == nil {
		b.previewByToken = map[string]telegramPreviewItem{}
	}
	cutoff := time.Now().Add(-telegramPreviewLifetime)
	for existing, candidate := range b.previewByToken {
		if candidate.CreatedAt.Before(cutoff) {
			delete(b.previewByToken, existing)
		}
	}
	b.previewByToken[token] = item
	return token
}

func (b *telegramBridge) telegramPreviewForCallback(data, telegramChatID, workspaceChatID string) (telegramPreviewItem, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(data, telegramPreviewCallbackPrefix))
	if !strings.HasPrefix(data, telegramPreviewCallbackPrefix) || token == "" || len([]byte(data)) > 64 {
		return telegramPreviewItem{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.previewByToken[token]
	if !ok || item.CreatedAt.Before(time.Now().Add(-telegramPreviewLifetime)) || item.TelegramChatID != telegramChatID || item.WorkspaceChatID != workspaceChatID {
		return telegramPreviewItem{}, false
	}
	delete(b.previewByToken, token)
	return item, true
}

func (b *telegramBridge) sendTelegramPreview(ctx context.Context, token, telegramChatID string, item telegramPreviewItem) {
	var filename, caption string
	var data []byte
	var err error
	switch item.Kind {
	case telegramPreviewFile:
		_, project, projectErr := workspaceChatProjectRequired(item.WorkspaceChatID)
		if projectErr != nil {
			err = projectErr
			break
		}
		// Telegram previews should show the file, not only the context around the
		// referenced line. Keep a generous safety cap for SVG dimensions.
		preview, previewErr := buildFilePreview([]string{project.LocalPath}, item.FilePath, item.Line, filePreviewOptions{Full: true})
		if previewErr != nil {
			err = previewErr
			break
		}
		filename = filepath.Base(preview.Path) + ".svg"
		caption = "پیش‌نمایش رنگی " + filepath.Base(preview.Path)
		data = []byte(telegramCodePreviewSVG(preview))
	case telegramPreviewMermaid:
		filename = "mermaid-chart.svg"
		caption = "چارت Mermaid"
		data = []byte(telegramMermaidSVG(item.MermaidSource))
	default:
		err = fmt.Errorf("unknown preview type")
	}
	if err != nil {
		b.sendText(ctx, token, telegramChatID, "ساخت پیش‌نمایش ناموفق بود: "+err.Error())
		return
	}
	if err := b.sendDocument(ctx, token, telegramChatID, filename, data, caption); err != nil {
		b.setError(err)
		b.sendText(ctx, token, telegramChatID, "ارسال پیش‌نمایش ناموفق بود: "+err.Error())
	}
}

func (b *telegramBridge) sendDocument(ctx context.Context, token, chatID, filename string, data []byte, caption string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("chat_id", chatID)
	_ = writer.WriteField("caption", caption)
	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	endpoint := strings.TrimRight(b.apiBaseURL, "/") + "/bot" + url.PathEscape(token) + "/sendDocument"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return redactTelegramToken(err, token)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	var response telegramAPIResponse
	if err := b.do(request, &response); err != nil {
		return redactTelegramToken(err, token)
	}
	if !response.OK {
		return redactTelegramToken(fmt.Errorf("telegram sendDocument: %s", firstNonEmptyString(response.Error, "request failed")), token)
	}
	return nil
}

func telegramCodePreviewSVG(preview filePreviewResponse) string {
	lines := preview.Lines
	if len(lines) == 0 {
		lines = []filePreviewLine{{Number: 1, Text: "(empty file)"}}
	}
	if len(lines) > telegramPreviewMaxCodeLines {
		lines = append(append([]filePreviewLine(nil), lines[:telegramPreviewMaxCodeLines]...), filePreviewLine{
			Number: lines[telegramPreviewMaxCodeLines-1].Number + 1,
			Text:   fmt.Sprintf("… preview truncated; download the document for the remaining %d lines …", len(preview.Lines)-telegramPreviewMaxCodeLines),
		})
	}
	maxColumns := 0
	for _, line := range lines {
		maxColumns = max(maxColumns, min(120, len([]rune(line.Text)))+len(strconv.Itoa(line.Number)))
	}
	width := max(720, min(1440, 84+maxColumns*8))
	height := 54 + len(lines)*22
	var output strings.Builder
	// Telegram's SVG viewer does not consistently paint percentage-sized rects.
	// Use explicit dimensions so the document is opaque instead of checkerboard.
	fmt.Fprintf(&output, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><rect x="0" y="0" width="%d" height="%d" fill="#071426"/><rect x="0" y="0" width="%d" height="42" fill="#0d1d33"/><circle cx="22" cy="21" r="5" fill="#ef6b73"/><circle cx="40" cy="21" r="5" fill="#e7b35b"/><circle cx="58" cy="21" r="5" fill="#45c486"/><text x="82" y="26" fill="#d7e4f7" font-family="monospace" font-size="14">%s · %s</text>`, width, height, width, height, width, height, width, html.EscapeString(filepath.Base(preview.Path)), html.EscapeString(preview.Language))
	for index, line := range lines {
		y := 64 + index*22
		if line.Highlight {
			fmt.Fprintf(&output, `<rect x="0" y="%d" width="%d" height="22" fill="#16355a"/>`, y-16, width)
		}
		fmt.Fprintf(&output, `<text x="16" y="%d" text-anchor="end" fill="#506785" font-family="monospace" font-size="13">%d</text>`, y, line.Number)
		fmt.Fprintf(&output, `<text x="34" y="%d" fill="%s" font-family="monospace" font-size="14" xml:space="preserve">%s</text>`, y, telegramCodeColor(line.Text), html.EscapeString(telegramTrimCodeLine(line.Text)))
	}
	output.WriteString(`</svg>`)
	return output.String()
}

func telegramTrimCodeLine(value string) string {
	runes := []rune(value)
	if len(runes) > 120 {
		return string(runes[:119]) + "…"
	}
	return value
}

func telegramCodeColor(line string) string {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "--"):
		return "#6f8cae"
	case strings.Contains(trimmed, `"`) || strings.Contains(trimmed, "'"):
		return "#eab676"
	case strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "function "):
		return "#b99aff"
	case strings.Contains(trimmed, "return") || strings.Contains(trimmed, "if ") || strings.Contains(trimmed, "for "):
		return "#78b7ff"
	default:
		return "#d7e4f7"
	}
}

type telegramMermaidNode struct {
	ID    string
	Label string
}

type telegramMermaidEdge struct{ From, To string }

var telegramMermaidEdgePattern = regexp.MustCompile(`^\s*([[:alnum:]_-]+)(?:\[[^\]]*\]|\([^)]*\)|\{[^}]*\})?\s*[-.=]+>\s*([[:alnum:]_-]+)(?:\[[^\]]*\]|\([^)]*\)|\{[^}]*\})?`)

// telegramMermaidSVG deliberately supports the common flowchart edge syntax.
// It produces a vector document (rather than a screenshot) so Telegram users
// receive a crisp, downloadable chart even when the host has no browser.
func telegramMermaidSVG(source string) string {
	nodes := map[string]telegramMermaidNode{}
	edges := make([]telegramMermaidEdge, 0)
	for _, line := range strings.Split(source, "\n") {
		match := telegramMermaidEdgePattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		for _, id := range []string{match[1], match[2]} {
			if _, ok := nodes[id]; !ok {
				nodes[id] = telegramMermaidNode{ID: id, Label: id}
			}
		}
		edges = append(edges, telegramMermaidEdge{From: match[1], To: match[2]})
	}
	if len(nodes) == 0 {
		return telegramMermaidSourceSVG(source)
	}
	ordered := make([]telegramMermaidNode, 0, len(nodes))
	for _, node := range nodes {
		ordered = append(ordered, node)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	positions := map[string][2]int{}
	for index, node := range ordered {
		positions[node.ID] = [2]int{100 + (index%3)*250, 100 + (index/3)*130}
	}
	rows := (len(ordered) + 2) / 3
	width, height := 780, max(220, 110+rows*130)
	var output strings.Builder
	fmt.Fprintf(&output, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><defs><marker id="arrow" markerWidth="10" markerHeight="10" refX="9" refY="3" orient="auto"><path d="M0,0 L0,6 L9,3 z" fill="#83b6f4"/></marker></defs><rect x="0" y="0" width="%d" height="%d" rx="16" fill="#071426"/><text x="24" y="32" fill="#d7e4f7" font-family="sans-serif" font-size="16">Mermaid chart</text>`, width, height, width, height, width, height)
	for _, edge := range edges {
		from, to := positions[edge.From], positions[edge.To]
		fmt.Fprintf(&output, `<path d="M%d %d L%d %d" stroke="#83b6f4" stroke-width="2" fill="none" marker-end="url(#arrow)"/>`, from[0]+70, from[1]+26, to[0]-70, to[1]+26)
	}
	for _, node := range ordered {
		point := positions[node.ID]
		fmt.Fprintf(&output, `<rect x="%d" y="%d" width="140" height="52" rx="10" fill="#102847" stroke="#5d9ad6"/><text x="%d" y="%d" text-anchor="middle" dominant-baseline="middle" fill="#edf5ff" font-family="sans-serif" font-size="15">%s</text>`, point[0]-70, point[1], point[0], point[1]+26, html.EscapeString(node.Label))
	}
	output.WriteString(`</svg>`)
	return output.String()
}

func telegramMermaidSourceSVG(source string) string {
	lines := strings.Split(strings.TrimSpace(source), "\n")
	height := max(120, 70+len(lines)*22)
	var output strings.Builder
	fmt.Fprintf(&output, `<svg xmlns="http://www.w3.org/2000/svg" width="900" height="%d" viewBox="0 0 900 %d"><rect x="0" y="0" width="900" height="%d" fill="#071426"/><text x="20" y="30" fill="#d7e4f7" font-family="sans-serif" font-size="16">Mermaid source</text>`, height, height, height)
	for index, line := range lines {
		fmt.Fprintf(&output, `<text x="20" y="%d" fill="#d7e4f7" font-family="monospace" font-size="14">%s</text>`, 58+index*22, html.EscapeString(line))
	}
	output.WriteString(`</svg>`)
	return output.String()
}
