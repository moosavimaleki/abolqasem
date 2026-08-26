import { memo, useState } from "react"
import { Archive, ChevronDown, Loader2, Split } from "lucide-react"
import type { AgentProvider, SidebarChatRow } from "../../../../shared/types"
import { AnimatedShinyText } from "../../ui/animated-shiny-text"
import { Button } from "../../ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "../../ui/popover"
import { Kbd } from "../../ui/kbd"
import { PROVIDER_ICONS } from "../ChatPreferenceControls"
import { formatSidebarAgeLabel } from "../../../lib/formatters"
import { getSidebarChatTimestamp } from "../../../lib/sidebarChats"
import { cn, normalizeChatId } from "../../../lib/utils"
import { ChatRowMenu } from "./Menus"
import { useI18n } from "../../../i18n/context"

const loadingStatuses = new Set(["starting", "running"])
const providerLabels: Record<AgentProvider, string> = {
  claude: "Claude",
  codex: "Codex",
}
interface Props {
  chat: SidebarChatRow
  activeChatId: string | null
  nowMs: number
  shortcutHint?: string | null
  showShortcutHint?: boolean
  onSelectChat: (chatId: string) => void
  onRenameChat: (chatId: string) => void
  onOpenInNewTab: (chatId: string) => void
  onOpenInFinder: (localPath: string) => void
  onForkChat: (chatId: string) => void
  onConvertChat: (chatId: string, provider: AgentProvider) => void
  onArchiveChat: (chatId: string) => void
  onDeleteChat: (chatId: string) => void
  isArchiving?: boolean
}

function ChatRowImpl({
  chat,
  activeChatId,
  nowMs,
  shortcutHint = null,
  showShortcutHint = false,
  onSelectChat,
  onRenameChat,
  onOpenInNewTab,
  onOpenInFinder,
  onForkChat,
  onConvertChat,
  onArchiveChat,
  onDeleteChat,
  isArchiving = false,
}: Props) {
  const { t, direction, locale } = useI18n()
  const [forkMenuOpen, setForkMenuOpen] = useState(false)
  const displayTitle = locale === "fa" && chat.title === "New Chat" ? t.sidebar.newChat : chat.title
  const ageLabel = formatSidebarAgeLabel(getSidebarChatTimestamp(chat), nowMs)
  const trailingLabel = showShortcutHint && shortcutHint ? shortcutHint : ageLabel
  const showShortcutKeycap = showShortcutHint && Boolean(shortcutHint)
  const normalizedChatId = normalizeChatId(chat.chatId)
  const ProviderIcon = chat.provider ? PROVIDER_ICONS[chat.provider] : null

  if (isArchiving) {
    return (
      <div
        data-chat-id={normalizedChatId}
        dir={direction}
        className="flex items-center gap-2 rounded-lg border border-border/0 px-[9px] py-1.5 opacity-80"
        aria-busy="true"
        aria-live="polite"
      >
        <Loader2 className="size-3.5 shrink-0 animate-spin text-muted-foreground" />
        <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md border border-border/30 bg-muted/45">
          <div className="h-3 w-3 rounded-sm bg-muted-foreground/20" />
        </div>
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="h-3 w-4/5 animate-pulse rounded-full bg-muted-foreground/18" />
          <div className="h-2 w-2/5 animate-pulse rounded-full bg-muted-foreground/12" />
        </div>
        <div className="h-6 w-8 shrink-0 animate-pulse rounded-full bg-muted-foreground/12" />
      </div>
    )
  }

  const row = (
    <div
      key={chat._id}
      data-chat-id={normalizedChatId}
      dir={direction}
      className={cn(
        "group flex items-center gap-2 ps-[9px] pe-0.5 py-0.5 rounded-lg cursor-pointer border-border/0 hover:border-border hover:bg-muted/20 active:scale-[0.985] border transition-all",
        activeChatId === normalizedChatId ? "bg-muted hover:bg-muted border-border" : "border-border/0 hover:border-border/60"
      )}
      onClick={() => onSelectChat(chat.chatId)}
    >
      {loadingStatuses.has(chat.status) ? (
        <Loader2 className="size-3.5 flex-shrink-0 animate-spin text-logo" />
      ) : chat.status === "waiting_for_user" ? (
        <div className="relative ">
          <div className=" rounded-full z-0 size-3.5 flex items-center justify-center ">
            <div className="absolute rounded-full z-0 size-2.5 bg-blue-400/80 animate-ping" />
            <div className=" rounded-full z-0 size-2.5 bg-blue-400 ring-2 ring-muted/50" />
          </div>
        </div>
      ) : chat.unread ? (
        <div
          className="relative"
          role="status"
          title={locale === "fa" ? "خوانده‌نشده" : "Unread"}
          aria-label={locale === "fa" ? "خوانده‌نشده" : "Unread"}
        >
          <div className=" rounded-full z-0 size-3.5 flex items-center justify-center ">
            <div className="absolute rounded-full z-0 size-2.5 bg-emerald-400/80 animate-ping" />
            <div className=" rounded-full z-0 size-2.5 bg-emerald-400 ring-2 ring-muted/50" />
          </div>
        </div>
      ) : null}
      {ProviderIcon && chat.provider ? (
        <span
          className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md border border-border/40 bg-background/35 text-muted-foreground opacity-65 transition-opacity group-hover:opacity-90"
          title={providerLabels[chat.provider]}
          aria-label={providerLabels[chat.provider]}
        >
          <ProviderIcon className="size-3.5" />
        </span>
      ) : null}
      <span dir={direction} className="text-sm text-start truncate flex-1 translate-y-[-0.5px]">
        {chat.status !== "idle" && chat.status !== "waiting_for_user" ? (
          <AnimatedShinyText
            animate={chat.status === "running"}
            shimmerWidth={Math.max(20, displayTitle.length * 3)}
          >
            <span dir="auto">{displayTitle}</span>
          </AnimatedShinyText>
        ) : chat.status !== "idle" || activeChatId === normalizedChatId || chat.unread ? (
          <span dir="auto">{displayTitle}</span>
        ) : (
          <span dir="auto" className="text-muted-foreground">{displayTitle}</span>
        )}
      </span>
      {chat.readOnly ? (
        <span dir={direction} className="hidden rounded-full border border-border/70 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground md:inline-flex">
          {t.sidebar.readOnly}
        </span>
      ) : null}
      <div className={cn("relative h-7 me-[2px] shrink-0", chat.canFork ? "w-12" : "w-6")}>
        {trailingLabel ? (
          showShortcutKeycap ? (
            <span className="hidden md:flex absolute inset-0 items-center justify-end pe-0.5 text-[11px] text-foreground transition-opacity group-hover:opacity-0">
              <Kbd className="h-4 min-w-4 rounded-sm border-border/50 bg-transparent px-1 text-[10px]">
                {shortcutHint}
              </Kbd>
            </span>
          ) : (
            <span className="hidden md:flex absolute inset-0 items-center justify-end pe-2 text-[11px] text-muted-foreground opacity-60 transition-opacity group-hover:opacity-0">
              <span dir="ltr">{trailingLabel}</span>
            </span>
          )
        ) : null}
        <div
          className={cn(
            "absolute inset-0 flex items-center justify-end gap-0 opacity-100 me-[3px]",
            trailingLabel
              ? "md:opacity-0 md:group-hover:opacity-100"
              : "opacity-100 md:opacity-0 md:group-hover:opacity-100"
          )}
        >
          {chat.canFork ? (
            <Popover open={forkMenuOpen} onOpenChange={setForkMenuOpen}>
              <PopoverTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 cursor-pointer rounded-sm hover:!bg-transparent !border-0"
                  onClick={(event) => {
                    event.stopPropagation()
                  }}
                  title={t.sidebar.forkChat}
                >
                  <Split className="size-3.5" />
                </Button>
              </PopoverTrigger>
              <PopoverContent
                align={direction === "rtl" ? "start" : "end"}
                className="w-52 p-1"
                onClick={(event) => event.stopPropagation()}
              >
                <div dir={direction} className="flex flex-col gap-1 text-start">
                  <button
                    className="flex items-center justify-between rounded-lg px-3 py-2 text-xs font-medium hover:bg-muted"
                    onClick={(event) => {
                      event.stopPropagation()
                      setForkMenuOpen(false)
                      onForkChat(chat.chatId)
                    }}
                  >
                    <span>{t.common.fork}</span>
                    <Split className="size-3.5 text-muted-foreground" />
                  </button>
                  {(["claude", "codex"] as const).map((provider) => {
                    const label = provider === "claude" ? "Claude" : "Codex"
                    return (
                      <button
                        key={provider}
                        className="flex items-center justify-between rounded-lg px-3 py-2 text-xs font-medium hover:bg-muted"
                        onClick={(event) => {
                          event.stopPropagation()
                          setForkMenuOpen(false)
                          onConvertChat(chat.chatId, provider)
                        }}
                      >
                        <span>{t.sidebar.forkToProvider(label)}</span>
                        <ChevronDown className="size-3.5 rotate-[-90deg] text-muted-foreground" />
                      </button>
                    )
                  })}
                </div>
              </PopoverContent>
            </Popover>
          ) : null}
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 cursor-pointer rounded-sm hover:!bg-transparent !border-0"
            onClick={(event) => {
              event.stopPropagation()
              onArchiveChat(chat.chatId)
            }}
            title={t.sidebar.archiveChat}
          >
            <Archive className="size-3.5" />
          </Button>
        </div>
      </div>
    </div>
  )

  return (
    <ChatRowMenu
      canFork={chat.canFork}
      onRename={() => onRenameChat(chat.chatId)}
      onOpenInNewTab={() => onOpenInNewTab(chat.chatId)}
      onOpenInFinder={() => onOpenInFinder(chat.localPath)}
      onFork={() => onForkChat(chat.chatId)}
      onConvert={(provider) => onConvertChat(chat.chatId, provider)}
      onArchive={() => onArchiveChat(chat.chatId)}
      onDelete={() => onDeleteChat(chat.chatId)}
    >
      {row}
    </ChatRowMenu>
  )
}

export const ChatRow = memo(ChatRowImpl)
