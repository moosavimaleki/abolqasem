import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { createMarkdownComponents, markdownComponents, OpenLocalLinkProvider } from "./shared"

describe("markdownComponents", () => {
  test("renders markdown headings with transcript-specific sizes and no bold weight", () => {
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {"# One\n## Two\n### Three\n#### Four\n##### Five\n###### Six"}
      </Markdown>
    )

    expect(html).toContain('<h1 dir="ltr" class="text-[20px] font-normal')
    expect(html).toContain('<h2 dir="ltr" class="text-[18px] font-normal')
    expect(html).toContain('<h3 dir="ltr" class="text-[16px] font-normal')
    expect(html).toContain('<h4 dir="ltr" class="text-[16px] font-normal')
    expect(html).toContain('<h5 dir="ltr" class="text-[16px] font-normal')
    expect(html).toContain('<h6 dir="ltr" class="text-[16px] font-normal')
  })

  test("renders markdown blockquotes with quote styling", () => {
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {"> quoted line"}
      </Markdown>
    )

    expect(html).toContain("<blockquote")
    expect(html).toContain("abolqasem-blockquote")
    expect(html).toContain("abolqasem-blockquote-mark")
    expect(html).toContain("abolqasem-blockquote-content")
    expect(html).toContain("<p")
    expect(html).toContain("quoted line")
  })

  test("preserves nested markdown inside blockquotes", () => {
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
        {"> [docs](https://example.com)\n> \n> - item"}
      </Markdown>
    )

    expect(html).toContain("<blockquote")
    expect(html).toContain("<a")
    expect(html).toContain("https://example.com")
    expect(html).toContain("<ul")
    expect(html).toContain("<li")
  })

  test("inherits assistant message RTL theme for short technical paragraphs", () => {
    const source = [
      "فایل ذخیره شد و فایل تجمیعی حالا ۹ تا Pack دارد.",
      "",
      "prod-mongodb.yaml",
      "",
      "all-packs.yaml حالا ۹ تا Pack دارد",
    ].join("\n")
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source })}>
        {source}
      </Markdown>
    )

    expect(html).toContain('<p dir="rtl"')
    expect(html).toMatch(/<p[^>]*dir="rtl"[^>]*>prod-mongodb\.yaml<\/p>/)
    expect(html).toMatch(/<p[^>]*dir="rtl"[^>]*>all-packs\.yaml حالا ۹ تا Pack دارد<\/p>/)
  })

  test("isolates technical inline code as LTR inside RTL messages", () => {
    const source = "فایل `prod-mongodb.yaml` ذخیره شد."
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source })}>
        {source}
      </Markdown>
    )

    expect(html).toContain('<p dir="rtl"')
    expect(html).toContain('<code dir="ltr"')
    expect(html).toContain("prod-mongodb.yaml")
  })

  test("allows non-code fenced Persian text to stay RTL", () => {
    const source = "توضیح:\n\n```\nبرای این پروژه یک Postgres می‌خوام\nاز نوع استاندارد شرکت\n```"
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source })}>
        {source}
      </Markdown>
    )

    expect(html).toContain('<pre class="')
    expect(html).toContain('dir="rtl"')
    expect(html).toContain("برای این پروژه")
  })

  test("keeps explicit code blocks LTR even in RTL messages", () => {
    const source = "چک کردم:\n\n```bash\nkubectl get pods\n```"
    const html = renderToStaticMarkup(
      <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source })}>
        {source}
      </Markdown>
    )

    expect(html).toContain("Shell")
    expect(html).toContain('dir="ltr"')
    expect(html).toContain("kubectl")
  })

  test("renders local file links without browser target handling", () => {
    const html = renderToStaticMarkup(
      <Markdown
        remarkPlugins={[remarkGfm]}
        components={createMarkdownComponents({ onOpenLocalLink: () => {} })}
      >
        {"[app.ts](/Users/jake/Projects/abolqasem/src/client/app/App.tsx#L1)"}
      </Markdown>
    )

    expect(html).toContain("/Users/jake/Projects/abolqasem/src/client/app/App.tsx#L1")
    expect(html).not.toContain('target="_blank"')
  })

  test("renders local file links without browser target handling when provided by context", () => {
    const html = renderToStaticMarkup(
      <OpenLocalLinkProvider onOpenLocalLink={() => {}}>
        <Markdown
          remarkPlugins={[remarkGfm]}
          components={createMarkdownComponents()}
        >
          {"[app.ts](/Users/jake/Projects/abolqasem/src/client/app/App.tsx#L1)"}
        </Markdown>
      </OpenLocalLinkProvider>
    )

    expect(html).toContain("/Users/jake/Projects/abolqasem/src/client/app/App.tsx#L1")
    expect(html).not.toContain('target="_blank"')
  })
})
