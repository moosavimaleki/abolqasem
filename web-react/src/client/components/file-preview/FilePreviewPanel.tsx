import { ExternalLink, X } from "lucide-react"
import { Button } from "../ui/button"
import { CodeFrame } from "../messages/shared"
import { ReaderView } from "../messages/ReaderView"
import { cn } from "../../lib/utils"

export interface FilePreviewLine {
  number: number
  text: string
  highlight: boolean
}

export interface FilePreviewResponse {
  path: string
  line: number
  full: boolean
  start_line: number
  end_line: number
  language: string
  html?: string
  lines: FilePreviewLine[]
}

export function filePreviewSource(preview: FilePreviewResponse) {
  return preview.lines.map((line) => line.text).join("\n")
}

export function fileRouteHref(path: string, line?: number) {
  const encodedPath = path.split("/").map((part, index) => index === 0 ? part : encodeURIComponent(part)).join("/")
  return `${encodedPath}${line && line > 0 ? `:${line}` : ""}`
}

export function FilePreviewPanel({
  preview,
  title,
  compact = false,
  showOpenRoute = false,
  onClose,
  hideHeader = false,
  surface = "card",
  bodyClassName,
  codeFrameClassName,
  className,
}: {
  preview: FilePreviewResponse
  title?: string
  compact?: boolean
  showOpenRoute?: boolean
  onClose?: () => void
  hideHeader?: boolean
  surface?: "card" | "bare"
  bodyClassName?: string
  codeFrameClassName?: string
  className?: string
}) {
  const source = filePreviewSource(preview)
  const relativeHighlight = preview.line > 0 ? preview.line - preview.start_line + 1 : undefined
  const isMarkdown = preview.language === "markdown"
  const titleLabel = title ?? preview.path.split(/[\\/]/).pop() ?? preview.path
  const subtitle = `${preview.path}${preview.line > 0 ? `:${preview.line}` : ""}`

  if (isMarkdown && !compact) {
    return (
      <ReaderView
        title={titleLabel}
        subtitle={subtitle}
        content={source}
        className={cn("h-full", className)}
        onClose={hideHeader ? undefined : onClose}
        hideHeader={hideHeader}
      />
    )
  }

  return (
    <div className={cn(
      "flex min-h-0 flex-col overflow-hidden",
      surface === "card" ? "rounded-3xl border border-border bg-background" : "bg-transparent",
      className,
    )}>
      {!hideHeader ? (
      <div className="flex items-center justify-between gap-4 border-b border-border px-4 py-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium text-foreground">{titleLabel}</div>
          <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
            {subtitle}
          </div>
        </div>
        {showOpenRoute || onClose ? (
          <div className="flex shrink-0 items-center gap-1">
            {showOpenRoute ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 rounded-xl text-muted-foreground hover:text-foreground"
                aria-label="Open file route"
                title="Open file route"
                onClick={() => window.open(fileRouteHref(preview.path, preview.line), "_blank", "noopener,noreferrer")}
              >
                <ExternalLink className="h-4 w-4" />
              </Button>
            ) : null}
            {onClose ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 rounded-xl text-muted-foreground hover:text-foreground"
                aria-label="Close preview"
                title="Close preview"
                onClick={onClose}
              >
                <X className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      ) : null}
      <div className={cn(
        "min-h-0",
        compact ? "max-h-[72vh] overflow-auto p-4" : "flex-1 overflow-hidden",
        surface === "card" ? "p-4" : "p-0",
        bodyClassName,
      )}>
        <CodeFrame
          source={source}
          language={preview.language}
          highlightLine={relativeHighlight}
          fill={!compact}
          className={cn(!compact && "h-full", codeFrameClassName)}
        />
      </div>
    </div>
  )
}
