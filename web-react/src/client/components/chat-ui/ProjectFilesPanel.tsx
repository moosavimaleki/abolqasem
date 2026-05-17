import { ChevronDown, ChevronRight, Code2, Copy, ExternalLink, FileText, Folder, FolderOpen, GripVertical, Loader2, PanelLeftClose, PanelRightClose, RefreshCw, Search, X } from "lucide-react"
import { useCallback, useEffect, useMemo, useReducer, useRef, useState, type DragEvent, type ReactElement } from "react"
import { useI18n } from "../../i18n/context"
import { cn } from "../../lib/utils"
import { FilePreviewPanel, fileRouteHref, type FilePreviewResponse } from "../file-preview/FilePreviewPanel"
import { Button } from "../ui/button"
import { Input } from "../ui/input"
import {
  invalidateProjectFiles,
  readProjectFilePreview,
  readProjectFileTree,
  searchProjectFiles,
  type ProjectFileEntry,
  type ProjectFileListResponse,
} from "./projectFilesData"
import { createProjectFileActions } from "./projectFileActions"

export type { ProjectFileEntry } from "./projectFilesData"

interface ProjectFilesPanelProps {
  projectId: string
  localPath?: string | null
  initialPath?: string | null
  previewMode?: "inline" | "none"
  side?: "left" | "right"
  closeKind?: "close" | "collapse"
  showCloseButton?: boolean
  focusSearchToken?: number
  onClose?: () => void
  onSelectFile?: (entry: ProjectFileEntry) => void
  onOpenFile?: (path: string) => void
  onOpenInFinder?: (path: string) => void
  onCopyFilePath?: (path: string) => void
  onCopyRelativePath?: (path: string) => void
}

const ROOT_PATH = ""
const PREFETCH_ROOT_DIRECTORY_LIMIT = 8

interface ProjectFilesPanelViewState {
  treeError: string | null
  selectedEntry: ProjectFileEntry | null
  preview: FilePreviewResponse | null
  previewLoading: boolean
  previewError: string | null
  searchQuery: string
  searchResults: ProjectFileEntry[]
  searchLoading: boolean
  searchError: string | null
  searchTruncated: boolean
}

const initialProjectFilesPanelViewState: ProjectFilesPanelViewState = {
  treeError: null,
  selectedEntry: null,
  preview: null,
  previewLoading: false,
  previewError: null,
  searchQuery: "",
  searchResults: [],
  searchLoading: false,
  searchError: null,
  searchTruncated: false,
}

function projectFilesPanelViewReducer(
  state: ProjectFilesPanelViewState,
  patch: Partial<ProjectFilesPanelViewState>,
): ProjectFilesPanelViewState {
  return { ...state, ...patch }
}

function formatFileSize(size: number | undefined) {
  if (!Number.isFinite(size ?? NaN)) return ""
  const value = size ?? 0
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${Math.round(value / 102.4) / 10} KB`
  return `${Math.round(value / (1024 * 102.4)) / 10} MB`
}

function isTextLikeFile(entry: ProjectFileEntry) {
  if (entry.type !== "file") return false
  const mimeType = entry.mimeType?.toLowerCase() ?? ""
  return mimeType.startsWith("text/")
    || mimeType === "application/json; charset=utf-8"
    || Boolean(entry.language && entry.language !== "text")
}

function displayProjectPath(localPath: string | null | undefined) {
  if (!localPath) return ""
  const parts = localPath.split(/[\\/]/).filter(Boolean)
  return parts.at(-1) ?? localPath
}

function fileParentPath(path: string, name: string) {
  const normalized = path.replaceAll("\\", "/")
  const suffix = `/${name}`
  if (normalized === name) return ""
  if (normalized.endsWith(suffix)) return normalized.slice(0, -suffix.length)
  const slashIndex = normalized.lastIndexOf("/")
  return slashIndex > 0 ? normalized.slice(0, slashIndex) : ""
}

function ancestorPathsFor(path: string) {
  return path.split("/").slice(0, -1).reduce<string[]>((paths, segment) => {
    const previous = paths.at(-1)
    paths.push(previous ? `${previous}/${segment}` : segment)
    return paths
  }, [])
}

function FileIcon({ entry, expanded }: { entry: ProjectFileEntry; expanded?: boolean }) {
  if (entry.type === "directory") {
    return expanded ? <FolderOpen className="h-4 w-4 text-amber-400" /> : <Folder className="h-4 w-4 text-amber-400" />
  }
  return isTextLikeFile(entry)
    ? <Code2 className="h-4 w-4 text-sky-400" />
    : <FileText className="h-4 w-4 text-muted-foreground" />
}

function EmptyPanelState({ children }: { children: string }) {
  return (
    <div className="rounded-2xl border border-dashed border-border/70 px-4 py-5 text-center text-xs leading-6 text-muted-foreground">
      {children}
    </div>
  )
}

function ProjectFileRow({
  entry,
  depth,
  selected,
  expanded,
  loading,
  variant = "tree",
  onToggle,
  onSelect,
}: {
  entry: ProjectFileEntry
  depth: number
  selected: boolean
  expanded: boolean
  loading: boolean
  variant?: "tree" | "search"
  onToggle: (path: string) => void
  onSelect: (entry: ProjectFileEntry) => void
}) {
  const isDirectory = entry.type === "directory"
  const shouldToggleDirectory = isDirectory && variant !== "search"
  const rowRef = useRef<HTMLDivElement>(null)
  const parentPath = fileParentPath(entry.path, entry.name)
  const handleDragStart = useCallback((event: DragEvent<HTMLSpanElement>) => {
    event.dataTransfer.effectAllowed = "copy"
    event.dataTransfer.setData("application/x-ai-agent-manager-project-path", entry.path)
    event.dataTransfer.setData("text/plain", entry.path)
  }, [entry.path])

  useEffect(() => {
    if (!selected) return
    rowRef.current?.scrollIntoView({ block: "center", inline: "nearest" })
  }, [selected])

  return (
    <div
      ref={rowRef}
      title={entry.path}
      className={cn(
        "group flex min-h-8 w-full min-w-0 items-center gap-1 rounded-xl py-1.5 pe-2 text-left text-sm transition-colors hover:bg-muted/55",
        variant === "search" && "min-h-10",
        selected && "bg-accent text-accent-foreground"
      )}
      style={{ paddingLeft: 8 + depth * 14 }}
      dir="ltr"
    >
      <span
        draggable
        onDragStart={handleDragStart}
        className="flex h-6 w-5 shrink-0 cursor-grab items-center justify-center rounded-lg text-muted-foreground/55 opacity-50 transition-opacity hover:bg-muted hover:text-foreground group-hover:opacity-100 active:cursor-grabbing"
        title={entry.path}
      >
        <GripVertical className="h-3.5 w-3.5" />
      </span>
      <button
        type="button"
        title={entry.path}
        onClick={() => shouldToggleDirectory ? onToggle(entry.path) : onSelect(entry)}
        className="flex min-w-0 flex-1 items-center gap-2 py-0.5 text-left"
      >
        <span className="flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground">
          {isDirectory ? (
            loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />
          ) : (
            <span className="h-3.5 w-3.5" />
          )}
        </span>
        <FileIcon entry={entry} expanded={expanded} />
        <span className={cn("min-w-0 flex-1", variant === "search" ? "leading-4" : "truncate")}>
          <span className="block truncate">{entry.name}</span>
          {variant === "search" && parentPath ? (
            <span className={cn(
              "block truncate font-mono text-[10px]",
              selected ? "text-accent-foreground/70" : "text-muted-foreground/75",
            )}>
              {parentPath}
            </span>
          ) : null}
        </span>
        {entry.type === "file" && entry.size !== undefined ? (
          <span className="hidden shrink-0 text-[10px] text-muted-foreground group-hover:inline">{formatFileSize(entry.size)}</span>
        ) : null}
      </button>
    </div>
  )
}

export function ProjectFilesPanel({
  projectId,
  localPath,
  initialPath,
  previewMode = "none",
  side = "right",
  closeKind = "close",
  showCloseButton = true,
  focusSearchToken,
  onClose,
  onSelectFile,
  onOpenFile,
  onOpenInFinder,
  onCopyFilePath,
  onCopyRelativePath,
}: ProjectFilesPanelProps) {
  const { t, direction } = useI18n()
  const labels = t.filesPanel
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(() => new Set([ROOT_PATH]))
  const [treeByPath, setTreeByPath] = useState<Record<string, ProjectFileListResponse | undefined>>({})
  const [loadingPaths, setLoadingPaths] = useState<Record<string, boolean>>({})
  const [viewState, setViewState] = useReducer(projectFilesPanelViewReducer, initialProjectFilesPanelViewState)
  const treeByPathRef = useRef<Record<string, ProjectFileListResponse | undefined>>({})
  const loadingPathSetRef = useRef<Set<string>>(new Set())
  const treeRequestGenerationRef = useRef(0)
  const treeControllersRef = useRef<Map<string, AbortController>>(new Map())
  const searchInputRef = useRef<HTMLInputElement | null>(null)
  const {
    treeError,
    selectedEntry,
    preview,
    previewLoading,
    previewError,
    searchQuery,
    searchResults,
    searchLoading,
    searchError,
    searchTruncated,
  } = viewState
  const trimmedSearchQuery = searchQuery.trim()
  const rootTitle = displayProjectPath(localPath) || labels.project
  const CloseIcon = closeKind === "collapse"
    ? side === "left" ? PanelLeftClose : PanelRightClose
    : X
  const closeLabel = closeKind === "collapse" ? labels.collapse : labels.close

  const showInlinePreview = previewMode === "inline"
  const selectedFileActions = useMemo(() => createProjectFileActions(selectedEntry, preview, {
    onOpenFile,
    onOpenInFinder,
    onCopyFilePath,
    onCopyRelativePath,
    onOpenFullPage: (path) => window.open(fileRouteHref(path), "_blank", "noopener,noreferrer"),
  }), [onCopyFilePath, onCopyRelativePath, onOpenFile, onOpenInFinder, preview, selectedEntry])

  const resetTreeRequests = useCallback(() => {
    treeRequestGenerationRef.current += 1
    for (const controller of treeControllersRef.current.values()) {
      controller.abort()
    }
    treeControllersRef.current.clear()
    loadingPathSetRef.current.clear()
    setLoadingPaths({})
  }, [])

  useEffect(() => {
    if (focusSearchToken === undefined) return
    const input = searchInputRef.current
    if (!input) return
    input.focus({ preventScroll: true })
    input.select()
  }, [focusSearchToken])

  const loadTree = useCallback((path: string, options: { force?: boolean } = {}) => {
    if (!options.force && treeByPathRef.current[path]) return
    if (loadingPathSetRef.current.has(path)) return
    const generation = treeRequestGenerationRef.current
    const controller = new AbortController()
    loadingPathSetRef.current.add(path)
    treeControllersRef.current.set(path, controller)
    setLoadingPaths((current) => ({ ...current, [path]: true }))
    setViewState({ treeError: null })
    readProjectFileTree(projectId, path, { signal: controller.signal, force: options.force })
      .then((payload) => {
        if (controller.signal.aborted || generation !== treeRequestGenerationRef.current) return
        setTreeByPath((current) => {
          const next = { ...current, [path]: payload }
          treeByPathRef.current = next
          return next
        })
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted || generation !== treeRequestGenerationRef.current) return
        setViewState({ treeError: error instanceof Error ? error.message : String(error) })
      })
      .finally(() => {
        const isCurrentController = treeControllersRef.current.get(path) === controller
        if (isCurrentController) {
          treeControllersRef.current.delete(path)
          loadingPathSetRef.current.delete(path)
        }
        if (isCurrentController && generation === treeRequestGenerationRef.current) {
          setLoadingPaths((current) => ({ ...current, [path]: false }))
        }
      })
  }, [projectId])

  const refreshTree = useCallback(() => {
    resetTreeRequests()
    invalidateProjectFiles(projectId)
    treeByPathRef.current = {}
    setTreeByPath({})
    setExpandedPaths(new Set([ROOT_PATH]))
    setViewState({
      selectedEntry: null,
      preview: null,
      previewLoading: false,
      previewError: null,
      treeError: null,
    })
    loadTree(ROOT_PATH, { force: true })
  }, [loadTree, resetTreeRequests])

  useEffect(() => {
    resetTreeRequests()
    treeByPathRef.current = {}
    setExpandedPaths(new Set([ROOT_PATH]))
    setTreeByPath({})
    setViewState(initialProjectFilesPanelViewState)
  }, [projectId, resetTreeRequests])

  useEffect(() => () => {
    resetTreeRequests()
  }, [resetTreeRequests])

  useEffect(() => {
    const normalizedInitialPath = (initialPath ?? "").replaceAll("\\", "/").replace(/^\/+/, "")
    if (!normalizedInitialPath) return
    const ancestorPaths = normalizedInitialPath.split("/").slice(0, -1).reduce<string[]>((paths, segment) => {
      const previous = paths.at(-1)
      paths.push(previous ? `${previous}/${segment}` : segment)
      return paths
    }, [])
    setExpandedPaths(new Set([ROOT_PATH, ...ancestorPaths]))
    setViewState({
      selectedEntry: {
        name: normalizedInitialPath.split("/").at(-1) ?? normalizedInitialPath,
        path: normalizedInitialPath,
        type: "file",
      },
    })
    ancestorPaths.forEach((path) => loadTree(path))
  }, [initialPath, loadTree])

  useEffect(() => {
    loadTree(ROOT_PATH)
  }, [loadTree])

  useEffect(() => {
    const root = treeByPath[ROOT_PATH]
    if (!root) return
    root.entries
      .filter((entry) => entry.type === "directory" && entry.hasChildren)
      .slice(0, PREFETCH_ROOT_DIRECTORY_LIMIT)
      .forEach((entry) => loadTree(entry.path))
  }, [loadTree, treeByPath])

  useEffect(() => {
    if (!showInlinePreview || !selectedEntry || selectedEntry.type !== "file") {
      setViewState({ preview: null, previewError: null, previewLoading: false })
      return
    }
    const controller = new AbortController()
    setViewState({ previewLoading: true, previewError: null })
    readProjectFilePreview(projectId, selectedEntry.path, { signal: controller.signal })
      .then((payload) => {
        if (controller.signal.aborted) return
        setViewState({ preview: payload })
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return
        setViewState({
          preview: null,
          previewError: error instanceof Error ? error.message : String(error),
        })
      })
      .finally(() => {
        if (!controller.signal.aborted) setViewState({ previewLoading: false })
      })
    return () => controller.abort()
  }, [projectId, selectedEntry, showInlinePreview])

  useEffect(() => {
    if (!trimmedSearchQuery) {
      setViewState({
        searchResults: [],
        searchLoading: false,
        searchError: null,
        searchTruncated: false,
      })
      return
    }
    const controller = new AbortController()
    const timeoutId = window.setTimeout(() => {
      setViewState({ searchLoading: true, searchError: null })
      searchProjectFiles(projectId, trimmedSearchQuery, { signal: controller.signal })
        .then((payload) => {
          if (controller.signal.aborted) return
          setViewState({
            searchResults: payload.entries,
            searchTruncated: payload.truncated,
          })
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted) return
          setViewState({
            searchResults: [],
            searchError: error instanceof Error ? error.message : String(error),
          })
        })
        .finally(() => {
          if (!controller.signal.aborted) setViewState({ searchLoading: false })
        })
    }, 180)
    return () => {
      window.clearTimeout(timeoutId)
      controller.abort()
    }
  }, [projectId, trimmedSearchQuery])

  const toggleDirectory = useCallback((path: string) => {
    setExpandedPaths((current) => {
      const next = new Set(current)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
        loadTree(path)
      }
      return next
    })
  }, [loadTree])

  const revealDirectoryInTree = useCallback((entry: ProjectFileEntry) => {
    const ancestors = ancestorPathsFor(entry.path)
    setExpandedPaths((current) => {
      const next = new Set(current)
      next.add(ROOT_PATH)
      ancestors.forEach((path) => next.add(path))
      next.add(entry.path)
      return next
    })
    ancestors.forEach((path) => loadTree(path))
    loadTree(entry.path)
    setViewState({ selectedEntry: entry, searchQuery: "" })
  }, [loadTree])

  const selectEntry = useCallback((entry: ProjectFileEntry) => {
    if (entry.type === "directory") {
      revealDirectoryInTree(entry)
      return
    }
    onSelectFile?.(entry)
    setViewState({ selectedEntry: entry, searchQuery: "" })
  }, [onSelectFile, revealDirectoryInTree])

  const treeContent = useMemo(() => {
    const renderEntries = (path: string, depth: number): ReactElement[] => {
      const entries = treeByPath[path]?.entries ?? []
      return entries.flatMap((entry) => {
        const expanded = expandedPaths.has(entry.path)
        const row = (
          <ProjectFileRow
            key={entry.path}
            entry={entry}
            depth={depth}
            selected={selectedEntry?.path === entry.path}
            expanded={expanded}
            loading={Boolean(loadingPaths[entry.path])}
            onToggle={toggleDirectory}
            onSelect={selectEntry}
          />
        )
        if (entry.type !== "directory" || !expanded) return [row]
        return [row, ...renderEntries(entry.path, depth + 1)]
      })
    }
    return renderEntries(ROOT_PATH, 0)
  }, [expandedPaths, loadingPaths, selectEntry, selectedEntry?.path, toggleDirectory, treeByPath])

  const visibleSearchResults = trimmedSearchQuery ? searchResults : []

  return (
    <div
      className={cn(
        "flex h-full min-h-0 flex-col overflow-hidden bg-background md:min-w-[370px]",
        side === "left"
          ? "[border-inline-end:1px_solid_hsl(var(--border))]"
          : "[border-inline-start:1px_solid_hsl(var(--border))]",
      )}
      dir={direction}
    >
      <div className="border-b border-border px-3 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
              <FolderOpen className="h-4 w-4 text-amber-400" />
              <span>{labels.title}</span>
            </div>
            <div className="mt-0.5 truncate text-[11px] text-muted-foreground" dir="auto">{rootTitle}</div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="none"
              aria-label={labels.refresh}
              title={labels.refresh}
              onClick={refreshTree}
              className="h-8 w-8 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground"
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
            {showCloseButton && onClose ? (
              <Button
                type="button"
                variant="ghost"
                size="none"
                aria-label={closeLabel}
                title={closeLabel}
                onClick={onClose}
                className="h-8 w-8 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground"
              >
                <CloseIcon className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
        </div>
        <div className="mt-3 flex items-center gap-2 rounded-xl border border-border bg-muted/25 px-2 py-1.5" dir="ltr">
          <Search className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <Input
            ref={searchInputRef}
            value={searchQuery}
            onChange={(event) => setViewState({ searchQuery: event.target.value })}
            placeholder={labels.searchPlaceholder}
            className="h-8 border-0 bg-transparent px-0 text-left text-sm shadow-none focus-visible:ring-0"
            dir="ltr"
          />
        </div>
      </div>

      <div className={cn("grid min-h-0 flex-1", showInlinePreview ? "grid-rows-[minmax(180px,42%)_minmax(0,1fr)]" : "grid-rows-[minmax(0,1fr)]")}>
        <div className="min-h-0 overflow-auto border-b border-border p-2" dir="ltr">
          {trimmedSearchQuery ? (
            <div className="space-y-1">
              {searchLoading && visibleSearchResults.length === 0 ? (
                <div className="flex items-center gap-2 px-3 py-4 text-xs text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {labels.searching}
                </div>
              ) : searchError ? (
                <div className="rounded-xl border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{searchError}</div>
              ) : visibleSearchResults.length > 0 ? (
                <>
                  {visibleSearchResults.map((entry) => (
                    <ProjectFileRow
                      key={entry.path}
                      entry={entry}
                      depth={0}
                      selected={selectedEntry?.path === entry.path}
                      expanded={entry.type === "directory" && expandedPaths.has(entry.path)}
                      loading={Boolean(loadingPaths[entry.path])}
                      variant="search"
                      onToggle={toggleDirectory}
                      onSelect={selectEntry}
                    />
                  ))}
                  {searchTruncated ? <div className="px-3 py-2 text-[11px] text-muted-foreground">{labels.resultsTruncated}</div> : null}
                </>
              ) : (
                <EmptyPanelState>{labels.noSearchResults}</EmptyPanelState>
              )}
            </div>
          ) : treeError ? (
            <div className="rounded-xl border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{treeError}</div>
          ) : treeContent.length > 0 ? (
            <div className="space-y-0.5">{treeContent}</div>
          ) : loadingPaths[ROOT_PATH] ? (
            <div className="flex items-center gap-2 px-3 py-4 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {labels.loadingTree}
            </div>
          ) : (
            <EmptyPanelState>{labels.emptyTree}</EmptyPanelState>
          )}
        </div>

        {showInlinePreview ? (
          <div className="min-h-0 overflow-hidden p-3">
          {selectedEntry ? (
            <div className="flex h-full min-h-0 flex-col gap-3">
              <div className="flex items-center justify-between gap-2 rounded-2xl border border-border bg-muted/25 px-3 py-2">
                <div className="min-w-0">
                  <div className="truncate text-xs font-semibold text-foreground" dir="auto">{selectedEntry.name}</div>
                  <div className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground" dir="ltr">{selectedEntry.path}</div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="none"
                    disabled={!selectedFileActions.canOpenInEditor}
                    title={labels.openInEditor}
                    aria-label={labels.openInEditor}
                    onClick={selectedFileActions.openInEditor}
                    className="h-7 w-7 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground disabled:opacity-40"
                  >
                    <Code2 className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="none"
                    disabled={!selectedFileActions.canRevealInFinder}
                    title={labels.revealInFinder}
                    aria-label={labels.revealInFinder}
                    onClick={selectedFileActions.revealInFinder}
                    className="h-7 w-7 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground disabled:opacity-40"
                  >
                    <FolderOpen className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="none"
                    disabled={!selectedFileActions.canCopyAbsolutePath}
                    title={labels.copyAbsolutePath}
                    aria-label={labels.copyAbsolutePath}
                    onClick={selectedFileActions.copyAbsolutePath}
                    className="h-7 w-7 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground disabled:opacity-40"
                  >
                    <Copy className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="none"
                    disabled={!selectedFileActions.canCopyRelativePath}
                    title={labels.copyRelativePath}
                    aria-label={labels.copyRelativePath}
                    onClick={selectedFileActions.copyRelativePath}
                    className="h-7 w-7 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground disabled:opacity-40"
                  >
                    <FileText className="h-3.5 w-3.5" />
                  </Button>
                  {selectedFileActions.canOpenFullPage ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="none"
                      title={labels.openFullPage}
                      aria-label={labels.openFullPage}
                      onClick={selectedFileActions.openFullPage}
                      className="h-7 w-7 border-border/0 text-muted-foreground hover:!border-border/0 hover:!bg-transparent hover:text-foreground"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                    </Button>
                  ) : null}
                </div>
              </div>

              <div className="min-h-0 flex-1 overflow-hidden">
                {previewLoading ? (
                  <div className="flex h-full items-center justify-center gap-2 text-xs text-muted-foreground">
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    {labels.loadingPreview}
                  </div>
                ) : previewError ? (
                  <div className="h-full rounded-2xl border border-border bg-muted/20 px-4 py-4 text-xs leading-6 text-muted-foreground">
                    <div className="font-medium text-foreground">{labels.previewUnavailable}</div>
                    <div className="mt-2 whitespace-pre-wrap" dir="auto">{previewError}</div>
                    <div className="mt-4 flex flex-wrap gap-2">
                      <Button type="button" size="sm" variant="outline" disabled={!selectedFileActions.canOpenInEditor} onClick={selectedFileActions.openInEditor}>
                        {labels.openInEditor}
                      </Button>
                      <Button type="button" size="sm" variant="outline" disabled={!selectedFileActions.canCopyAbsolutePath} onClick={selectedFileActions.copyAbsolutePath}>
                        {labels.copyAbsolutePath}
                      </Button>
                    </div>
                  </div>
                ) : preview ? (
                  <FilePreviewPanel preview={preview} className="h-full rounded-2xl" />
                ) : selectedEntry.type === "directory" ? (
                  <EmptyPanelState>{labels.selectFileFromDirectory}</EmptyPanelState>
                ) : (
                  <EmptyPanelState>{labels.selectFile}</EmptyPanelState>
                )}
              </div>
            </div>
          ) : (
            <EmptyPanelState>{labels.selectFile}</EmptyPanelState>
          )}
          </div>
        ) : null}
      </div>
    </div>
  )
}
