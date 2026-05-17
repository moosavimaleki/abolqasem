import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { AlertTriangle, Loader2, PanelLeftOpen } from "lucide-react"
import { parseLocalFileLink } from "../lib/pathUtils"
import { FilePreviewPanel, fileRouteHref, type FilePreviewResponse } from "../components/file-preview/FilePreviewPanel"
import { ProjectFilesPanel } from "../components/chat-ui/ProjectFilesPanel"
import { Button } from "../components/ui/button"
import { getAppearanceThemeClassName, useReaderAppearanceSettings } from "../components/appearance/ReaderAppearance"
import { useI18n } from "../i18n/context"
import { cn } from "../lib/utils"

function filePreviewURL(path: string, line?: number) {
  const params = new URLSearchParams({
    path,
    full: "1",
  })
  if (line && line > 0) {
    params.set("line", String(line))
  }
  return `/api/file-preview?${params.toString()}`
}

function fileContextURL(path: string) {
  const params = new URLSearchParams({ path })
  return `/api/file-context?${params.toString()}`
}

interface FileProjectContext {
  projectId: string
  localPath: string
  title: string
  relativePath: string
}

function joinProjectPath(localPath: string, relativePath: string) {
  const usesBackslash = localPath.includes("\\")
  const separator = usesBackslash ? "\\" : "/"
  const root = localPath.replace(/[\\/]+$/, "")
  const relative = relativePath.replaceAll("/", separator).replace(/^[\\/]+/, "")
  return `${root}${separator}${relative}`
}

function FilePreviewLoadingState({ label }: { label: string }) {
  return (
    <div className="flex h-full min-h-[320px] items-center justify-center gap-3 text-sm text-muted-foreground">
      <Loader2 className="h-4 w-4 animate-spin" />
      <span>{label}…</span>
    </div>
  )
}

function FilePreviewLoadingOverlay({ label }: { label: string }) {
  return (
    <div className="pointer-events-none absolute left-1/2 top-4 z-20 flex -translate-x-1/2 items-center gap-2 rounded-full border border-border bg-background/90 px-3 py-1.5 text-xs text-muted-foreground shadow-lg backdrop-blur">
      <Loader2 className="h-3.5 w-3.5 animate-spin" />
      <span>{label}…</span>
    </div>
  )
}

function FilePreviewErrorState({
  title,
  error,
  path,
}: {
  title: string
  error: string
  path?: string
}) {
  return (
    <div className="flex h-full min-h-[320px] items-center justify-center p-6">
      <div className="w-full max-w-2xl rounded-3xl border border-border bg-card px-5 py-5 text-card-foreground shadow-sm">
        <div className="flex items-start gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-destructive/10 text-destructive">
            <AlertTriangle className="h-5 w-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold text-foreground">{title}</div>
            {path ? <div className="mt-1 truncate font-mono text-xs text-muted-foreground" dir="ltr">{path}</div> : null}
            <div className="mt-4 whitespace-pre-wrap rounded-2xl border border-border bg-muted/30 px-3 py-3 text-xs leading-6 text-muted-foreground" dir="auto">
              {error}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function CollapsedExplorerRail({
  title,
  label,
  onExpand,
}: {
  title: string
  label: string
  onExpand: () => void
}) {
  return (
    <aside className="flex h-full min-h-0 flex-col items-center border-r border-border bg-background/95 px-2 py-3">
      <Button
        type="button"
        variant="ghost"
        size="none"
        aria-label={label}
        title={label}
        onClick={onExpand}
        className="h-10 w-10 rounded-2xl border border-border bg-card text-muted-foreground shadow-sm hover:text-foreground"
      >
        <PanelLeftOpen className="h-4 w-4" />
      </Button>
      <div className="mt-3 h-px w-8 bg-border" />
      <div className="mt-4 flex min-h-0 flex-1 items-start justify-center overflow-hidden">
        <div className="max-h-56 truncate text-[11px] font-medium text-muted-foreground [writing-mode:vertical-rl]" title={title}>
          {title}
        </div>
      </div>
    </aside>
  )
}

export function FileRoutePage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { t } = useI18n()
  const target = useMemo(() => parseLocalFileLink(`${window.location.origin}${location.pathname}${location.hash}`), [location.hash, location.pathname])
  const [preview, setPreview] = useState<FilePreviewResponse | null>(null)
  const [projectContext, setProjectContext] = useState<FileProjectContext | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [explorerCollapsed, setExplorerCollapsed] = useState(false)
  const [explorerSearchFocusToken, setExplorerSearchFocusToken] = useState(0)
  const lastShiftKeydownRef = useRef(0)
  const [appearanceSettings] = useReaderAppearanceSettings()
  const isMarkdown = preview?.language === "markdown"

  useEffect(() => {
    if (!target) {
      setPreview(null)
      setError("This route is not a local file path.")
      setLoading(false)
      return
    }

    const controller = new AbortController()
    setLoading(true)
    setError(null)
    fetch(filePreviewURL(target.path, target.line), {
      signal: controller.signal,
      headers: { Accept: "application/json" },
      cache: "no-store",
    })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(await response.text() || `File preview failed with ${response.status}`)
        }
        return response.json() as Promise<FilePreviewResponse>
      })
      .then((payload) => {
        if (controller.signal.aborted) return
        setPreview(payload)
        setError(null)
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        setPreview(null)
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })

    return () => {
      controller.abort()
    }
  }, [target])

  useEffect(() => {
    if (!target) {
      setProjectContext(null)
      return
    }
    const controller = new AbortController()
    fetch(fileContextURL(target.path), {
      signal: controller.signal,
      headers: { Accept: "application/json" },
      cache: "no-store",
    })
      .then(async (response) => {
        if (!response.ok) return null
        return response.json() as Promise<FileProjectContext>
      })
      .then((payload) => {
        if (!controller.signal.aborted) setProjectContext(payload)
      })
      .catch(() => {
        if (!controller.signal.aborted) setProjectContext(null)
      })
    return () => {
      controller.abort()
    }
  }, [target])

  useEffect(() => {
    setExplorerCollapsed(false)
  }, [projectContext?.projectId])

  const focusExplorerSearch = useCallback(() => {
    if (!projectContext) return
    setExplorerCollapsed(false)
    setExplorerSearchFocusToken((current) => current + 1)
  }, [projectContext])

  useEffect(() => {
    function handleKeydown(event: KeyboardEvent) {
      if (!projectContext) return
      if (event.key !== "Shift" || event.repeat) return
      const now = window.performance.now()
      if (now - lastShiftKeydownRef.current <= 450) {
        event.preventDefault()
        lastShiftKeydownRef.current = 0
        focusExplorerSearch()
        return
      }
      lastShiftKeydownRef.current = now
    }

    window.addEventListener("keydown", handleKeydown)
    return () => window.removeEventListener("keydown", handleKeydown)
  }, [focusExplorerSearch, projectContext])

  const openProjectFile = (relativePath: string) => {
    if (!projectContext) return
    navigate(fileRouteHref(joinProjectPath(projectContext.localPath, relativePath)), { preventScrollReset: true })
  }

  const explorer = projectContext && !explorerCollapsed ? (
    <ProjectFilesPanel
      projectId={projectContext.projectId}
      localPath={projectContext.localPath}
      initialPath={projectContext.relativePath}
      previewMode="none"
      side="left"
      closeKind="collapse"
      focusSearchToken={explorerSearchFocusToken || undefined}
      onClose={() => setExplorerCollapsed(true)}
      onSelectFile={(entry) => {
        if (entry.type !== "file") return
        openProjectFile(entry.path)
      }}
      onOpenFile={openProjectFile}
      onCopyFilePath={(path) => void navigator.clipboard?.writeText(joinProjectPath(projectContext.localPath, path))}
      onCopyRelativePath={(path) => void navigator.clipboard?.writeText(path)}
    />
  ) : null

  const collapsedExplorer = projectContext && explorerCollapsed ? (
    <CollapsedExplorerRail
      title={projectContext.title}
      label={t.filesPanel.expand}
      onExpand={() => setExplorerCollapsed(false)}
    />
  ) : null

  const previewContent = error ? (
    <FilePreviewErrorState title={t.filesPanel.previewUnavailable} error={error} path={target?.path} />
  ) : preview ? (
    <div className="relative h-full min-h-0">
      <FilePreviewPanel
        preview={preview}
        className={isMarkdown ? "h-full w-full" : "mx-auto h-full w-full max-w-6xl"}
      />
      {loading ? <FilePreviewLoadingOverlay label={t.common.loading} /> : null}
    </div>
  ) : loading ? (
    <FilePreviewLoadingState label={t.common.loading} />
  ) : null

  return (
    <main className={cn("min-h-[100dvh] bg-background text-foreground", getAppearanceThemeClassName(appearanceSettings), projectContext ? "overflow-hidden" : "overflow-auto")}>
      {projectContext ? (
        <div
          className={cn(
            "grid h-[100dvh] min-h-0 transition-[grid-template-columns] duration-200 ease-out",
            explorerCollapsed ? "grid-cols-[56px_minmax(0,1fr)]" : "grid-cols-[minmax(300px,390px)_minmax(0,1fr)]",
          )}
          dir="ltr"
        >
          {explorerCollapsed ? collapsedExplorer : explorer}
          <section className={cn("relative min-h-0 overflow-hidden", isMarkdown ? "" : "px-4 py-4")} dir="auto">
            {previewContent}
          </section>
        </div>
      ) : (
        <section className={cn("relative h-[100dvh] min-h-0", isMarkdown ? "" : "px-4 py-4")}>
          {previewContent}
        </section>
      )}
    </main>
  )
}
