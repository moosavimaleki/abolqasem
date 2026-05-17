import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { Check, Copy, X } from "lucide-react"
import { useCallback, useMemo, useState, type ComponentPropsWithoutRef, type ReactNode } from "react"
import { Button } from "../ui/button"
import { cn } from "../../lib/utils"
import { useI18n } from "../../i18n/context"
import {
  getAppearanceArticleClassName,
  getAppearanceHeaderClassName,
  getAppearanceRootClassName,
  getAppearanceTextStyle,
  ReaderAppearancePopover,
  useReaderAppearanceSettings,
} from "../appearance/ReaderAppearance"
import { copyTextToClipboard, createMarkdownComponents } from "./shared"

function labels(isPersian: boolean) {
  return {
    reader: isPersian ? "حالت مطالعه" : "Reader",
    toc: isPersian ? "فهرست" : "Contents",
  }
}

interface ReaderHeading {
  id: string
  level: number
  text: string
}

function plainTextFromNode(node: ReactNode): string {
  if (node === null || node === undefined || typeof node === "boolean") return ""
  if (typeof node === "string" || typeof node === "number") return String(node)
  if (Array.isArray(node)) return node.map(plainTextFromNode).join("")
  if (typeof node === "object" && "props" in node) {
    const props = node.props as { children?: ReactNode }
    return plainTextFromNode(props.children)
  }
  return ""
}

function stripHeadingMarkup(value: string) {
  return value
    .replace(/`([^`]+)`/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/[*_~]/g, "")
    .trim()
}

function slugifyHeading(value: string) {
  const slug = stripHeadingMarkup(value)
    .toLocaleLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, "-")
    .replace(/^-+|-+$/g, "")
  return slug || "section"
}

function uniqueHeadingId(text: string, counts: Map<string, number>) {
  const base = slugifyHeading(text)
  const nextCount = (counts.get(base) ?? 0) + 1
  counts.set(base, nextCount)
  return nextCount === 1 ? base : `${base}-${nextCount}`
}

function extractReaderHeadings(markdown: string): ReaderHeading[] {
  const counts = new Map<string, number>()
  const headings: ReaderHeading[] = []
  let inFence = false

  for (const line of markdown.split(/\r?\n/)) {
    if (/^\s*(```|~~~)/.test(line)) {
      inFence = !inFence
      continue
    }
    if (inFence) continue

    const match = /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line)
    if (!match) continue

    const text = stripHeadingMarkup(match[2] ?? "")
    if (!text) continue
    headings.push({
      id: uniqueHeadingId(text, counts),
      level: match[1]?.length ?? 1,
      text,
    })
  }

  return headings
}

export function ReaderView({
  title,
  subtitle,
  content,
  className,
  onClose,
  variant = "page",
  hideHeader = false,
}: {
  title: string
  subtitle?: string
  content: string
  className?: string
  onClose?: () => void
  variant?: "page" | "dialog"
  hideHeader?: boolean
}) {
  const { locale, direction, t } = useI18n()
  const documentDirection = typeof document !== "undefined" ? document.documentElement.dir : ""
  const isPersian = locale === "fa" || direction === "rtl" || documentDirection === "rtl"
  const text = labels(isPersian)
  const copyLabel = isPersian ? "کپی کل متن" : "Copy reader text"
  const [settings] = useReaderAppearanceSettings()
  const [copied, setCopied] = useState(false)
  const articleStyle = getAppearanceTextStyle(settings)
  const articleClassName = getAppearanceArticleClassName(settings)
  const rootThemeClassName = getAppearanceRootClassName(settings)
  const headerClassName = getAppearanceHeaderClassName()
  const headings = useMemo(() => extractReaderHeadings(content), [content])
  const showToc = variant === "page" && headings.length > 1
  const renderHeadingCounts = new Map<string, number>()
  const headingId = (children: ReactNode) => uniqueHeadingId(plainTextFromNode(children), renderHeadingCounts)
  const markdownComponents = {
    ...createMarkdownComponents({ source: content }),
    h1: ({ children, ...props }: ComponentPropsWithoutRef<"h1">) => (
      <h1 id={headingId(children)} dir="auto" className="scroll-mt-20 text-[20px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</h1>
    ),
    h2: ({ children, ...props }: ComponentPropsWithoutRef<"h2">) => (
      <h2 id={headingId(children)} dir="auto" className="scroll-mt-20 text-[18px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</h2>
    ),
    h3: ({ children, ...props }: ComponentPropsWithoutRef<"h3">) => (
      <h3 id={headingId(children)} dir="auto" className="scroll-mt-20 text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</h3>
    ),
    h4: ({ children, ...props }: ComponentPropsWithoutRef<"h4">) => (
      <h4 id={headingId(children)} dir="auto" className="scroll-mt-20 text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</h4>
    ),
    h5: ({ children, ...props }: ComponentPropsWithoutRef<"h5">) => (
      <h5 id={headingId(children)} dir="auto" className="scroll-mt-20 text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</h5>
    ),
    h6: ({ children, ...props }: ComponentPropsWithoutRef<"h6">) => (
      <h6 id={headingId(children)} dir="auto" className="scroll-mt-20 text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</h6>
    ),
  }

  function scrollToHeading(id: string) {
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" })
  }

  const handleCopy = useCallback(async () => {
    if (!content) return
    await copyTextToClipboard(content)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }, [content])

  return (
    <div className={cn(rootThemeClassName, className)}>
      {!hideHeader ? (
        <header className={headerClassName}>
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">{title}</div>
            <div className="mt-0.5 flex min-w-0 items-center gap-2 text-[11px] uppercase tracking-[0.16em] text-muted-foreground">
              <span className="shrink-0">{text.reader}</span>
              {subtitle ? (
                <>
                  <span aria-hidden="true">/</span>
                  <span className="truncate normal-case tracking-normal">{subtitle}</span>
                </>
              ) : null}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <ReaderAppearancePopover title={title} />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className={cn(
                "h-9 w-9 shrink-0 rounded-full text-muted-foreground hover:text-foreground disabled:hidden",
                copied && "text-emerald-500 hover:text-emerald-500"
              )}
              aria-label={copied ? t.common.copied : copyLabel}
              title={copied ? t.common.copied : copyLabel}
              disabled={!content}
              onClick={handleCopy}
            >
              {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            </Button>
            {onClose ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-9 w-9 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
                aria-label={isPersian ? "بستن حالت مطالعه" : "Close reader"}
                onClick={onClose}
              >
                <X className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
        </header>
      ) : null}

      <div className={cn("min-h-0 flex-1 overflow-y-auto", variant === "dialog" && "reader-dialog-scroll")}>
        <div className={cn("mx-auto grid w-full min-w-0 gap-8", showToc ? "xl:max-w-[1180px] xl:grid-cols-[minmax(0,1fr)_240px]" : "")}>
          <article className={cn("mx-auto", articleClassName)} style={articleStyle}>
            <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
              {content}
            </Markdown>
          </article>
          {showToc ? (
            <aside className="hidden min-w-0 py-10 pe-5 xl:block" dir={direction}>
              <nav className="sticky top-5 max-h-[calc(100dvh-7rem)] overflow-auto rounded-3xl border border-border bg-card/55 p-3 text-sm shadow-xl shadow-background/15 backdrop-blur">
                <div className="mb-2 px-2 text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">{text.toc}</div>
                <div className="space-y-0.5">
                  {headings.map((heading) => (
                    <button
                      key={heading.id}
                      type="button"
                      className="block w-full truncate rounded-xl px-2 py-1.5 text-start text-xs leading-5 text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
                      style={{ paddingInlineStart: `${8 + Math.max(0, heading.level - 1) * 10}px` }}
                      title={heading.text}
                      onClick={() => scrollToHeading(heading.id)}
                    >
                      {heading.text}
                    </button>
                  ))}
                </div>
              </nav>
            </aside>
          ) : null}
        </div>
      </div>
    </div>
  )
}
