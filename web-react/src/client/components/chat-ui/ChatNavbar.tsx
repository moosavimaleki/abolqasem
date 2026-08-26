import { useEffect, useRef, useState, type MouseEvent as ReactMouseEvent } from "react"
import { Check, Copy, Files, GitBranch, Globe, Loader2, Menu, MoreHorizontal, PanelLeft, PanelRight, Search as SearchIcon, Settings2, SquarePen, Terminal } from "lucide-react"
import type { EditorOpenSettings, EditorPreset, OpenExternalAction } from "../../../shared/protocol"
import { Button } from "../ui/button"
import { CardHeader } from "../ui/card"
import { Input } from "../ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover"
import { HotkeyTooltip, HotkeyTooltipContent, HotkeyTooltipTrigger } from "../ui/tooltip"
import { cn } from "../../lib/utils"
import { AbolqasemLogo } from "../AbolqasemLogo"
import { OpenExternalSelect } from "../open-external-menu"
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from "../ui/context-menu"
import { useI18n } from "../../i18n/context"
import { ReaderAppearancePopover } from "../appearance/ReaderAppearance"
import type { HydratedTranscriptMessage } from "../../../shared/types"
import { copyTextToClipboard } from "../messages/shared"
import { buildSessionCopyText, collectSessionCopyTurns } from "../../app/ChatPage/sessionCopy"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select"

export interface ChatSearchMatch {
  message_id?: string
  entry_id?: string
  role: string
  kind?: string
  index: number
  snippet: string
  created_at?: string | null
}

function openContextMenuFromButton(event: ReactMouseEvent<HTMLButtonElement>) {
  event.preventDefault()
  event.stopPropagation()
  const rect = event.currentTarget.getBoundingClientRect()
  event.currentTarget.dispatchEvent(new MouseEvent("contextmenu", {
    bubbles: true,
    cancelable: true,
    clientX: rect.left + rect.width / 2,
    clientY: rect.bottom,
    view: window,
  }))
}

function NavbarOverflowMenu({
  showOnDesktop,
  onToggleEmbeddedTerminal,
}: {
  showOnDesktop: boolean
  onToggleEmbeddedTerminal?: () => void
}) {
  const { t } = useI18n()
  if (!onToggleEmbeddedTerminal) return null

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <Button
          variant="ghost"
          size="none"
          onClick={openContextMenuFromButton}
          title={t.chat.moreActions}
          className={cn(
            "border border-border/0 hover:!border-border/0 px-1.5 h-9 hover:!bg-transparent",
            showOnDesktop ? "flex" : "flex md:hidden"
          )}
        >
          <MoreHorizontal strokeWidth={2} className="h-4.5" />
        </Button>
      </ContextMenuTrigger>
      <ContextMenuContent>
        {onToggleEmbeddedTerminal ? (
          <ContextMenuItem
            onSelect={(event) => {
              event.preventDefault()
              onToggleEmbeddedTerminal()
            }}
          >
            <Terminal strokeWidth={2} className="h-3.5 w-3.5" />
            <span className="text-xs font-medium">{t.chat.toggleTerminal}</span>
          </ContextMenuItem>
        ) : null}
      </ContextMenuContent>
    </ContextMenu>
  )
}

function ChatSessionSearchPopover({
  chatId,
  align,
  labels,
  onSelect,
}: {
  chatId: string
  align: "start" | "end"
  labels: {
    title: string
    placeholder: string
    hint: string
    loading: string
    empty: string
    failed: string
  }
  onSelect: (match: ChatSearchMatch) => void | Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [matches, setMatches] = useState<ChatSearchMatch[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const trimmedQuery = query.trim()

  useEffect(() => {
    if (!open) return
    const timeout = window.setTimeout(() => {
      inputRef.current?.focus()
    }, 0)
    return () => window.clearTimeout(timeout)
  }, [open])

  useEffect(() => {
    if (!open || !trimmedQuery) {
      setMatches([])
      setLoading(false)
      setError(null)
      return
    }

    const controller = new AbortController()
    const timeout = window.setTimeout(() => {
      setLoading(true)
      setError(null)
      const params = new URLSearchParams({
        chat_id: chatId,
        q: trimmedQuery,
        limit: "40",
      })
      fetch(`/api/search?${params.toString()}`, {
        signal: controller.signal,
        headers: { Accept: "application/json" },
        cache: "no-store",
      })
        .then(async (response) => {
          if (!response.ok) throw new Error(await response.text() || `Search failed with ${response.status}`)
          return response.json() as Promise<{ matches?: ChatSearchMatch[] }>
        })
        .then((payload) => {
          setMatches(payload.matches ?? [])
        })
        .catch((err: unknown) => {
          if (controller.signal.aborted) return
          setMatches([])
          setError(err instanceof Error ? err.message : String(err))
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false)
        })
    }, 250)

    return () => {
      window.clearTimeout(timeout)
      controller.abort()
    }
  }, [chatId, open, trimmedQuery])

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="none"
          title={labels.title}
          aria-label={labels.title}
          className="border border-border/0 px-1.5 h-9 hover:!border-border/0 hover:!bg-transparent"
        >
          <SearchIcon strokeWidth={2.1} className="h-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align={align} sideOffset={8} className="w-[min(calc(100vw-2rem),430px)] p-2">
        <div className="flex items-center gap-2 rounded-xl border border-border bg-muted/35 px-2 py-1.5">
          <SearchIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <Input
            ref={inputRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={labels.placeholder}
            className="h-8 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
          />
        </div>
        <div className="mt-2 max-h-[360px] overflow-y-auto">
          {!trimmedQuery ? (
            <div className="px-2 py-4 text-xs leading-6 text-muted-foreground">{labels.hint}</div>
          ) : loading && matches.length === 0 ? (
            <div className="flex items-center gap-2 px-2 py-4 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {labels.loading}
            </div>
          ) : error ? (
            <div className="px-2 py-4 text-xs text-destructive">{labels.failed}: {error}</div>
          ) : matches.length > 0 ? (
            <div className="space-y-1">
              {matches.map((match) => (
                <button
                  key={`${match.index}-${match.message_id ?? match.entry_id ?? match.snippet}`}
                  type="button"
                  onClick={() => {
                    setOpen(false)
                    void onSelect(match)
                  }}
                  className="block w-full rounded-xl px-2 py-2 text-start hover:bg-muted"
                >
                  <div className="text-xs font-medium text-foreground">{match.role || match.kind || "message"} · #{match.index}</div>
                  <div className="mt-1 line-clamp-3 text-xs leading-5 text-muted-foreground">{match.snippet}</div>
                </button>
              ))}
            </div>
          ) : (
            <div className="px-2 py-4 text-xs text-muted-foreground">{labels.empty}</div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

function ChatSessionCopyPopover({
  messages,
  align,
  isPersian,
}: {
  messages: HydratedTranscriptMessage[]
  align: "start" | "end"
  isPersian: boolean
}) {
  const [open, setOpen] = useState(false)
  const [turnCount, setTurnCount] = useState("5")
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const turnsAvailable = collectSessionCopyTurns(messages).length
  const title = isPersian ? "کپی سشن" : "Copy session"
  const labels = isPersian ? { user: "کاربر", assistant: "AI" } : { user: "User", assistant: "AI" }
  const selectedTurns = Number(turnCount)
  const copiedTurns = Math.min(selectedTurns, turnsAvailable)

  const handleCopy = async () => {
    const text = buildSessionCopyText(messages, selectedTurns, labels)
    if (!text) return
    try {
      await copyTextToClipboard(text)
      setCopied(true)
      setError(null)
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      setError(isPersian ? "کپی در کلیپ‌بورد ناموفق بود." : "Could not copy to the clipboard.")
    }
  }

  return (
    <Popover open={open} onOpenChange={(nextOpen) => {
      setOpen(nextOpen)
      if (!nextOpen) setError(null)
    }}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="none"
          title={title}
          aria-label={title}
          className="gap-1.5 border border-border/0 px-2 h-9 hover:!border-border/0 hover:!bg-transparent"
        >
          <Copy strokeWidth={2.1} className="h-4 w-4" />
          <span className="text-xs font-medium">{title}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align={align} sideOffset={8} className="w-[min(calc(100vw-2rem),300px)] p-3">
        <div className="space-y-3">
          <div>
            <div className="text-sm font-medium text-foreground">{title}</div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {isPersian
                ? "هر turn شامل یک پیام کاربر و همهٔ پاسخ‌های AI تا پیام بعدی است."
                : "Each turn includes one user message and every AI reply before the next user message."}
            </p>
          </div>
          <label className="block space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">{isPersian ? "تعداد turn آخر" : "Last turns"}</span>
            <Select value={turnCount} onValueChange={setTurnCount}>
              <SelectTrigger aria-label={isPersian ? "تعداد turn برای کپی" : "Number of turns to copy"} className="h-10">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[1, 3, 5, 10].map((count) => (
                  <SelectItem key={count} value={String(count)}>{count}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
          {turnsAvailable === 0 ? (
            <p role="status" className="text-xs text-muted-foreground">{isPersian ? "هنوز turn قابل کپی وجود ندارد." : "There are no turns to copy yet."}</p>
          ) : null}
          {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
          <Button type="button" className="w-full" onClick={() => void handleCopy()} disabled={turnsAvailable === 0}>
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
            {copied
              ? (isPersian ? "کپی شد" : "Copied")
              : (isPersian ? `کپی ${copiedTurns} turn` : `Copy ${copiedTurns} turn${copiedTurns === 1 ? "" : "s"}`)}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

interface Props {
  sidebarCollapsed: boolean
  onOpenSidebar: () => void
  onExpandSidebar: () => void
  onNewChat: () => void
  localPath?: string
  embeddedTerminalVisible?: boolean
  onToggleEmbeddedTerminal?: () => void
  rightPanel?: "hidden" | "git" | "browser" | "files"
  onToggleGitPanel?: () => void
  onToggleBrowserPanel?: () => void
  onToggleFilesPanel?: () => void
  onOpenExternal?: (action: OpenExternalAction, editor?: EditorOpenSettings) => void
  activeChatId?: string | null
  messages?: HydratedTranscriptMessage[]
  onChatSearchResultSelect?: (match: ChatSearchMatch) => void | Promise<void>
  editorPreset?: EditorPreset
  editorCommandTemplate?: string
  platform?: NodeJS.Platform
  finderShortcut?: string[]
  editorShortcut?: string[]
  terminalShortcut?: string[]
  rightSidebarShortcut?: string[]
  branchName?: string
  hasGitRepo?: boolean
  gitStatus?: "unknown" | "ready" | "no_repo"
}

export function ChatNavbar({
  sidebarCollapsed,
  onOpenSidebar,
  onExpandSidebar,
  onNewChat,
  localPath,
  embeddedTerminalVisible = false,
  onToggleEmbeddedTerminal,
  rightPanel = "hidden",
  onToggleGitPanel,
  onToggleBrowserPanel,
  onToggleFilesPanel,
  onOpenExternal,
  activeChatId,
  messages = [],
  onChatSearchResultSelect,
  editorPreset = "cursor",
  editorCommandTemplate,
  platform = "darwin",
  finderShortcut,
  editorShortcut,
  terminalShortcut,
  rightSidebarShortcut,
  branchName,
  hasGitRepo = true,
  gitStatus = "unknown",
}: Props) {
  const { t, locale, direction } = useI18n()
  const isPersian = locale === "fa" || direction === "rtl"
  const appearanceLabel = isPersian ? "تنظیمات نمایش" : "Appearance settings"
  const branchLabel = !hasGitRepo
    ? t.chat.setupGit
    : gitStatus === "unknown"
      ? null
      : (branchName ?? t.chat.detachedHead)
  const isMac = platform === "darwin"
  const rightPanelVisible = rightPanel !== "hidden"
  const handleCloseRightPanel = rightPanel === "browser"
    ? onToggleBrowserPanel
    : rightPanel === "git"
      ? onToggleGitPanel
      : rightPanel === "files"
        ? onToggleFilesPanel
        : undefined
  const showBrowserPanelButton = rightPanel !== "browser"
  const showFilesPanelButton = rightPanel !== "files"
  const showGitPanelButton = rightPanel !== "git"
  const canSearchCurrentChat = Boolean(activeChatId && onChatSearchResultSelect)
  const chatSearchLabels = isPersian ? {
    title: "جست‌وجو در همین نشست",
    placeholder: "جست‌وجو در transcript همین نشست",
    hint: "کل پیام‌های این نشست از بک‌اند جست‌وجو می‌شود؛ حتی پیام‌هایی که هنوز در صفحه لود نشده‌اند.",
    loading: "در حال جست‌وجو در همین نشست…",
    empty: "نتیجه‌ای در این نشست پیدا نشد.",
    failed: "جست‌وجو ناموفق بود",
  } : {
    title: "Search this chat",
    placeholder: "Search this chat transcript",
    hint: "Search runs on the backend across the full transcript, including messages not loaded in the viewport yet.",
    loading: "Searching this chat…",
    empty: "No results in this chat.",
    failed: "Search failed",
  }
  const hasHeaderActions = Boolean(
    onOpenExternal
    || canSearchCurrentChat
    || messages.length > 0
    || onToggleEmbeddedTerminal
    || onToggleGitPanel
    || onToggleBrowserPanel
    || onToggleFilesPanel
  )

  return (
    <CardHeader
      className={cn(
        "absolute top-0 left-0 right-0 z-10 px-2 md:pt-[9px] border-border/0 flex items-center justify-center",
        "bg-gradient-to-b from-background lg:from-background/0"
      )}
    >
      <div className="absolute top-0 left-0 right-0 z-0 h-[100px] bg-gradient-to-b from-background via-background/50 pointer-events-none block"></div>
      <div className="relative flex items-center gap-2 w-full">
        <div className={`h-[30px] flex items-center gap-0 flex-shrink-0 border border-border/0 rounded-[9px] ${sidebarCollapsed ? 'px-1.5  border-border' : ''} px-[2px]`}>
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden !h-[auto] hover:!border-border/0 hover:!bg-transparent"
            onClick={onOpenSidebar}
          >
            <Menu className="size-4" />
          </Button>
          {sidebarCollapsed && (
            <>
              <div className="hidden md:flex items-center justify-center w-[36px] h-[36px]">
                <AbolqasemLogo className="h-4 w-4 sm:h-5 sm:w-5 text-logo mx-1 hidden md:block" />
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="hidden md:flex  hover:!border-border/0 hover:!bg-transparent"
                onClick={onExpandSidebar}
                title={t.sidebar.expandSidebar}
              >
                {isPersian ? <PanelRight className="size-4" /> : <PanelLeft className="size-4" />}
              </Button>
            </>
          )}
          <Button
            variant="ghost"
            size="icon"
            className="hover:!border-border/0 hover:!bg-transparent"
            onClick={onNewChat}
            title={t.chat.compose}
          >
            <SquarePen className="size-4" />
          </Button>
        </div>

        <div className="min-w-0 flex-1" />

        {(localPath || canSearchCurrentChat || messages.length > 0) && hasHeaderActions ? (
          <div className="flex items-center gap-2 flex-shrink-0">
            {localPath && onOpenExternal ? (
              <div className="hidden md:flex h-[30px] items-center overflow-hidden border border-border/70 rounded-[9px] backdrop-blur-lg">
                <OpenExternalSelect
                  isMac={isMac}
                  editorPreset={editorPreset}
                  editorCommandTemplate={editorCommandTemplate}
                  finderShortcut={finderShortcut}
                  editorShortcut={editorShortcut}
                  onOpenExternal={onOpenExternal}
                />
              </div>
            ) : null}
            {(canSearchCurrentChat || onToggleEmbeddedTerminal || onToggleGitPanel || onToggleBrowserPanel || onToggleFilesPanel) ? (
              <div className="flex items-center  rounded-[9px] h-[30px]">
                <ChatSessionCopyPopover
                  messages={messages}
                  align={isPersian ? "start" : "end"}
                  isPersian={isPersian}
                />
                {canSearchCurrentChat && activeChatId && onChatSearchResultSelect ? (
                  <ChatSessionSearchPopover
                    chatId={activeChatId}
                    align={isPersian ? "start" : "end"}
                    labels={chatSearchLabels}
                    onSelect={onChatSearchResultSelect}
                  />
                ) : null}
                <ReaderAppearancePopover
                  title={appearanceLabel}
                  align={isPersian ? "start" : "end"}
                  sideOffset={8}
                  trigger={(
                    <Button
                      type="button"
                      variant="ghost"
                      size="none"
                      title={appearanceLabel}
                      aria-label={appearanceLabel}
                      className="hidden border border-border/0 px-1.5 h-9 hover:!border-border/0 hover:!bg-transparent md:flex"
                    >
                      <Settings2 strokeWidth={2.1} className="h-4" />
                    </Button>
                  )}
                />
                <NavbarOverflowMenu
                  showOnDesktop={rightPanelVisible}
                  onToggleEmbeddedTerminal={onToggleEmbeddedTerminal}
                />
                {onToggleEmbeddedTerminal ? (
                <HotkeyTooltip>
                  <HotkeyTooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="none"
                      onClick={onToggleEmbeddedTerminal}
                      className={cn(
                        rightPanelVisible ? "hidden" : "hidden md:flex",
                        "border border-border/0 hover:!border-border/0 px-1.5 h-9 hover:!bg-transparent",
                        embeddedTerminalVisible && "text-foreground"
                      )}
                    >
                      <Terminal strokeWidth={2} className="h-4" />
                    </Button>
                  </HotkeyTooltipTrigger>
                  <HotkeyTooltipContent side="bottom" shortcut={terminalShortcut} />
                </HotkeyTooltip>
              ) : null}
                {onToggleBrowserPanel && showBrowserPanelButton ? (
                  <Button
                    variant="ghost"
                    size="none"
                    onClick={onToggleBrowserPanel}
                    title={t.chat.browser}
                    aria-label={t.chat.browser}
                    className={cn(
                      "border border-border/0 hover:!border-border/0 px-1.5 h-9 hover:!bg-transparent"
                    )}
                  >
                    <Globe strokeWidth={2.25} className="h-4" />
                  </Button>
                ) : null}
                {onToggleFilesPanel && showFilesPanelButton ? (
                  <Button
                    variant="ghost"
                    size="none"
                    onClick={onToggleFilesPanel}
                    title={t.filesPanel.title}
                    aria-label={t.filesPanel.title}
                    className="border border-border/0 hover:!border-border/0 px-1.5 h-9 hover:!bg-transparent"
                  >
                    <Files strokeWidth={2.25} className="h-4" />
                  </Button>
                ) : null}
                {onToggleGitPanel && showGitPanelButton ? (
                  <HotkeyTooltip>
                    <HotkeyTooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="none"
                        onClick={onToggleGitPanel}
                        className={cn(
                          "border flex flex-row items-center gap-1.5 h-9 border-border/0 hover:!border-border/0 hover:!bg-transparent",
                          rightPanelVisible ? "w-[38px] justify-center px-0" : "pl-1.5 pr-2"
                        )}
                      >
                        <GitBranch strokeWidth={2.25} className="h-4" />
                        {branchLabel && !rightPanelVisible ? <div className="font-[13px] max-w-[140px] truncate hidden md:block">{branchLabel}</div> : null}
                      </Button>
                    </HotkeyTooltipTrigger>
                    <HotkeyTooltipContent side="bottom" shortcut={rightSidebarShortcut} />
                  </HotkeyTooltip>
                ) : null}
                {rightPanelVisible && handleCloseRightPanel ? (
                  <Button
                    variant="ghost"
                    size="none"
                    onClick={handleCloseRightPanel}
                    title={t.chat.collapseSidebar}
                    aria-label={t.chat.collapseSidebar}
                    className="border border-border/0 hover:!border-border/0 px-1.5 h-9 hover:!bg-transparent text-foreground"
                  >
                    <PanelRight strokeWidth={2.25} className="h-4" />
                  </Button>
                ) : null}
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </CardHeader>
  )
}
