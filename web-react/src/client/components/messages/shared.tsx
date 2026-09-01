import {
  Children,
  cloneElement,
  createContext,
  isValidElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type ReactNode,
} from "react"
import hljs from "highlight.js/lib/common"
import "highlight.js/styles/github-dark.css"
import { Button } from "../ui/button"
import {
  ArrowDownToLine,
  CheckLine,
  ChevronRight,
  ListTodo,
  Map,
  MessageCircleQuestion,
  Pencil,
  Search,
  Sparkles,
  SquareX,
  Terminal,
  ToyBrick,
  Plug,
  type LucideIcon,
  File,
  FilePen,
  FilePlusCorner,
  FileX,
  Copy,
  Check,
  Maximize2,
  Minimize2,
  RotateCcw,
  Quote,
  ZoomIn,
  ZoomOut,
} from "lucide-react"
import { cn } from "../../lib/utils"
import { parseLocalFileLink } from "../../lib/pathUtils"
import { useTranscriptRenderOptions } from "./render-context"

export type OpenLocalLinkTarget = {
  path: string
  line?: number
  column?: number
  clientX?: number
  clientY?: number
  trigger?: "click" | "contextmenu"
}
type OpenLocalLinkHandler = (target: OpenLocalLinkTarget) => void

const defaultOpenLocalLink: OpenLocalLinkHandler = () => {}

const OpenLocalLinkContext = createContext<OpenLocalLinkHandler>(defaultOpenLocalLink)

export function OpenLocalLinkProvider({
  children,
  onOpenLocalLink,
}: {
  children: ReactNode
  onOpenLocalLink?: OpenLocalLinkHandler
}) {
  return (
    <OpenLocalLinkContext.Provider value={onOpenLocalLink ?? defaultOpenLocalLink}>
      {children}
    </OpenLocalLinkContext.Provider>
  )
}

// Tool icon mapping - shared between ToolCallMessage and SystemMessage
export const toolIcons: Record<string, LucideIcon> = {
  Task: ListTodo,
  TaskOutput: ListTodo,
  Bash: Terminal,
  Glob: Search,
  Grep: Search,
  ExitPlanMode: Map,
  Read: File,
  Edit: FilePen,
  Write: FilePlusCorner,
  Delete: FileX,
  NotebookEdit: Pencil,
  WebFetch: ArrowDownToLine,
  TodoWrite: CheckLine,
  WebSearch: Search,
  KillShell: SquareX,
  AskUserQuestion: MessageCircleQuestion,
  Skill: Sparkles,
  EnterPlanMode: Map,
}

export const defaultToolIcon: LucideIcon = ToyBrick

// Get icon for a tool.
export function getToolIcon(toolName: string): LucideIcon {
  if (toolName.startsWith("mcp__")) {
    return Plug
  }
  if (toolIcons[toolName]) {
    return toolIcons[toolName]
  }
  return defaultToolIcon
}

// Container for meta-style messages (system, tool, result)
export function MetaRow({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("flex gap-3 justify-start items-center", className)}>
      {children}
    </div>
  )
}

// Content row with consistent text styling
export function MetaContent({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("flex items-center gap-1.5 text-xs", className)}>
      {children}
    </div>
  )
}

// Separator pipe
export function MetaSeparator() {
  return <span className="text-muted-foreground">|</span>
}

// Bold label text
export function MetaLabel({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn("font-medium text-foreground/80", className)}>{children}</span>
}

// Muted text
export function MetaText({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>
}

// Expandable row with chevron
interface ExpandableRowProps {
  children: ReactNode
  expandedContent: ReactNode
  defaultExpanded?: boolean
}

export function ExpandableRow({ children, expandedContent, defaultExpanded = false }: ExpandableRowProps) {
  const [expanded, setExpanded] = useState(defaultExpanded)

  return (
    <div className="flex flex-col w-full">

      <button
        onClick={() => setExpanded(!expanded)}
        className={`group/expandable-row cursor-pointer grid grid-cols-[auto_1fr] items-center gap-1 text-sm ${!expanded ? "hover:opacity-60 transition-opacity" : ""}`}
      >
        <div className="grid grid-cols-[auto_1fr] items-center gap-1.5">
          {children}
        </div>
        <ChevronRight
          className={`h-4.5 w-4.5 text-muted-icon translate-y-[0.5px] transition-all duration-200 opacity-0 group-hover/expandable-row:opacity-100 ${expanded ? "rotate-90 opacity-100" : ""}`}
        />
      </button>
      {expanded && expandedContent}
    </div>
  )
}

// Code block for expanded content
export function MetaCodeBlock({ label, children, copyText }: { label: ReactNode; children: ReactNode; copyText?: string }) {
  const [copied, setCopied] = useState(false)
  const textContent = copyText ?? extractText(children)

  const handleCopy = useCallback(async () => {
    await navigator.clipboard.writeText(textContent)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }, [textContent])

  return (
    <div>
      <span className="font-medium text-muted-foreground">{label}</span>
      <div className="relative group/codeblock">
        <pre className="my-1 text-xs font-mono whitespace-no-wrap break-all bg-muted border border-border  rounded-lg p-2 max-h-64 overflow-auto w-full">
          {children}
        </pre>
        <Button
          variant="ghost"
          size="icon"
          className={cn(
            "absolute top-[4px] right-[4px] z-10 h-6.5 w-6.5 rounded-sm text-muted-foreground opacity-0 group-hover/codeblock:opacity-100 transition-opacity",
            !copied && "hover:text-foreground",
            copied && "hover:!bg-transparent hover:!border-transparent"
          )}
          onClick={handleCopy}
        >
          {copied ? <Check className="h-3.5 w-3.5 text-green-400" /> : <Copy className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  )
}

export async function copyTextToClipboard(text: string) {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch {
      // Fall back to a temporary textarea for browser contexts where Clipboard
      // API exists but is blocked by permissions or the current document state.
    }
  }

  if (typeof document === "undefined") {
    return Promise.reject(new Error("Clipboard is not available"))
  }

  const textarea = document.createElement("textarea")
  textarea.value = text
  textarea.setAttribute("readonly", "")
  textarea.style.position = "fixed"
  textarea.style.opacity = "0"
  document.body.appendChild(textarea)
  textarea.select()
  const copied = document.execCommand("copy")
  document.body.removeChild(textarea)
  if (!copied) {
    throw new Error("Clipboard copy failed")
  }
}

export function MessageCopyButton({
  text,
  label,
  copiedLabel,
  className,
}: {
  text: string
  label: string
  copiedLabel: string
  className?: string
}) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(async () => {
    if (!text) return
    await copyTextToClipboard(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }, [text])

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={copied ? copiedLabel : label}
      title={copied ? copiedLabel : label}
      disabled={!text}
      onClick={handleCopy}
      className={cn(
        "h-7 w-7 rounded-full border-border/0 bg-background/80 text-muted-foreground opacity-0 shadow-sm backdrop-blur-sm transition-all hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover/message:opacity-100 group-focus-within/message:opacity-100 disabled:hidden",
        copied && "text-emerald-500 hover:text-emerald-500",
        className
      )}
    >
      {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
    </Button>
  )
}

// Pill/badge for tags
export function MetaPill({ children, icon: Icon, className }: { children: ReactNode; icon?: LucideIcon; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1 px-2 py-1 bg-muted border border-border  rounded-full", className)}>
      {Icon && <Icon className="h-3 w-3 text-muted-foreground" />}
      {children}
    </span>
  )
}

// Container with vertical line on the left
export function VerticalLineContainer({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("grid grid-cols-[auto_1fr] gap-2 min-w-0", className)}>
      <div className=" min-w-5 flex flex-col relative items-center justify-center">
        <div className="min-h-full w-[1px] bg-muted-foreground/20" />
      </div>
      <div className="-ml-[1px] min-w-0 overflow-hidden">
        {children}
      </div>
    </div>
  )
}

// Helper function to extract text content from ReactNode
function extractText(node: ReactNode): string {
  if (typeof node === "string") {
    return node
  }
  if (typeof node === "number") {
    return String(node)
  }
  if (Array.isArray(node)) {
    return node.map(extractText).join("")
  }
  if (node && typeof node === "object" && "props" in node) {
    const props = node.props as { children?: ReactNode }
    return extractText(props.children)
  }
  return ""
}

type MarkdownChildNode = ReturnType<typeof Children.toArray>[number]

function withChildClassName(node: MarkdownChildNode, className: string): MarkdownChildNode {
  if (!isValidElement<{ className?: string }>(node)) {
    return node
  }

  return cloneElement(node, {
    className: cn(node.props.className, className),
  })
}

type TextDirection = "ltr" | "rtl"

interface DirectionStats {
  direction?: TextDirection
  rtlCount: number
  latinCount: number
  total: number
  confidence: number
}

const rtlCharacterPattern = /[\u0590-\u05ff\u0600-\u06ff\u0750-\u077f\u08a0-\u08ff]/
const latinCharacterPattern = /[A-Za-z]/
const technicalTokenPattern = new RegExp([
  String.raw`https?:\/\/[^\s)]+`,
  String.raw`(?:^|\s)(?:[A-Za-z]:[\\/]|\/|~\/|\.{1,2}\/)[^\s]+`,
  String.raw`\b[\w.-]+\.(?:bash|conf|css|csv|go|html|ini|js|json|jsx|lock|log|md|mod|ps1|py|sh|sql|sum|toml|ts|tsx|tsv|txt|xml|ya?ml|zsh)\b`,
  String.raw`\b[A-Fa-f0-9]{7,40}\b`,
  String.raw`\bv?\d+(?:\.\d+){1,4}(?:[-+][\w.-]+)?\b`,
  String.raw`\b[A-Za-z0-9]+(?:[-_./][A-Za-z0-9]+){1,}\b`,
  String.raw`--?[A-Za-z][\w-]*`,
].join("|"), "g")

function stripMarkdownThemeExclusions(text: string) {
  return text
    .replace(/```[\s\S]*?```/g, "\n")
    .replace(/~~~[\s\S]*?~~~/g, "\n")
    .replace(/`[^`\n]*`/g, " ")
    .split(/\n/)
    .filter((line) => !/^\s*>/.test(line))
    .join("\n")
}

function stripTechnicalDirectionTokens(text: string) {
  return text.replace(technicalTokenPattern, " ")
}

function directionStats(text: string): DirectionStats {
  const normalized = stripTechnicalDirectionTokens(text).trim()
  if (!normalized) {
    return { rtlCount: 0, latinCount: 0, total: 0, confidence: 0 }
  }

  let rtlCount = 0
  let latinCount = 0
  for (const char of normalized) {
    if (rtlCharacterPattern.test(char)) {
      rtlCount += 1
    } else if (latinCharacterPattern.test(char)) {
      latinCount += 1
    }
  }

  const total = rtlCount + latinCount
  if (total === 0) {
    return { rtlCount, latinCount, total, confidence: 0 }
  }

  const rtlRatio = rtlCount / total
  const latinRatio = latinCount / total
  if (rtlCount >= 2 && rtlRatio >= 0.22) {
    return { direction: "rtl", rtlCount, latinCount, total, confidence: rtlRatio }
  }
  if (latinCount >= 4 && latinRatio >= 0.62) {
    return { direction: "ltr", rtlCount, latinCount, total, confidence: latinRatio }
  }
  return { rtlCount, latinCount, total, confidence: Math.max(rtlRatio, latinRatio) }
}

function getTextDirection(text: string) {
  return directionStats(text).direction
}

function getMarkdownMessageDirection(source: string | undefined): TextDirection | undefined {
  if (!source) return undefined
  return directionStats(stripMarkdownThemeExclusions(source)).direction
}

function textDirectionFromChildren(children: ReactNode, inheritedDirection?: TextDirection) {
  return inheritedDirection ?? getTextDirection(extractText(children)) ?? "ltr"
}

function exceptionalBlockDirectionFromChildren(children: ReactNode, inheritedDirection?: TextDirection) {
  const text = extractText(children)
  if (!inheritedDirection) {
    return getTextDirection(text) ?? "ltr"
  }
  const stats = directionStats(text)
  if (
    stats.direction &&
    stats.direction !== inheritedDirection &&
    stats.total >= 12 &&
    stats.confidence >= 0.7
  ) {
    return stats.direction
  }
  return inheritedDirection
}

function isExplicitCodeLanguage(language: string | undefined) {
  if (!language) return false
  return !["md", "markdown", "plain", "plaintext", "text", "txt"].includes(language.toLowerCase())
}

function looksLikeCodeBlock(source: string, language: string | undefined) {
  if (isExplicitCodeLanguage(language)) return true
  const trimmed = source.trim()
  if (!trimmed) return false
  const lines = trimmed.split(/\n/)
  const codeLineCount = lines.filter((line) => (
    /^\s*(?:[$>#]\s*)?(?:bun|cargo|curl|deno|docker|git|go|helm|kubectl|make|node|npm|pnpm|python\d*|sudo|uv|yarn)\b/.test(line) ||
    /[{}[\]();=<>|$]/.test(line) ||
    /^\s*(?:const|def|export|from|func|function|import|let|package|return|var)\b/.test(line)
  )).length
  const symbolCount = (trimmed.match(/[{}[\]();=<>|$]/g) ?? []).length
  const strongCount = Array.from(trimmed).filter((char) => rtlCharacterPattern.test(char) || latinCharacterPattern.test(char)).length
  return codeLineCount >= Math.max(1, Math.ceil(lines.length * 0.45)) || symbolCount / Math.max(1, strongCount) >= 0.08
}

function codeBlockDirection(source: string, language: string | undefined, inheritedDirection?: TextDirection): TextDirection {
  if (looksLikeCodeBlock(source, language)) return "ltr"
  return getTextDirection(source) ?? inheritedDirection ?? "ltr"
}

function isTechnicalInlineCode(text: string) {
  const trimmed = text.trim()
  if (!trimmed) return false
  if (/^https?:\/\//.test(trimmed)) return true
  if (/^(?:[A-Za-z]:[\\/]|\/|~\/|\.{1,2}\/)/.test(trimmed)) return true
  if (/^[\w.-]+\.[A-Za-z0-9]{1,8}$/.test(trimmed)) return true
  if (/^[A-Za-z0-9]+(?:[-_./][A-Za-z0-9]+)+$/.test(trimmed)) return true
  if (/^--?[A-Za-z][\w-]*$/.test(trimmed)) return true
  return false
}

function inlineCodeDirection(children: ReactNode, inheritedDirection?: TextDirection): TextDirection {
  const text = extractText(children)
  if (isTechnicalInlineCode(text)) return "ltr"
  return getTextDirection(text) ?? inheritedDirection ?? "ltr"
}

function displayLanguage(language: string) {
  if (!language) return "text"
  const normalized = language.toLowerCase()
  const labels: Record<string, string> = {
    bash: "Shell",
    shell: "Shell",
    sh: "Shell",
    zsh: "Shell",
    js: "JavaScript",
    javascript: "JavaScript",
    ts: "TypeScript",
    typescript: "TypeScript",
    jsx: "JSX",
    tsx: "TSX",
    md: "Markdown",
    markdown: "Markdown",
    py: "Python",
    python: "Python",
    go: "Go",
    json: "JSON",
    yaml: "YAML",
    yml: "YAML",
  }
  return labels[normalized] ?? normalized.toUpperCase()
}

function languageFromClassName(className: string | undefined) {
  const match = className?.match(/(?:^|\s)language-([a-z0-9_+-]+)(?:\s|$)/i)
  return match?.[1]?.toLowerCase() ?? ""
}

function markdownCodePayload(children: ReactNode) {
  const child = Children.toArray(children).find((entry) => isValidElement<{ className?: string }>(entry))
  const className = isValidElement<{ className?: string }>(child) ? child.props.className : undefined
  return {
    language: languageFromClassName(className),
    source: extractText(children).replace(/\n$/, ""),
  }
}

export function CodeFrame({
  source,
  language,
  className,
  highlightLine,
  direction: directionOverride,
  fill = false,
}: {
  source: string
  language?: string
  className?: string
  highlightLine?: number
  direction?: TextDirection
  fill?: boolean
}) {
  const [copied, setCopied] = useState(false)
  const direction = directionOverride ?? codeBlockDirection(source, language)
  const highlighted = useMemo(() => {
    if (!source.trim()) return ""
    try {
      if (language && hljs.getLanguage(language)) {
        return hljs.highlight(source, { language, ignoreIllegals: true }).value
      }
      return hljs.highlightAuto(source).value
    } catch {
      return source
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
    }
  }, [language, source])

  const handleCopy = useCallback(async () => {
    await navigator.clipboard.writeText(source)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1400)
  }, [source])

  const lines = highlighted.split(/\n/)

  return (
    <div className={cn("code-frame group/code-frame overflow-hidden rounded-2xl border border-border bg-card/70", fill && "flex h-full min-h-0 flex-col", className)}>
      <div className="flex min-h-9 items-center justify-between gap-3 border-b border-border bg-muted/35 px-3 py-1.5">
        <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          {displayLanguage(language ?? "")}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 rounded-lg text-muted-foreground hover:text-foreground"
          aria-label="Copy code"
          onClick={handleCopy}
        >
          {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
        </Button>
      </div>
      <pre
        className={cn(
          "m-0 overflow-auto bg-[hsl(var(--syntax-bg))] p-0 text-[12px] leading-5 text-[hsl(var(--syntax-fg))]",
          fill ? "min-h-0 flex-1" : "max-h-[70vh]",
          direction === "rtl" ? "text-right" : "text-left",
        )}
        dir={direction}
      >
        <code className="block min-w-max py-3">
          {lines.map((line, index) => {
            const lineNumber = index + 1
            return (
              <span
                key={lineNumber}
                className={cn(
                  "block px-4",
                  highlightLine === lineNumber && "code-line-highlight",
                )}
                data-line={lineNumber}
                dangerouslySetInnerHTML={{ __html: line || " " }}
              />
            )
          })}
        </code>
      </pre>
    </div>
  )
}

export function MermaidFrame({ source, className }: { source: string; className?: string }) {
  const frameRef = useRef<HTMLDivElement | null>(null)
  const [svg, setSvg] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [scale, setScale] = useState(1)
  const [offset, setOffset] = useState({ x: 0, y: 0 })
  const [fullscreen, setFullscreen] = useState(false)
  const dragRef = useRef<{ pointerId: number; x: number; y: number; startX: number; startY: number } | null>(null)

  useEffect(() => {
    let cancelled = false
    const id = `mermaid-${Math.random().toString(36).slice(2)}`
    setError(null)
    setSvg("")
    import("mermaid")
      .then((module) => {
        const mermaid = module.default
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: "strict",
          theme: "dark",
        })
        return mermaid.render(id, source)
      })
      .then((result) => {
        if (!cancelled) setSvg(result.svg)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Mermaid render failed")
      })
    return () => {
      cancelled = true
    }
  }, [source])

  const handleCopy = useCallback(async () => {
    await navigator.clipboard.writeText(source)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1400)
  }, [source])

  const reset = useCallback(() => {
    setScale(1)
    setOffset({ x: 0, y: 0 })
  }, [])

  const zoomBy = useCallback((factor: number) => {
    setScale((current) => Math.max(0.25, Math.min(5, current * factor)))
  }, [])

  useEffect(() => {
    const handleFullscreenChange = () => {
      setFullscreen(document.fullscreenElement === frameRef.current)
      dragRef.current = null
    }

    document.addEventListener("fullscreenchange", handleFullscreenChange)
    return () => {
      document.removeEventListener("fullscreenchange", handleFullscreenChange)
    }
  }, [])

  const toggleFullscreen = useCallback(() => {
    const frame = frameRef.current
    if (!frame) return
    if (document.fullscreenElement === frame) {
      void document.exitFullscreen()
      return
    }
    void frame.requestFullscreen?.()
  }, [])

  return (
    <div
      ref={frameRef}
      className={cn("mermaid-frame group/mermaid flex flex-col overflow-hidden rounded-2xl border border-border bg-card/70", className)}
      data-fullscreen={fullscreen ? "true" : "false"}
    >
      <div className="mermaid-toolbar flex min-h-9 shrink-0 items-center justify-between gap-3 border-b border-border bg-muted/35 px-3 py-1.5">
        <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">Mermaid</span>
        <div className="flex items-center gap-1">
          <Button type="button" variant="ghost" size="icon" className="h-7 w-7 rounded-lg text-muted-foreground hover:text-foreground" onClick={handleCopy} aria-label="Copy Mermaid">
            {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
          </Button>
          <Button type="button" variant="ghost" size="icon" className="h-7 w-7 rounded-lg text-muted-foreground hover:text-foreground" onClick={() => zoomBy(0.82)} aria-label="Zoom out">
            <ZoomOut className="h-3.5 w-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="icon" className="h-7 w-7 rounded-lg text-muted-foreground hover:text-foreground" onClick={reset} aria-label="Reset diagram">
            <RotateCcw className="h-3.5 w-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="icon" className="h-7 w-7 rounded-lg text-muted-foreground hover:text-foreground" onClick={() => zoomBy(1.18)} aria-label="Zoom in">
            <ZoomIn className="h-3.5 w-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="icon" className="h-7 w-7 rounded-lg text-muted-foreground hover:text-foreground" onClick={toggleFullscreen} aria-label="Fullscreen diagram">
            {fullscreen ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
          </Button>
        </div>
      </div>
      <div
        className={cn("mermaid-canvas min-h-24 cursor-grab overflow-auto bg-background/70 p-5 active:cursor-grabbing", error && "cursor-auto")}
        onWheel={(event) => {
          if (!svg) return
          event.preventDefault()
          zoomBy(Math.exp(-event.deltaY * 0.001))
        }}
        onPointerDown={(event) => {
          if (!svg) return
          dragRef.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, startX: offset.x, startY: offset.y }
          event.currentTarget.setPointerCapture(event.pointerId)
        }}
        onPointerMove={(event) => {
          const drag = dragRef.current
          if (!drag || drag.pointerId !== event.pointerId) return
          setOffset({
            x: drag.startX + event.clientX - drag.x,
            y: drag.startY + event.clientY - drag.y,
          })
        }}
        onPointerUp={(event) => {
          if (dragRef.current?.pointerId === event.pointerId) {
            dragRef.current = null
          }
        }}
        onPointerCancel={() => {
          dragRef.current = null
        }}
      >
        {error ? (
          <div className="space-y-3">
            <p className="text-sm text-destructive">{error}</p>
            <CodeFrame source={source} language="mermaid" />
          </div>
        ) : svg ? (
          <div
            className="mermaid-svg-shell mx-auto w-fit origin-center transition-transform duration-100 [&_svg]:max-w-none"
            style={{ transform: `translate(${offset.x}px, ${offset.y}px) scale(${scale})` }}
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        ) : (
          <div className="flex min-h-24 items-center justify-center text-sm text-muted-foreground">Rendering Mermaid…</div>
        )}
      </div>
    </div>
  )
}

// Markdown component overrides
function createBaseMarkdownComponents(inheritedDirection?: TextDirection) {
  return {
  h1: ({ children }: { children?: ReactNode }) => (
    <h1 dir={textDirectionFromChildren(children, inheritedDirection)} className="text-[20px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0">{children}</h1>
  ),
  h2: ({ children }: { children?: ReactNode }) => (
    <h2 dir={textDirectionFromChildren(children, inheritedDirection)} className="text-[18px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0">{children}</h2>
  ),
  h3: ({ children }: { children?: ReactNode }) => (
    <h3 dir={textDirectionFromChildren(children, inheritedDirection)} className="text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0">{children}</h3>
  ),
  h4: ({ children }: { children?: ReactNode }) => (
    <h4 dir={textDirectionFromChildren(children, inheritedDirection)} className="text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0">{children}</h4>
  ),
  h5: ({ children }: { children?: ReactNode }) => (
    <h5 dir={textDirectionFromChildren(children, inheritedDirection)} className="text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0">{children}</h5>
  ),
  h6: ({ children }: { children?: ReactNode }) => (
    <h6 dir={textDirectionFromChildren(children, inheritedDirection)} className="text-[16px] font-normal leading-tight mt-5 mb-3 first:mt-0 last:mb-0">{children}</h6>
  ),

  pre: ({ children }: ComponentPropsWithoutRef<"pre">) => {
    const payload = markdownCodePayload(children)
    if (payload.language === "mermaid") {
      return <MermaidFrame source={payload.source} className="my-5 first:mt-0 last:mb-0" />
    }
    return <CodeFrame source={payload.source} language={payload.language} direction={codeBlockDirection(payload.source, payload.language, inheritedDirection)} className="my-5 first:mt-0 last:mb-0" />
  },

  code: ({ children, className, ...props }: ComponentPropsWithoutRef<"code">) => {
    const isInline = !className
    if (isInline) {
      return <code dir={inlineCodeDirection(children, inheritedDirection)} className="break-all px-1 bg-border/60 dark:[.no-pre-highlight_&]:bg-background dark:[.text-pretty_&]:bg-neutral [.no-code-highlight_&]:!bg-transparent py-0.5 rounded text-sm whitespace-wrap [unicode-bidi:isolate]" {...props}>{children}</code>
    }
    return (
      <code className="block text-xs whitespace-pre" {...props}>
        {children}
      </code>
    )
  },

  table: ({ children, ...props }: ComponentPropsWithoutRef<"table">) => (
    <div className="border border-border  rounded-xl overflow-x-auto">
      <table className="table-auto min-w-full divide-y divide-border bg-background" {...props}>{children}</table>
    </div>
  ),

  tbody: ({ children, ...props }: ComponentPropsWithoutRef<"tbody">) => (
    <tbody className="divide-y divide-border" {...props}>{children}</tbody>
  ),

  th: ({ children, ...props }: ComponentPropsWithoutRef<"th">) => (
    <th dir={textDirectionFromChildren(children, inheritedDirection)} className="text-start text-xs uppercase text-muted-foreground tracking-wider p-2 pl-0 first:pl-3 bg-muted dark:bg-card [&_*]:font-semibold" {...props}>{children}</th>
  ),
  td: ({ children, ...props }: ComponentPropsWithoutRef<"td">) => (
    <td dir={textDirectionFromChildren(children, inheritedDirection)} className="text-start p-2 pl-0 first:pl-3 [&_*]:font-normal " {...props}>{children}</td>
  ),

  p: ({ children, ...props }: ComponentPropsWithoutRef<"p">) => (
    <p dir={textDirectionFromChildren(children, inheritedDirection)} className="break-words mt-5 mb-3 first:mt-0 last:mb-0" {...props}>{children}</p>
  ),

  li: ({ children, ...props }: ComponentPropsWithoutRef<"li">) => (
    <li dir={textDirectionFromChildren(children, inheritedDirection)} className="my-1" {...props}>{children}</li>
  ),

  blockquote: ({ children, ...props }: ComponentPropsWithoutRef<"blockquote">) => (
    (() => {
      const childNodes = Children.toArray(children)

      const firstChild = childNodes[0]
      if (firstChild !== undefined) {
        childNodes[0] = withChildClassName(firstChild, "mt-0")
      }

      const lastIndex = childNodes.length - 1
      const lastChild = childNodes[lastIndex]
      if (lastChild !== undefined) {
        childNodes[lastIndex] = withChildClassName(lastChild, "mb-0")
      }

      return (
        <blockquote
          dir={exceptionalBlockDirectionFromChildren(children, inheritedDirection)}
          className="abolqasem-blockquote my-5 first:mt-0 last:mb-0"
          {...props}
        >
          <span className="abolqasem-blockquote-mark" aria-hidden="true"><Quote /></span>
          <div className="abolqasem-blockquote-content">{childNodes}</div>
        </blockquote>
      )
    })()
  ),

  a: ({ children, ...props }: ComponentPropsWithoutRef<"a">) => (
    <a
      className="transition-all underline decoration-2 text-logo decoration-logo/50 hover:text-logo/70 dark:text-logo dark:decoration-logo/70 dark:hover:text-logo/60 dark:hover:decoration-logo/40 "
      target="_blank"
      rel="noopener noreferrer"
      {...props}
    >
      {children}
    </a>
  ),
}
}

export const markdownComponents = createBaseMarkdownComponents()

export function createMarkdownComponents(options?: {
  onOpenLocalLink?: OpenLocalLinkHandler
  source?: string
  baseDirection?: TextDirection
}) {
  const inheritedDirection = options?.baseDirection ?? getMarkdownMessageDirection(options?.source)
  const baseComponents = createBaseMarkdownComponents(inheritedDirection)
  return {
    ...baseComponents,
    a: ({ children, href, onClick, ...props }: ComponentPropsWithoutRef<"a">) => {
      const onOpenLocalLink = options?.onOpenLocalLink ?? useContext(OpenLocalLinkContext)
      const renderOptions = useTranscriptRenderOptions()
      const parsedLocalLink = parseLocalFileLink(href)

      if (parsedLocalLink && renderOptions.localLinkMode === "text") {
        return (
          <span className="transition-all underline decoration-2 text-logo decoration-logo/50">
            {children}
          </span>
        )
      }

      return (
        <a
          className="transition-all underline decoration-2 text-logo decoration-logo/50 hover:text-logo/70 dark:text-logo dark:decoration-logo/70 dark:hover:text-logo/60 dark:hover:decoration-logo/40 "
          href={href}
          target={parsedLocalLink ? undefined : "_blank"}
          rel={parsedLocalLink ? undefined : "noopener noreferrer"}
          onClick={(event) => {
            onClick?.(event)
            if (event.defaultPrevented || !parsedLocalLink || onOpenLocalLink === defaultOpenLocalLink) return
            event.preventDefault()
            onOpenLocalLink({
              ...parsedLocalLink,
              clientX: event.clientX,
              clientY: event.clientY,
              trigger: "click",
            })
          }}
          onContextMenu={(event) => {
            if (!parsedLocalLink || onOpenLocalLink === defaultOpenLocalLink) return
            event.preventDefault()
            onOpenLocalLink({
              ...parsedLocalLink,
              clientX: event.clientX,
              clientY: event.clientY,
              trigger: "contextmenu",
            })
          }}
          {...props}
        >
          {children}
        </a>
      )
    },
  }
}

export const markdownWithHeadingsComponents = {
  ...markdownComponents,
}
