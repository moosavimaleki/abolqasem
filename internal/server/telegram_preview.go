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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
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
	previewedFiles := map[string]struct{}{}
	addFilePreview := func(path string, line int) bool {
		if len(buttons) >= telegramPreviewButtonLimit {
			return false
		}
		// A response frequently cites several lines of one file. One semantic
		// action per file is easier to use on mobile and prevents the first file
		// from consuming the entire RichBlockButtons allowance.
		if _, exists := previewedFiles[path]; exists {
			return false
		}
		token := b.rememberTelegramPreview(telegramPreviewItem{
			Kind: telegramPreviewFile, TelegramChatID: telegramChatID, WorkspaceChatID: workspaceChatID, FilePath: path, Line: line, CreatedAt: time.Now(),
		})
		if token == "" {
			return false
		}
		previewedFiles[path] = struct{}{}
		label := "📄 " + filepath.Base(path)
		if line > 0 {
			label += " · خط " + strconv.Itoa(line)
		}
		buttons = append(buttons, telegramPreviewButton{Label: label, Token: token})
		return true
	}
	markdown = b.telegramReplaceMermaidBlocks(telegramChatID, workspaceChatID, markdown, &buttons)
	markdown = telegramMarkdownFileLink.ReplaceAllStringFunc(markdown, func(raw string) string {
		parts := telegramMarkdownFileLink.FindStringSubmatch(raw)
		if len(parts) != 3 {
			return raw
		}
		path, line, ok := telegramResolvePreviewFilePath(workspaceChatID, parts[2])
		if !ok {
			return raw
		}
		_ = addFilePreview(path, line)
		return "`" + telegramMarkdownInline(parts[1]) + "`"
	})
	markdown = telegramPlainFileReference.ReplaceAllStringFunc(markdown, func(raw string) string {
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
		_ = addFilePreview(path, line)
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
		path, err = resolveTelegramPreviewPath(project.LocalPath, decoded)
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
		path, err = resolveTelegramPreviewPath(project.LocalPath, candidate)
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
	_, err = resolveTelegramPreviewPath(project.LocalPath, path)
	return err == nil
}

// resolveTelegramPreviewPath permits an explicitly selected workspace even
// when it is the home directory. The general file-preview API intentionally
// rejects home as a broad root; Telegram reaches this function only after the
// chat/project mapping was selected by an authorised user. Symlink resolution
// and strict containment still prevent an escape from that selected root.
func resolveTelegramPreviewPath(projectRoot, requestedPath string) (string, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil {
		return "", errFilePreviewForbidden
	}
	rootEval, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", errFilePreviewForbidden
	}
	targetPath := cleanRequestedPreviewPath(requestedPath)
	if targetPath == "" || !filepath.IsAbs(targetPath) {
		return "", errFilePreviewForbidden
	}
	targetEval, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errFilePreviewNotFound
		}
		return "", err
	}
	relative, err := filepath.Rel(rootEval, targetEval)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errFilePreviewForbidden
	}
	return targetEval, nil
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
		resolvedPath, resolveErr := resolveTelegramPreviewPath(project.LocalPath, item.FilePath)
		if resolveErr != nil {
			err = resolveErr
			break
		}
		preview, previewErr := buildFilePreviewResolved(resolvedPath, item.Line, filePreviewOptions{Full: true})
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
	highlightedLines := telegramHighlightCode(preview.Path, preview.Language, lines)
	width := max(720, min(1440, 84+maxColumns*8))
	height := 54 + len(lines)*22
	var output strings.Builder
	// Telegram's SVG viewer does not consistently paint percentage-sized rects.
	// Use explicit dimensions so the document is opaque instead of checkerboard.
	fmt.Fprintf(&output, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><rect x="0" y="0" width="%d" height="%d" fill="#0d1117"/><rect x="0" y="0" width="%d" height="42" fill="#161b22"/><circle cx="22" cy="21" r="5" fill="#ff7b72"/><circle cx="40" cy="21" r="5" fill="#d29922"/><circle cx="58" cy="21" r="5" fill="#3fb950"/><text x="82" y="26" fill="#c9d1d9" font-family="monospace" font-size="14">%s · %s</text>`, width, height, width, height, width, height, width, html.EscapeString(filepath.Base(preview.Path)), html.EscapeString(preview.Language))
	for index, line := range lines {
		y := 64 + index*22
		if line.Highlight {
			fmt.Fprintf(&output, `<rect x="0" y="%d" width="%d" height="22" fill="#1f3a5f"/>`, y-16, width)
		}
		fmt.Fprintf(&output, `<text x="16" y="%d" text-anchor="end" fill="#6e7681" font-family="monospace" font-size="13">%d</text>`, y, line.Number)
		fmt.Fprintf(&output, `<text x="34" y="%d" fill="#c9d1d9" font-family="monospace" font-size="14" xml:space="preserve">`, y)
		for _, span := range telegramTrimHighlightedLine(highlightedLines[index], 120) {
			attributes := ""
			if span.Bold {
				attributes += ` font-weight="600"`
			}
			if span.Italic {
				attributes += ` font-style="italic"`
			}
			fmt.Fprintf(&output, `<tspan fill="%s"%s>%s</tspan>`, span.Colour, attributes, html.EscapeString(span.Text))
		}
		output.WriteString(`</text>`)
	}
	output.WriteString(`</svg>`)
	return output.String()
}

type telegramCodeSpan struct {
	Text   string
	Colour string
	Bold   bool
	Italic bool
}

// telegramHighlightCode uses Chroma's Pygments-derived lexer registry instead
// of assigning one heuristic colour to an entire source line. Tokenising the
// whole file preserves multiline strings/comments, then the tokens are split
// back into SVG rows without embedding unsafe HTML.
func telegramHighlightCode(path, language string, source []filePreviewLine) [][]telegramCodeSpan {
	plain := func() [][]telegramCodeSpan {
		lines := make([][]telegramCodeSpan, len(source))
		for index, line := range source {
			lines[index] = []telegramCodeSpan{{Text: strings.ReplaceAll(line.Text, "\t", "    "), Colour: "#c9d1d9"}}
		}
		return lines
	}

	lexer := lexers.Match(path)
	if lexer == nil && language != "" {
		lexer = lexers.Get(language)
	}
	codeLines := make([]string, len(source))
	for index, line := range source {
		codeLines[index] = strings.ReplaceAll(line.Text, "\t", "    ")
	}
	code := strings.Join(codeLines, "\n")
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return plain()
	}
	style := styles.Get("github-dark")
	if style == nil {
		style = styles.Fallback
	}

	lines := make([][]telegramCodeSpan, 1, len(source))
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		colour := "#c9d1d9"
		if entry.Colour.IsSet() {
			colour = entry.Colour.String()
		}
		parts := strings.Split(token.Value, "\n")
		for index, part := range parts {
			if part != "" {
				lineIndex := len(lines) - 1
				lines[lineIndex] = append(lines[lineIndex], telegramCodeSpan{
					Text: part, Colour: colour, Bold: entry.Bold == chroma.Yes, Italic: entry.Italic == chroma.Yes,
				})
			}
			if index < len(parts)-1 {
				lines = append(lines, nil)
			}
		}
	}
	for len(lines) < len(source) {
		lines = append(lines, nil)
	}
	if len(lines) > len(source) {
		lines = lines[:len(source)]
	}
	return lines
}

func telegramTrimHighlightedLine(spans []telegramCodeSpan, maxRunes int) []telegramCodeSpan {
	totalRunes := 0
	for _, span := range spans {
		totalRunes += len([]rune(span.Text))
	}
	if totalRunes <= maxRunes {
		return spans
	}
	if maxRunes <= 0 {
		return nil
	}

	trimmed := make([]telegramCodeSpan, 0, len(spans)+1)
	remaining := maxRunes - 1 // Reserve the last column for the ellipsis.
	for _, span := range spans {
		if remaining <= 0 {
			break
		}
		runes := []rune(span.Text)
		if len(runes) <= remaining {
			trimmed = append(trimmed, span)
			remaining -= len(runes)
			continue
		}
		span.Text = string(runes[:remaining])
		if span.Text != "" {
			trimmed = append(trimmed, span)
		}
		break
	}
	trimmed = append(trimmed, telegramCodeSpan{Text: "…", Colour: "#8b949e"})
	return trimmed
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
