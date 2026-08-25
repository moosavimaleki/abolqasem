import { memo, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react"
import { FolderKanban, Loader2, MessageSquare, PanelLeft, PanelRight, X, Menu, Plus, Settings, Search as SearchIcon, SquarePen } from "lucide-react"
import { useLocation, useNavigate } from "react-router-dom"
import { APP_NAME } from "../../shared/branding"
import { AbolqasemLogo } from "../components/AbolqasemLogo"
import { Button } from "../components/ui/button"
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "../components/ui/dialog"
import { formatSidebarAgeLabel } from "../lib/formatters"
import { getSidebarChatTimestamp } from "../lib/sidebarChats"
import { cn } from "../lib/utils"
import { ChatRow } from "../components/chat-ui/sidebar/ChatRow"
import { LocalProjectsSection } from "../components/chat-ui/sidebar/LocalProjectsSection"
import { getResolvedKeybindings } from "../lib/keybindings"
import type { AgentProvider, AppLocale, KeybindingsSnapshot, SidebarData, SidebarChatRow, UpdateSnapshot } from "../../shared/types"
import type { SocketStatus } from "./socket"
import {
  getSidebarJumpTargetIndex,
  getSidebarNumberJumpHint,
  getVisibleSidebarChats,
  isSidebarModifierShortcut,
  shouldShowSidebarNumberJumpHints,
} from "./sidebarNumberJump"
import { useI18n } from "../i18n/context"
import { chatRoute, settingsRoute } from "./routes"

const SIDEBAR_WIDTH_STORAGE_KEY = "abolqasem:sidebar-width"
const SIDEBAR_VIEW_STORAGE_KEY = "abolqasem:sidebar-view"
type SidebarView = "chats" | "projects"
export const DEFAULT_SIDEBAR_WIDTH = 275
export const MIN_SIDEBAR_WIDTH = 220
export const MAX_SIDEBAR_WIDTH = 520

export function clampSidebarWidth(width: number) {
  if (!Number.isFinite(width)) return DEFAULT_SIDEBAR_WIDTH
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, Math.round(width)))
}

function readStoredSidebarWidth() {
  if (typeof window === "undefined") return DEFAULT_SIDEBAR_WIDTH
  const stored = window.localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY)
  return stored ? clampSidebarWidth(Number(stored)) : DEFAULT_SIDEBAR_WIDTH
}

function persistSidebarWidth(width: number) {
  if (typeof window === "undefined") return
  window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(clampSidebarWidth(width)))
}

function readStoredSidebarView(): SidebarView {
  if (typeof window === "undefined") return "chats"
  return window.localStorage.getItem(SIDEBAR_VIEW_STORAGE_KEY) === "projects" ? "projects" : "chats"
}

function SidebarSearch({
  data,
  onSelectChat,
}: {
  data: SidebarData
  onSelectChat: (chatId: string) => void
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState("")
  const [scope, setScope] = useState<SearchScope>("names")
  const [globalResults, setGlobalResults] = useState<BackendSearchResult[]>([])
  const [globalNextOffset, setGlobalNextOffset] = useState(0)
  const [globalTotal, setGlobalTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmedQuery = query.trim()

  const nameResults = useMemo(() => {
    if (!trimmedQuery || scope !== "names") return []
    const lower = trimmedQuery.toLowerCase()
    return allSidebarSearchChats(data)
      .filter(({ chat, projectName, projectPath }) => (
        chat.title.toLowerCase().includes(lower)
        || projectName.toLowerCase().includes(lower)
        || projectPath.toLowerCase().includes(lower)
      ))
      .slice(0, 50)
  }, [data, scope, trimmedQuery])

  useEffect(() => {
    if (!trimmedQuery || scope !== "content") {
      setGlobalResults([])
      setGlobalNextOffset(0)
      setGlobalTotal(0)
      setLoading(false)
      setError(null)
      return
    }

    const controller = new AbortController()
    const timeout = window.setTimeout(() => {
      setLoading(true)
      setError(null)
      const params = new URLSearchParams({ q: trimmedQuery, limit: "30" })
      fetch(`/api/search?${params.toString()}`, {
        signal: controller.signal,
        headers: { Accept: "application/json" },
        cache: "no-store",
      })
        .then(async (response) => {
          if (!response.ok) throw new Error(await response.text() || `Search failed with ${response.status}`)
        return response.json() as Promise<{ items: BackendSearchResult[]; next_offset: number; total: number }>
      })
      .then((payload) => {
        setGlobalResults(payload.items ?? [])
        setGlobalNextOffset(Number(payload.next_offset || 0))
        setGlobalTotal(Number(payload.total || payload.items?.length || 0))
      })
        .catch((err: unknown) => {
          if (controller.signal.aborted) return
          setGlobalResults([])
          setError(err instanceof Error ? err.message : String(err))
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false)
        })
    }, 300)

    return () => {
      window.clearTimeout(timeout)
      controller.abort()
    }
  }, [scope, trimmedQuery])

  const loadMoreGlobalResults = useCallback(() => {
    if (!trimmedQuery || scope !== "content" || globalNextOffset <= 0 || loading) return
    setLoading(true)
    setError(null)
    const params = new URLSearchParams({
      q: trimmedQuery,
      limit: "30",
      offset: String(globalNextOffset),
    })
    fetch(`/api/search?${params.toString()}`, {
      headers: { Accept: "application/json" },
      cache: "no-store",
    })
      .then(async (response) => {
        if (!response.ok) throw new Error(await response.text() || `Search failed with ${response.status}`)
        return response.json() as Promise<{ items: BackendSearchResult[]; next_offset: number; total: number }>
      })
      .then((payload) => {
        setGlobalResults((current) => [...current, ...(payload.items ?? [])])
        setGlobalNextOffset(Number(payload.next_offset || 0))
        setGlobalTotal(Number(payload.total || globalTotal))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => setLoading(false))
  }, [globalNextOffset, globalTotal, loading, scope, trimmedQuery])

  return (
    <div data-sidebar-control="search" className="mb-2">
      <div className="flex h-10 items-center gap-2 rounded-xl border border-border/80 bg-background/70 px-2.5 transition-colors focus-within:border-foreground/25 focus-within:ring-1 focus-within:ring-ring/40">
        <SearchIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        {scope === "names" ? (
          <button
            type="button"
            className="inline-flex h-6 shrink-0 cursor-pointer items-center gap-1 rounded-md bg-muted/70 px-1.5 text-[10px] text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => setScope("content")}
            title={t.sidebar.searchBackspaceHint}
          >
            {t.sidebar.searchNamesScope}
            <X className="h-3 w-3" />
          </button>
        ) : null}
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Backspace" && query.length === 0 && scope === "names") {
              setScope("content")
            }
          }}
          placeholder={scope === "names" ? t.sidebar.searchNamesPlaceholder : t.sidebar.searchContentPlaceholder}
          aria-label={scope === "names" ? t.sidebar.searchNamesPlaceholder : t.sidebar.searchContentPlaceholder}
          className="min-w-0 flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground/80"
        />
        {query ? (
          <button
            type="button"
            className="flex size-7 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => setQuery("")}
            aria-label={t.sidebar.searchClear}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        ) : null}
      </div>

      {trimmedQuery ? (
        <div className="mt-2 max-h-[35vh] space-y-1 overflow-y-auto">
          {scope === "names" ? (
            nameResults.length > 0 ? nameResults.map(({ chat, projectName }) => (
              <button
                key={chat.chatId}
                type="button"
                onClick={() => onSelectChat(chat.chatId)}
                className="block w-full rounded-xl px-2 py-2 text-start hover:bg-muted"
              >
                <div className="truncate text-xs font-medium text-foreground">{chat.title}</div>
                <div className="mt-1 truncate text-xs text-muted-foreground">{projectName || chat.localPath || t.localDev.localProjects}</div>
              </button>
            )) : (
              <div className="px-2 py-3 text-xs text-muted-foreground">{t.sidebar.searchNoNameResults}</div>
            )
          ) : loading && globalResults.length === 0 ? (
            <div className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {t.sidebar.searchingContent}
            </div>
          ) : error ? (
            <div className="px-2 py-3 text-xs text-destructive">{error}</div>
          ) : globalResults.length > 0 ? (
            <>
              {globalResults.map((result) => {
                const chatId = chatIdForSearchResult(data, result)
                return (
                  <button
                    key={result.key}
                    type="button"
                    disabled={!chatId}
                    onClick={() => chatId ? onSelectChat(chatId) : undefined}
                    className="block w-full rounded-xl px-2 py-2 text-start hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    <div className="truncate text-xs font-medium text-foreground">{result.project_name || t.localDev.localProjects} / {result.session_name}</div>
                    <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">{result.search_matches?.[0]?.snippet ?? ""}</div>
                  </button>
                )
              })}
              {globalNextOffset > 0 ? (
                <button
                  type="button"
                  onClick={loadMoreGlobalResults}
                  disabled={loading}
                  className="w-full rounded-xl px-2 py-2 text-center text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-60"
                >
                  {loading ? `${t.common.loading}…` : t.sidebar.searchMoreResults(globalResults.length, globalTotal)}
                </button>
              ) : null}
            </>
          ) : (
            <div className="px-2 py-3 text-xs text-muted-foreground">{t.sidebar.searchNoContentResults}</div>
          )}
        </div>
      ) : scope === "content" ? (
        <button
          type="button"
          onClick={() => setScope("names")}
          className="mt-2 w-full rounded-xl px-2 py-2 text-start text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          {t.sidebar.searchReturnToNames}
        </button>
      ) : null}
    </div>
  )
}

export function SidebarPrimaryControls({
  data,
  locale,
  sidebarView,
  onChangeView,
  onNewChat,
  onAddProject,
  onSelectChat,
}: {
  data: SidebarData
  locale: AppLocale
  sidebarView: SidebarView
  onChangeView: (view: SidebarView) => void
  onNewChat: () => void
  onAddProject: () => void
  onSelectChat: (chatId: string) => void
}) {
  const isPersian = locale === "fa"

  return (
    <div className="shrink-0 border-b border-border/60 px-2 py-2">
      <div
        data-sidebar-control="actions"
        className="mb-2 grid grid-cols-2 gap-1.5"
        role="group"
        aria-label={isPersian ? "عملیات پروژه" : "Project actions"}
      >
        <button
          type="button"
          onClick={onNewChat}
          data-sidebar-action="new-chat"
          className="flex h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-transparent px-2 text-xs font-medium text-foreground/80 transition-colors hover:border-border/50 hover:bg-muted/45 hover:text-foreground active:bg-muted/65 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <SquarePen className="size-3.5" />
          {isPersian ? "چت جدید" : "New chat"}
        </button>
        <button
          type="button"
          onClick={onAddProject}
          data-sidebar-action="add-project"
          className="flex h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border border-transparent px-2 text-xs text-muted-foreground transition-colors hover:border-border/50 hover:bg-muted/45 hover:text-foreground active:bg-muted/65 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Plus className="size-3.5" />
          {isPersian ? "افزودن پروژه" : "Add project"}
        </button>
      </div>

      <SidebarSearch data={data} onSelectChat={onSelectChat} />

      <div
        data-sidebar-control="view-filter"
        className="grid grid-cols-2 gap-1 rounded-lg bg-muted/35 p-1"
        role="tablist"
        aria-label={isPersian ? "نمایش سایدبار" : "Sidebar view"}
      >
        {(["chats", "projects"] as const).map((view) => (
          <button
            key={view}
            type="button"
            role="tab"
            aria-selected={sidebarView === view}
            onClick={() => onChangeView(view)}
            className={cn(
              "flex h-8 cursor-pointer items-center justify-center gap-1.5 rounded-md text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              sidebarView === view
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:bg-background/35 hover:text-foreground",
            )}
          >
            {view === "chats" ? <MessageSquare className="size-3.5" /> : <FolderKanban className="size-3.5" />}
            {view === "chats" ? (isPersian ? "چت‌ها" : "Chats") : (isPersian ? "پروژه‌ها" : "Projects")}
          </button>
        ))}
      </div>
    </div>
  )
}

interface AbolqasemSidebarProps {
  data: SidebarData
  activeChatId: string | null
  connectionStatus: SocketStatus
  ready: boolean
  pendingArchiveChatIds: ReadonlySet<string>
  open: boolean
  collapsed: boolean
  showMobileOpenButton: boolean
  onOpen: () => void
  onClose: () => void
  onCollapse: () => void
  onExpand: () => void
  onCreateChat: (projectId: string) => void
  onForkChat: (chat: SidebarChatRow) => void
  onConvertChat: (chat: SidebarChatRow, provider: AgentProvider) => void
  currentProjectId: string | null
  creatingChatProjectId: string | null
  keybindings: KeybindingsSnapshot | null
  onRenameChat: (chat: SidebarChatRow) => void
  onArchiveChat: (chat: SidebarChatRow) => void
  onOpenArchivedChat: (chatId: string) => void
  onDeleteChat: (chat: SidebarChatRow) => void
  onOpenAddProjectModal: () => void
  onCopyPath: (localPath: string) => void
  onOpenExternalPath: (action: "open_finder" | "open_editor", localPath: string) => void
  onRenameProject: (projectId: string, sidebarTitle: string | undefined, realTitle: string) => void
  onHideProject: (projectId: string) => void
  onReorderProjectGroups: (projectIds: string[]) => void
  editorLabel: string
  updateSnapshot: UpdateSnapshot | null
  onOpenChangelog: () => void
}

type SearchScope = "names" | "content"

interface BackendSearchMatch {
  role: string
  snippet: string
  index: number
}

interface BackendSearchResult {
  key: string
  chat_id?: string
  session_name: string
  project_name: string
  updated_at: string
  search_matches: BackendSearchMatch[]
}

function allSidebarSearchChats(data: SidebarData) {
  const seen = new Set<string>()
  return data.projectGroups.flatMap((group) => {
    const projectName = group.sidebarTitle || group.title || group.realTitle
    const projectPath = group.localPath
    return [
      ...group.chats,
      ...group.previewChats,
      ...group.olderChats,
      ...(group.archivedChats ?? []),
    ].flatMap((chat) => {
      if (seen.has(chat.chatId)) return []
      seen.add(chat.chatId)
      return [{ chat, projectName, projectPath }]
    })
  })
}

export function getUsageOrderedSidebarChats(data: SidebarData) {
  return allSidebarSearchChats(data).sort(
    (left, right) => getSidebarChatTimestamp(right.chat) - getSidebarChatTimestamp(left.chat),
  )
}

function chatIdForSearchResult(data: SidebarData, result: BackendSearchResult) {
  if (result.chat_id) return result.chat_id
  return allSidebarSearchChats(data).find(({ chat }) => chat.legacySessionKey === result.key)?.chat.chatId ?? null
}

function AbolqasemSidebarImpl({
  data,
  activeChatId,
  connectionStatus,
  ready,
  pendingArchiveChatIds,
  open,
  collapsed,
  showMobileOpenButton,
  onOpen,
  onClose,
  onCollapse,
  onExpand,
  onCreateChat,
  onForkChat,
  onConvertChat,
  currentProjectId,
  creatingChatProjectId,
  keybindings,
  onRenameChat,
  onArchiveChat,
  onOpenArchivedChat,
  onDeleteChat,
  onOpenAddProjectModal,
  onCopyPath,
  onOpenExternalPath,
  onRenameProject,
  onHideProject,
  onReorderProjectGroups,
  editorLabel,
  updateSnapshot,
  onOpenChangelog,
}: AbolqasemSidebarProps) {
  const { t, direction, locale } = useI18n()
  const isRtl = direction === "rtl"
  const location = useLocation()
  const navigate = useNavigate()
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const resizeStartRef = useRef<{ pointerX: number; width: number } | null>(null)
  const initializedCollapsedGroupKeysRef = useRef<Set<string>>(new Set())
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(new Set())
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [nowMs, setNowMs] = useState(() => Date.now())
  const [showNumberJumpHints, setShowNumberJumpHints] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(readStoredSidebarWidth)
  const [isResizingSidebar, setIsResizingSidebar] = useState(false)
  const [archivedProjectId, setArchivedProjectId] = useState<string | null>(null)
  const [sidebarView, setSidebarView] = useState<SidebarView>(readStoredSidebarView)
  const resolvedKeybindings = useMemo(() => getResolvedKeybindings(keybindings), [keybindings])
  const visibleChats = useMemo(
    () => getVisibleSidebarChats(data.projectGroups, collapsedSections, expandedGroups),
    [collapsedSections, data.projectGroups, expandedGroups]
  )
  const visibleChatsRef = useRef(visibleChats)
  const visibleIndexByChatId = useMemo(
    () => new Map(visibleChats.map((entry) => [entry.chat.chatId, entry.visibleIndex])),
    [visibleChats]
  )

  const projectIdByPath = useMemo(
    () => new Map(data.projectGroups.map((group) => [group.localPath, group.groupKey])),
    [data.projectGroups]
  )

  const activeVisibleCount = visibleChats.length
  const archivedProject = useMemo(
    () => data.projectGroups.find((group) => group.groupKey === archivedProjectId) ?? null,
    [archivedProjectId, data.projectGroups]
  )
  const flatChats = useMemo(
    () => getUsageOrderedSidebarChats(data),
    [data],
  )

  const changeSidebarView = useCallback((view: SidebarView) => {
    setSidebarView(view)
    window.localStorage.setItem(SIDEBAR_VIEW_STORAGE_KEY, view)
  }, [])

  useEffect(() => {
    visibleChatsRef.current = visibleChats
  }, [visibleChats])

  useEffect(() => {
    setCollapsedSections((previous) => {
      const next = new Set<string>()
      const projectKeys = new Set(data.projectGroups.map((group) => group.groupKey))
      const initializedKeys = initializedCollapsedGroupKeysRef.current

      for (const key of previous) {
        if (projectKeys.has(key)) {
          next.add(key)
        }
      }

      initializedCollapsedGroupKeysRef.current = new Set(
        [...initializedKeys].filter((key) => projectKeys.has(key))
      )

      for (const group of data.projectGroups) {
        if (initializedCollapsedGroupKeysRef.current.has(group.groupKey)) continue
        initializedCollapsedGroupKeysRef.current.add(group.groupKey)
        if (group.defaultCollapsed) {
          next.add(group.groupKey)
        }
      }

      if (next.size === previous.size && [...next].every((key) => previous.has(key))) {
        return previous
      }

      return next
    })
  }, [data.projectGroups])

  const toggleSection = useCallback((key: string) => {
    setCollapsedSections((previous) => {
      const next = new Set(previous)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }, [])

  const toggleExpandedGroup = useCallback((key: string) => {
    setExpandedGroups((previous) => {
      const next = new Set(previous)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  const renderChatRow = useCallback((chat: SidebarChatRow) => {
    const visibleIndex = visibleIndexByChatId.get(chat.chatId)

    return (
      <ChatRow
        key={chat._id}
        chat={chat}
        activeChatId={activeChatId}
        nowMs={nowMs}
        shortcutHint={visibleIndex ? getSidebarNumberJumpHint(resolvedKeybindings, visibleIndex) : null}
        showShortcutHint={showNumberJumpHints}
        onSelectChat={(chatId) => {
          navigate(chatRoute(chatId))
          onClose()
        }}
        onRenameChat={() => onRenameChat(chat)}
        onOpenInFinder={() => onOpenExternalPath("open_finder", chat.localPath)}
        onForkChat={() => onForkChat(chat)}
        onConvertChat={(_, provider) => onConvertChat(chat, provider)}
        onArchiveChat={() => onArchiveChat(chat)}
        onDeleteChat={() => onDeleteChat(chat)}
        isArchiving={pendingArchiveChatIds.has(chat.chatId)}
      />
    )
  }, [activeChatId, navigate, nowMs, onArchiveChat, onClose, onConvertChat, onDeleteChat, onForkChat, onOpenExternalPath, onRenameChat, pendingArchiveChatIds, resolvedKeybindings, showNumberJumpHints, visibleIndexByChatId])

  useEffect(() => {
    const intervalId = window.setInterval(() => {
      setNowMs(Date.now())
    }, 30_000)

    return () => window.clearInterval(intervalId)
  }, [])

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      setShowNumberJumpHints(shouldShowSidebarNumberJumpHints(resolvedKeybindings, event))

      if (isSidebarModifierShortcut(resolvedKeybindings, "createChatInCurrentProject", event)) {
        if (!currentProjectId) {
          return
        }

        event.preventDefault()
        onCreateChat(currentProjectId)
        return
      }

      if (isSidebarModifierShortcut(resolvedKeybindings, "openAddProject", event)) {
        event.preventDefault()
        navigate("/")
        onClose()
        onOpenAddProjectModal()
        return
      }

      const targetIndex = getSidebarJumpTargetIndex(resolvedKeybindings, event)
      if (targetIndex === null) {
        return
      }

      const targetChat = visibleChatsRef.current[targetIndex - 1]?.chat
      if (!targetChat) {
        return
      }

      event.preventDefault()
        navigate(chatRoute(targetChat.chatId))
        onClose()
    }

    function handleKeyUp(event: KeyboardEvent) {
      setShowNumberJumpHints(shouldShowSidebarNumberJumpHints(resolvedKeybindings, event))
    }

    function clearHints() {
      setShowNumberJumpHints(false)
    }

    window.addEventListener("keydown", handleKeyDown)
    window.addEventListener("keyup", handleKeyUp)
    window.addEventListener("blur", clearHints)

    return () => {
      window.removeEventListener("keydown", handleKeyDown)
      window.removeEventListener("keyup", handleKeyUp)
      window.removeEventListener("blur", clearHints)
    }
  }, [currentProjectId, navigate, onClose, onCreateChat, onOpenAddProjectModal, resolvedKeybindings])

  useEffect(() => {
    if (!activeChatId || !scrollContainerRef.current) return

    requestAnimationFrame(() => {
      const container = scrollContainerRef.current
      const activeElement = container?.querySelector(`[data-chat-id="${activeChatId}"]`) as HTMLElement | null
      if (!activeElement || !container) return

      const elementRect = activeElement.getBoundingClientRect()
      const containerRect = container.getBoundingClientRect()

      if (elementRect.top < containerRect.top + 38) {
        const relativeTop = elementRect.top - containerRect.top + container.scrollTop
        container.scrollTo({ top: relativeTop - 38, behavior: "smooth" })
      } else if (elementRect.bottom > containerRect.bottom) {
        const elementCenter = elementRect.top + elementRect.height / 2 - containerRect.top + container.scrollTop
        const containerCenter = container.clientHeight / 2
        container.scrollTo({ top: elementCenter - containerCenter, behavior: "smooth" })
      }
    })
  }, [activeChatId])

  useEffect(() => {
    if (!isResizingSidebar) return

    const previousCursor = document.body.style.cursor
    const previousUserSelect = document.body.style.userSelect
    document.body.style.cursor = "col-resize"
    document.body.style.userSelect = "none"

    function handlePointerMove(event: PointerEvent) {
      const resizeStart = resizeStartRef.current
      if (!resizeStart) return
      setSidebarWidth(clampSidebarWidth(
        isRtl
          ? resizeStart.width + resizeStart.pointerX - event.clientX
          : resizeStart.width + event.clientX - resizeStart.pointerX
      ))
    }

    function handlePointerUp() {
      setIsResizingSidebar(false)
      resizeStartRef.current = null
      setSidebarWidth((current) => {
        const next = clampSidebarWidth(current)
        persistSidebarWidth(next)
        return next
      })
    }

    window.addEventListener("pointermove", handlePointerMove)
    window.addEventListener("pointerup", handlePointerUp, { once: true })

    return () => {
      window.removeEventListener("pointermove", handlePointerMove)
      window.removeEventListener("pointerup", handlePointerUp)
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = previousUserSelect
    }
  }, [isResizingSidebar, isRtl])

  const hasVisibleChats = sidebarView === "chats" ? flatChats.length > 0 : activeVisibleCount > 0
  const isLocalProjectsActive = location.pathname === "/"
  const isSettingsActive = location.pathname.startsWith("/_/settings") || location.pathname.startsWith("/settings")
  const isUtilityPageActive = isLocalProjectsActive || isSettingsActive
  const isConnecting = connectionStatus === "connecting" || !ready
  const statusLabel = isConnecting ? t.sidebar.connecting : connectionStatus === "connected" ? t.sidebar.connected : t.sidebar.disconnected
  const statusDotClass = connectionStatus === "connected" ? "bg-emerald-500" : "bg-amber-500"
  const showUpdateButton = updateSnapshot?.updateAvailable === true
  const showDevBadge = updateSnapshot
    ? updateSnapshot.latestVersion === `${updateSnapshot.currentVersion}-dev`
    : false
  const isUpdating = updateSnapshot?.status === "updating" || updateSnapshot?.status === "restart_pending"

  return (
    <>
      {!open && showMobileOpenButton && (
        <Button
          variant="ghost"
          size="icon"
          className={cn("fixed top-3 z-50 md:hidden", isRtl ? "right-3" : "left-3")}
          onClick={onOpen}
        >
          <Menu className="h-5 w-5" />
        </Button>
      )}

      {collapsed && isUtilityPageActive && (
        <div className={cn(
          "hidden md:flex fixed top-0 h-full z-40 items-start pt-4 border-border/0",
          isRtl ? "right-0 pr-5 border-r" : "left-0 pl-5 border-l"
        )}>
          <div className="flex items-center gap-1">
            <AbolqasemLogo className="size-6 text-logo" />
            <Button
              variant="ghost"
              size="icon"
              onClick={onExpand}
              title={t.sidebar.expandSidebar}
            >
              {isRtl ? <PanelRight className="h-5 w-5" /> : <PanelLeft className="h-5 w-5" />}
            </Button>
          </div>
        </div>
      )}

      <div
        data-sidebar="open"
        className={cn(
          "fixed inset-0 z-50 bg-background dark:bg-card flex flex-col h-[100dvh] select-none",
          "md:relative md:inset-auto md:h-full md:w-[var(--sidebar-width)] md:shrink-0 md:rounded-none",
          open ? "flex" : "hidden md:flex",
          collapsed && "md:hidden",
          isRtl ? "md:border-l md:border-border" : "md:border-r md:border-border"
        )}
        style={{ "--sidebar-width": `${sidebarWidth}px` } as CSSProperties}
      >
        <div className="grid h-12 shrink-0 grid-cols-[40px_minmax(0,1fr)_40px] items-center border-b px-1.5 md:flex md:justify-between md:px-2">
          <div className="md:hidden">
            <Button
              variant="ghost"
              size="icon"
              className="size-10 rounded-lg hover:!border-border/0"
              onClick={onClose}
              title={t.sidebar.closeSidebar}
            >
              <X className="h-5 w-5" />
            </Button>
          </div>
          <div className="flex min-w-0 items-center justify-self-center gap-1.5 md:justify-self-auto">
            <button
              type="button"
              onClick={onCollapse}
              title={t.sidebar.collapseSidebar}
              className="group/sidebar-collapse relative hidden size-8 cursor-pointer items-center justify-center rounded-lg transition-colors hover:bg-muted md:flex"
            >
              <AbolqasemLogo className="absolute size-5 text-logo transition-all duration-200 ease-out opacity-100 scale-100 group-hover/sidebar-collapse:opacity-0 group-hover/sidebar-collapse:scale-0" />
              {isRtl ? (
                <PanelRight className="absolute size-5 text-muted-foreground transition-all duration-200 ease-out opacity-0 scale-0 group-hover/sidebar-collapse:opacity-100 group-hover/sidebar-collapse:scale-100" />
              ) : (
                <PanelLeft className="absolute size-5 text-muted-foreground transition-all duration-200 ease-out opacity-0 scale-0 group-hover/sidebar-collapse:opacity-100 group-hover/sidebar-collapse:scale-100" />
              )}
            </button>
            <AbolqasemLogo className="size-5 text-logo md:hidden" />
            <span className="truncate font-logo text-sm uppercase text-foreground">{APP_NAME}</span>
          </div>
          <div className="flex items-center justify-self-end md:justify-self-auto">
            {showDevBadge ? (
              <span
                className="hidden items-center rounded-full border border-border bg-muted px-2 py-0.5 text-[10px] font-bold tracking-wider text-muted-foreground md:inline-flex"
                title={t.sidebar.developmentBuild}
              >
                DEV
              </span>
            ) : showUpdateButton ? (
              <Button
                variant="outline"
                size="sm"
                className="hidden rounded-full !h-auto border-logo/20 bg-logo/15 px-2 py-0.5 text-[10px] font-bold tracking-wider text-logo hover:border-logo/20 hover:bg-logo/25 hover:text-foreground md:inline-flex"
                onClick={onOpenChangelog}
                disabled={isUpdating}
                title={updateSnapshot?.latestVersion ? t.sidebar.updateTo(updateSnapshot.latestVersion) : t.sidebar.updateAbolqasem}
              >
                {isUpdating ? <Loader2 className="me-1.5 h-3 w-3 animate-spin" /> : null}
                {t.sidebar.update}
              </Button>
            ) : null}
          </div>
        </div>

        <SidebarPrimaryControls
          data={data}
          locale={locale}
          sidebarView={sidebarView}
          onChangeView={changeSidebarView}
          onNewChat={() => currentProjectId ? onCreateChat(currentProjectId) : onOpenAddProjectModal()}
          onAddProject={() => {
            navigate("/")
            onClose()
            onOpenAddProjectModal()
          }}
          onSelectChat={(chatId) => {
            navigate(chatRoute(chatId))
            onClose()
          }}
        />

        <div
          ref={scrollContainerRef}
          className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden scrollbar-hide"
          style={{
            WebkitOverflowScrolling: "touch",
            touchAction: "pan-y",
          }}
        >
          <div className="px-[7px] py-1.5">
            {!hasVisibleChats && isConnecting ? (
              <div className="space-y-5 px-1 pt-3">
                {[0, 1, 2].map((section) => (
                  <div key={section} className="space-y-2 animate-pulse">
                    <div className="h-4 w-28 rounded bg-muted" />
                    <div className="space-y-1">
                      {[0, 1, 2].map((row) => (
                        <div key={row} className="flex items-center gap-2 rounded-md px-3 py-2">
                          <div className="h-3.5 w-3.5 rounded-full bg-muted" />
                          <div
                            className={cn(
                              "h-3.5 rounded bg-muted",
                              row === 0 ? "w-32" : row === 1 ? "w-40" : "w-28"
                            )}
                          />
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            ) : null}

            {!hasVisibleChats && !isConnecting && data.projectGroups.length === 0 ? (
              <p className="text-sm text-muted-foreground p-2 mt-6 text-center">{t.sidebar.noConversations}</p>
            ) : null}

            {sidebarView === "chats" ? (
              <div>
                {flatChats.map(({ chat, projectName }) => (
                  <div key={chat.chatId}>
                    <div className="truncate px-3 pt-1 text-[10px] text-muted-foreground/70">{projectName}</div>
                    {renderChatRow(chat)}
                  </div>
                ))}
              </div>
            ) : <LocalProjectsSection
              projectGroups={data.projectGroups}
              editorLabel={editorLabel}
              onReorderGroups={onReorderProjectGroups}
              collapsedSections={collapsedSections}
              expandedGroups={expandedGroups}
              onToggleSection={toggleSection}
              onToggleExpandedGroup={toggleExpandedGroup}
              renderChatRow={renderChatRow}
              onShowArchivedProject={setArchivedProjectId}
              onNewLocalChat={(localPath) => {
                const projectId = projectIdByPath.get(localPath)
                if (projectId) {
                  onCreateChat(projectId)
                }
              }}
              onCopyPath={onCopyPath}
              onOpenExternalPath={onOpenExternalPath}
              onRenameProject={onRenameProject}
              onHideProject={onHideProject}
              isConnected={connectionStatus === "connected"}
              creatingChatProjectId={creatingChatProjectId}
            />}
          </div>
        </div>

        <div className="border-t border-border p-2">
            <button
            type="button"
            onClick={() => {
              navigate(settingsRoute("general"))
              onClose()
            }}
            className={cn(
              "w-full rounded-xl rounded-t-md border px-3 py-2 text-start transition-colors",
              isSettingsActive
                ? "bg-muted border-border"
                : "border-border/0 hover:bg-muted hover:border-border active:bg-muted/80"
            )}
          >
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Settings className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm">{t.sidebar.settings}</span>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span>{statusLabel}</span>
                {isConnecting ? (
                  <Loader2 className="h-2 w-2 animate-spin" />
                ) : (
                  <span className={cn("h-2 w-2 rounded-full", statusDotClass)} />
                )}
              </div>
            </div>
          </button>
        </div>

        <div
          role="separator"
          aria-orientation="vertical"
          aria-label={t.sidebar.resizeSidebar}
          tabIndex={0}
          title={t.sidebar.resizeSidebar}
          className={cn(
            "hidden md:block absolute top-0 bottom-0 z-20 w-2 cursor-col-resize",
            isRtl ? "-left-1" : "-right-1",
            "focus-visible:outline-none"
          )}
          onPointerDown={(event) => {
            event.preventDefault()
            resizeStartRef.current = {
              pointerX: event.clientX,
              width: sidebarWidth,
            }
            setIsResizingSidebar(true)
          }}
          onDoubleClick={() => {
            setSidebarWidth(DEFAULT_SIDEBAR_WIDTH)
            persistSidebarWidth(DEFAULT_SIDEBAR_WIDTH)
          }}
          onKeyDown={(event) => {
            let nextWidth: number | null = null
            if (event.key === "ArrowLeft") nextWidth = isRtl ? sidebarWidth + 16 : sidebarWidth - 16
            else if (event.key === "ArrowRight") nextWidth = isRtl ? sidebarWidth - 16 : sidebarWidth + 16
            else if (event.key === "Home") nextWidth = MIN_SIDEBAR_WIDTH
            else if (event.key === "End") nextWidth = MAX_SIDEBAR_WIDTH
            else if (event.key === "Enter") nextWidth = DEFAULT_SIDEBAR_WIDTH
            if (nextWidth === null) return
            event.preventDefault()
            const clampedWidth = clampSidebarWidth(nextWidth)
            setSidebarWidth(clampedWidth)
            persistSidebarWidth(clampedWidth)
          }}
        />
      </div>

      <Dialog
        open={Boolean(archivedProject)}
        onOpenChange={(dialogOpen) => {
          if (!dialogOpen) setArchivedProjectId(null)
        }}
      >
        <DialogContent size="md">
          <DialogHeader>
            <DialogTitle>{t.sidebar.archivedChats}</DialogTitle>
            <DialogDescription>
              {archivedProject?.localPath ?? ""}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-1">
            {archivedProject?.archivedChats?.length ? (
              archivedProject.archivedChats.map((chat) => (
                <button
                  key={chat.chatId}
                  type="button"
                  className="flex w-full items-center justify-between gap-3 rounded-lg border border-border/0 px-3 py-2 text-left transition-colors hover:border-border hover:bg-muted"
                  onClick={() => {
                    onOpenArchivedChat(chat.chatId)
                    setArchivedProjectId(null)
                    onClose()
                  }}
                >
                  <span className="min-w-0 truncate text-sm">{locale === "fa" && chat.title === "New Chat" ? t.sidebar.newChat : chat.title}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {formatSidebarAgeLabel(getSidebarChatTimestamp(chat), nowMs)}
                  </span>
                </button>
              ))
            ) : (
              <p className="px-1 py-3 text-sm text-muted-foreground">{t.sidebar.noArchivedChats}</p>
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>

      {open ? <div className="fixed inset-0 bg-black/40 z-40 md:hidden" onClick={onClose} /> : null}
    </>
  )
}

export const AbolqasemSidebar = memo(AbolqasemSidebarImpl)
