package render

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	markdownRenderer = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	htmlPolicy = func() *bluemonday.Policy {
		policy := bluemonday.UGCPolicy()
		policy.AllowElements("table", "thead", "tbody", "tr", "th", "td", "details", "summary")
		policy.AllowAttrs("class").OnElements("code", "pre")
		return policy
	}()
)

func MarkdownToHTML(source string) string {
	var buffer bytes.Buffer
	if err := markdownRenderer.Convert([]byte(source), &buffer); err != nil {
		return htmlPolicy.Sanitize(source)
	}
	return htmlPolicy.Sanitize(buffer.String())
}
