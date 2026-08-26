import { LegendList, type LegendListRef } from "@legendapp/list/react"
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { ArrowDown, ArrowUp, BookOpen, Loader2, Upload } from "lucide-react"
import { AnimatedShinyText } from "../../components/ui/animated-shiny-text"
import { AbolqasemLogo } from "../../components/AbolqasemLogo"
import { ConversationMinimap, type MessageIndexItem } from "../../components/chat-ui/ConversationMinimap"
import { DrainingIndicator } from "../../components/messages/DrainingIndicator"
import { QueuedUserMessage } from "../../components/messages/QueuedUserMessage"
import { OpenLocalLinkProvider, type OpenLocalLinkTarget } from "../../components/messages/shared"
import { ProcessingMessage } from "../../components/messages/ProcessingMessage"
import { ContextMenu, ContextMenuTrigger } from "../../components/ui/context-menu"
import { Dialog, DialogBody, DialogContent } from "../../components/ui/dialog"
import { OpenExternalContextMenuContent } from "../../components/open-external-menu"
import { FilePreviewPanel, type FilePreviewResponse } from "../../components/file-preview/FilePreviewPanel"
import { ReaderDialog } from "../../components/messages/ReaderDialog"
import { getAppearanceTextStyle, isDarkAppearanceTheme, useReaderAppearanceSettings } from "../../components/appearance/ReaderAppearance"
import { cn } from "../../lib/utils"
import {
  buildResolvedTranscriptRows,
  AbolqasemTranscriptRow,
  PromptCheckpointProvider,
  type ResolvedTranscriptRow,
  useStableResolvedRows,
} from "../AbolqasemTranscript"
import type { AbolqasemState } from "../useAbolqasemState"
import {
  CHAT_NAVBAR_OFFSET_PX,
} from "./utils"
import { READER_MODE_CHANGE_EVENT } from "../chatFocusPolicy"
import { buildAssistantReaderDocument, type AssistantReaderDocument } from "./readerBlocks"
import type { EditorPreset } from "../../../shared/protocol"
import type { ChatCheckpointSummary, HydratedTranscriptMessage } from "../../../shared/types"
import { useI18n } from "../../i18n/context"
import { getProcessingStatus } from "./processingStatus"

const CHECKPOINT_PROMPT_PREVIEW_MAX = 120
const PROMPT_CHECKPOINT_MAX_DELAY_MS = 30 * 60 * 1000
const TRANSCRIPT_JUMP_HINT_TIMEOUT_MS = 2600

export function shouldShowTranscriptJumpHint(previousTop: number, nextTop: number, elapsedMs: number) {
  return elapsedMs > 0 && elapsedMs <= 140 && Math.abs(nextTop - previousTop) >= 220
}

export function transcriptJumpDistance(viewportHeight: number, direction: "up" | "down") {
  const amount = Math.max(280, Math.round(viewportHeight * 0.72))
  return direction === "up" ? -amount : amount
}

function checkpointPromptPreview(value: string) {
  const normalized = value.trim().split(/\s+/).filter(Boolean).join(" ")
  return Array.from(normalized).slice(0, CHECKPOINT_PROMPT_PREVIEW_MAX).join("")
}

function getMessageCreatedAt(message: HydratedTranscriptMessage) {
  const createdAt = Date.parse(message.timestamp)
  return Number.isFinite(createdAt) ? createdAt : null
}

function isCheckpointNearPrompt(checkpoint: ChatCheckpointSummary, createdAt: number | null) {
  if (createdAt === null) return true
  return checkpoint.createdAt <= createdAt + 1000
    && createdAt <= checkpoint.createdAt + PROMPT_CHECKPOINT_MAX_DELAY_MS
}

function buildPromptCheckpointMap(
  messages: HydratedTranscriptMessage[],
  checkpoints: ChatCheckpointSummary[],
  activeChatId: string | null,
) {
  const result = new Map<string, ChatCheckpointSummary>()
  if (!activeChatId || checkpoints.length === 0) {
    return result
  }

  const promptCheckpoints = checkpoints
    .filter((checkpoint) => (
      checkpoint.chatId === activeChatId
      && checkpoint.trigger === "before_user_prompt"
      && !checkpoint.restoreOf
    ))
    .sort((left, right) => left.createdAt - right.createdAt)
  const promptMessages = messages
    .filter((message) => message.kind === "user_prompt")
    .map((message) => ({
      message,
      createdAt: getMessageCreatedAt(message),
      preview: checkpointPromptPreview(message.content),
    }))
  const usedMessageIds = new Set<string>()

  for (const checkpoint of promptCheckpoints) {
    const promptPreview = checkpointPromptPreview(checkpoint.promptPreview ?? "")
    const exactPrompt = promptMessages.find(({ message, createdAt, preview }) => (
      !usedMessageIds.has(message.id)
      && promptPreview.length > 0
      && preview === promptPreview
      && isCheckpointNearPrompt(checkpoint, createdAt)
    ))
    const fallbackPrompt = exactPrompt ?? promptMessages.find(({ message, createdAt }) => (
      !usedMessageIds.has(message.id)
      && isCheckpointNearPrompt(checkpoint, createdAt)
    ))
    if (!fallbackPrompt) continue

    usedMessageIds.add(fallbackPrompt.message.id)
    result.set(fallbackPrompt.message.id, checkpoint)
  }

  return result
}

function buildLiveMinimapItems(messages: HydratedTranscriptMessage[]): MessageIndexItem[] {
  return messages
    .map((message, sequence): MessageIndexItem | null => {
      if (message.kind !== "user_prompt") return null
      return {
        id: message.id,
        sequence,
        role: "user",
        loaded: true,
        preview: message.content.trim().slice(0, CHECKPOINT_PROMPT_PREVIEW_MAX),
      }
    })
    .filter((item): item is MessageIndexItem => Boolean(item))
}

type TranscriptViewportRow = ResolvedTranscriptRow & {
  promptCheckpoint?: ChatCheckpointSummary
}

interface ChatTranscriptViewportProps {
  activeChatId: string | null
  listRef: React.RefObject<LegendListRef | null>
  messages: AbolqasemState["messages"]
  queuedMessages: AbolqasemState["queuedMessages"]
  transcriptPaddingBottom: number
  localPath: string | null | undefined
  latestToolIds: AbolqasemState["latestToolIds"]
  isHistoryLoading: boolean
  hasOlderHistory: boolean
  isProcessing: boolean
  hasTmuxRuntime?: boolean
  runtimeStatus: string | null
  isDraining: boolean
  commandError: string | null
  loadOlderHistory: () => Promise<void>
  onStopDraining: () => void
  onRemoveQueuedMessage: (queuedMessageId: string) => Promise<void>
  onSteerQueuedMessage: (queuedMessageId: string) => Promise<void>
  onEditQueuedMessage: (queuedMessageId: string) => Promise<void>
  onOpenLocalLink: AbolqasemState["handleOpenLocalLink"]
  onAskUserQuestionSubmit: AbolqasemState["handleAskUserQuestion"]
  onApprovalRequestSubmit: AbolqasemState["handleApprovalRequest"]
  onExitPlanModeConfirm: AbolqasemState["handleExitPlanMode"]
  checkpoints?: ChatCheckpointSummary[]
  onRestoreCheckpoint?: AbolqasemState["handleRestoreCheckpoint"]
  showScrollButton: boolean
  showUnreadDot: boolean
  onIsAtEndChange: (isAtEnd: boolean) => void
  scrollToBottom: () => void
  typedEmptyStateText: string
  isEmptyStateTypingComplete: boolean
  isPageFileDragActive: boolean
  showEmptyState: boolean
  emptyStateText: string
  editorPreset?: EditorPreset
  editorCommandTemplate?: string
  platform?: NodeJS.Platform
  headerOffsetPx?: number
  onMinimapScrollToMessage?: (item: MessageIndexItem) => Promise<void>
}

export const ChatTranscriptViewport = memo(function ChatTranscriptViewport({
  activeChatId,
  listRef,
  messages,
  queuedMessages,
  transcriptPaddingBottom,
  localPath,
  latestToolIds,
  isHistoryLoading,
  hasOlderHistory,
  isProcessing,
  hasTmuxRuntime = false,
  runtimeStatus,
  isDraining,
  commandError,
  loadOlderHistory,
  onStopDraining,
  onRemoveQueuedMessage,
  onSteerQueuedMessage,
  onEditQueuedMessage,
  onOpenLocalLink,
  onAskUserQuestionSubmit,
  onApprovalRequestSubmit,
  onExitPlanModeConfirm,
  checkpoints = [],
  onRestoreCheckpoint,
  showScrollButton,
  showUnreadDot,
  onIsAtEndChange,
  scrollToBottom,
  typedEmptyStateText,
  isEmptyStateTypingComplete,
  isPageFileDragActive,
  showEmptyState,
  emptyStateText,
  editorPreset = "cursor",
  editorCommandTemplate,
  platform = "darwin",
  headerOffsetPx = CHAT_NAVBAR_OFFSET_PX,
  onMinimapScrollToMessage,
}: ChatTranscriptViewportProps) {
  const { t, direction } = useI18n()
  const [appearanceSettings] = useReaderAppearanceSettings()
  const previousRowCountRef = useRef(0)
  const localLinkMenuTriggerRef = useRef<HTMLSpanElement | null>(null)
  const [toolGroupExpanded, setToolGroupExpanded] = useState<Record<string, boolean>>({})
  const [localLinkMenuTarget, setLocalLinkMenuTarget] = useState<OpenLocalLinkTarget | null>(null)
  const [filePreviewTarget, setFilePreviewTarget] = useState<OpenLocalLinkTarget | null>(null)
  const [filePreview, setFilePreview] = useState<FilePreviewResponse | null>(null)
  const [filePreviewLoading, setFilePreviewLoading] = useState(false)
  const [filePreviewError, setFilePreviewError] = useState<string | null>(null)
  const [readerDocument, setReaderDocument] = useState<AssistantReaderDocument | null>(null)
  const [floatingReaderMessageId, setFloatingReaderMessageId] = useState<string | null>(null)
  const [showTranscriptJumpHint, setShowTranscriptJumpHint] = useState(false)
  const scrollHintTimeoutRef = useRef<number | null>(null)
  const lastScrollSampleRef = useRef<{ top: number; time: number } | null>(null)
  const isMac = platform === "darwin"
  const transcriptAppearanceStyle = useMemo(() => getAppearanceTextStyle(appearanceSettings), [appearanceSettings])
  const transcriptAppearanceClassName = useMemo(() => cn(
    "appearance-content reader-article",
    isDarkAppearanceTheme(appearanceSettings.theme) && "prose-invert",
  ), [appearanceSettings.theme])
  const rawRows = useMemo(() => buildResolvedTranscriptRows(messages, {
    isLoading: isProcessing,
    localPath: localPath ?? undefined,
    latestToolIds,
  }), [isProcessing, latestToolIds, localPath, messages])
  const resolvedRows = useStableResolvedRows(rawRows)
  const promptCheckpointByMessageId = useMemo(
    () => buildPromptCheckpointMap(messages, checkpoints, activeChatId),
    [activeChatId, checkpoints, messages],
  )
  const rowsWithCheckpoints = useMemo<TranscriptViewportRow[]>(() => {
    if (promptCheckpointByMessageId.size === 0) {
      return resolvedRows
    }

    return resolvedRows.map((row) => {
      const promptCheckpoint = row.kind === "single" && row.message.kind === "user_prompt"
        ? promptCheckpointByMessageId.get(row.message.id)
        : undefined
      return promptCheckpoint ? { ...row, promptCheckpoint } : row
    })
  }, [promptCheckpointByMessageId, resolvedRows])
  const checkpointRenderVersion = useMemo(
    () => Array.from(promptCheckpointByMessageId.entries())
      .map(([messageId, checkpoint]) => `${messageId}:${checkpoint.id}`)
      .join("|"),
    [promptCheckpointByMessageId],
  )
  const listExtraData = useMemo(() => ({
    appearanceSettings,
    checkpointRenderVersion,
    toolGroupExpanded,
  }), [appearanceSettings, checkpointRenderVersion, toolGroupExpanded])
  const floatingReaderDocument = useMemo(
    () => buildAssistantReaderDocument(messages, floatingReaderMessageId),
    [floatingReaderMessageId, messages],
  )
  const minimapItems = useMemo<MessageIndexItem[]>(() => buildLiveMinimapItems(messages), [messages])

  useEffect(() => {
    setToolGroupExpanded({})
    setReaderDocument(null)
    setFloatingReaderMessageId(null)
  }, [activeChatId])

  useEffect(() => {
    return () => {
      if (scrollHintTimeoutRef.current !== null) window.clearTimeout(scrollHintTimeoutRef.current)
    }
  }, [])

  useEffect(() => {
    if (typeof window === "undefined") return
    window.dispatchEvent(new CustomEvent(READER_MODE_CHANGE_EVENT, {
      detail: { open: Boolean(readerDocument) },
    }))
  }, [readerDocument])

  useEffect(() => {
    return () => {
      if (typeof window === "undefined") return
      window.dispatchEvent(new CustomEvent(READER_MODE_CHANGE_EVENT, {
        detail: { open: false },
      }))
    }
  }, [])

  useEffect(() => {
    const previousRowCount = previousRowCountRef.current
    previousRowCountRef.current = rowsWithCheckpoints.length

    if (previousRowCount > 0 || rowsWithCheckpoints.length === 0) {
      return
    }

    onIsAtEndChange(true)
    const frameId = window.requestAnimationFrame(() => {
      void listRef.current?.scrollToEnd?.({ animated: false })
    })
    return () => window.cancelAnimationFrame(frameId)
  }, [listRef, onIsAtEndChange, rowsWithCheckpoints.length])

  const updateFloatingReaderMessage = useCallback(() => {
    const scrollNode = listRef.current?.getScrollableNode?.()
    if (!(scrollNode instanceof HTMLElement)) {
      setFloatingReaderMessageId(null)
      return
    }

    const scrollRect = scrollNode.getBoundingClientRect()
    const followY = scrollRect.bottom - Math.max(72, transcriptPaddingBottom + 22)
    const rows = Array.from(scrollNode.querySelectorAll<HTMLElement>("[data-reader-message-id]"))
    let nextMessageId: string | null = null

    for (const row of rows) {
      const rect = row.getBoundingClientRect()
      const messageId = row.dataset.readerMessageId
      if (!messageId) continue

      const rowContainsFollowLine = rect.top <= followY && rect.bottom >= followY
      const mainButtonStillBelowViewport = rect.bottom > scrollRect.bottom - Math.max(96, transcriptPaddingBottom + 44)
      const rowStillReadable = rect.bottom > scrollRect.top + headerOffsetPx + 80
      if (rowContainsFollowLine && mainButtonStillBelowViewport && rowStillReadable) {
        nextMessageId = messageId
        break
      }
    }

    setFloatingReaderMessageId((current) => current === nextMessageId ? current : nextMessageId)
  }, [headerOffsetPx, listRef, transcriptPaddingBottom])

  const handleToolGroupExpandedChange = useCallback((groupId: string, next: boolean) => {
    setToolGroupExpanded((current) => (
      current[groupId] === next
        ? current
        : {
            ...current,
            [groupId]: next,
          }
    ))
  }, [])

  const handleScroll = useCallback((event?: unknown) => {
    const currentTarget = (
      typeof event === "object"
      && event !== null
      && "currentTarget" in event
      && event.currentTarget instanceof HTMLElement
    )
      ? event.currentTarget
      : listRef.current?.getScrollableNode?.()

    if (currentTarget instanceof HTMLElement) {
      const now = performance.now()
      const previousSample = lastScrollSampleRef.current
      if (previousSample && shouldShowTranscriptJumpHint(previousSample.top, currentTarget.scrollTop, now - previousSample.time)) {
        setShowTranscriptJumpHint(true)
        if (scrollHintTimeoutRef.current !== null) window.clearTimeout(scrollHintTimeoutRef.current)
        scrollHintTimeoutRef.current = window.setTimeout(() => {
          setShowTranscriptJumpHint(false)
          scrollHintTimeoutRef.current = null
        }, TRANSCRIPT_JUMP_HINT_TIMEOUT_MS)
      }
      lastScrollSampleRef.current = { top: currentTarget.scrollTop, time: now }
      const distanceFromEnd = currentTarget.scrollHeight - currentTarget.clientHeight - currentTarget.scrollTop
      onIsAtEndChange(distanceFromEnd <= 4)
      return
    }

    const state = listRef.current?.getState?.()
    if (state) {
      onIsAtEndChange(state.isAtEnd)
    }
  }, [listRef, onIsAtEndChange])

  useEffect(() => {
    const handleJumpShortcut = (event: KeyboardEvent) => {
      if (!event.shiftKey || event.altKey || event.ctrlKey || event.metaKey || (event.key !== "ArrowUp" && event.key !== "ArrowDown")) return
      const target = event.target instanceof HTMLElement ? event.target : null
      if (target?.matches("input, textarea, select, [contenteditable='true']")) return
      const scrollNode = listRef.current?.getScrollableNode?.()
      if (!(scrollNode instanceof HTMLElement)) return
      event.preventDefault()
      scrollNode.scrollBy({
        top: transcriptJumpDistance(scrollNode.clientHeight, event.key === "ArrowUp" ? "up" : "down"),
        behavior: "smooth",
      })
    }
    window.addEventListener("keydown", handleJumpShortcut)
    return () => window.removeEventListener("keydown", handleJumpShortcut)
  }, [listRef])

  useEffect(() => {
    let cleanup: (() => void) | undefined
    const frameId = window.requestAnimationFrame(() => {
      const scrollNode = listRef.current?.getScrollableNode?.()
      if (!(scrollNode instanceof HTMLElement)) {
        return
      }

      const handleNativeScroll = () => {
        handleScroll({ currentTarget: scrollNode })
        updateFloatingReaderMessage()
      }

      scrollNode.addEventListener("scroll", handleNativeScroll, { passive: true })
      window.addEventListener("resize", updateFloatingReaderMessage)
      handleNativeScroll()
      cleanup = () => {
        scrollNode.removeEventListener("scroll", handleNativeScroll)
        window.removeEventListener("resize", updateFloatingReaderMessage)
      }
    })

    return () => {
      window.cancelAnimationFrame(frameId)
      cleanup?.()
    }
  }, [activeChatId, handleScroll, listRef, resolvedRows.length, updateFloatingReaderMessage])

  const handleStartReached = useCallback(() => {
    if (isHistoryLoading || !hasOlderHistory) {
      return
    }
    void loadOlderHistory()
  }, [hasOlderHistory, isHistoryLoading, loadOlderHistory])

  const handleOpenLocalLinkClick = useCallback((target: OpenLocalLinkTarget) => {
    if (target.trigger !== "contextmenu") {
      setFilePreviewTarget(target)
      return
    }

    setLocalLinkMenuTarget(target)
    window.requestAnimationFrame(() => {
      const trigger = localLinkMenuTriggerRef.current
      if (!trigger) return
      const clientX = target.clientX ?? window.innerWidth / 2
      const clientY = target.clientY ?? window.innerHeight / 2
      trigger.dispatchEvent(new MouseEvent("contextmenu", {
        bubbles: true,
        cancelable: true,
        clientX,
        clientY,
        view: window,
      }))
    })
  }, [])

  useEffect(() => {
    if (!filePreviewTarget) {
      setFilePreview(null)
      setFilePreviewError(null)
      setFilePreviewLoading(false)
      return
    }

    const controller = new AbortController()
    const params = new URLSearchParams({
      path: filePreviewTarget.path,
      full: "1",
    })
    if (filePreviewTarget.line && filePreviewTarget.line > 0) {
      params.set("line", String(filePreviewTarget.line))
    }

    setFilePreviewLoading(true)
    setFilePreviewError(null)
    fetch(`/api/file-preview?${params.toString()}`, {
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
        if (!controller.signal.aborted) setFilePreview(payload)
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        setFilePreview(null)
        setFilePreviewError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        if (!controller.signal.aborted) setFilePreviewLoading(false)
      })

    return () => {
      controller.abort()
    }
  }, [filePreviewTarget])

  const renderItem = useCallback(({ item }: { item: TranscriptViewportRow }) => {
    const readerMessageId = item.kind === "single" && item.message.kind === "assistant_text"
      ? item.message.id
      : undefined

    return (
    <div
      className={cn("mx-auto w-full max-w-[800px] pb-5", transcriptAppearanceClassName)}
      dir={direction}
      style={transcriptAppearanceStyle}
      data-transcript-row-id={item.id}
      data-reader-message-id={readerMessageId}
    >
      <AbolqasemTranscriptRow
        row={item}
        toolGroupExpanded={item.kind === "tool-group" ? (toolGroupExpanded[item.id] ?? false) : undefined}
        onToolGroupExpandedChange={handleToolGroupExpandedChange}
        onAskUserQuestionSubmit={onAskUserQuestionSubmit}
        onApprovalRequestSubmit={onApprovalRequestSubmit}
        onExitPlanModeConfirm={onExitPlanModeConfirm}
        promptCheckpoint={item.kind === "single" && item.message.kind === "user_prompt" ? item.promptCheckpoint : undefined}
        onRestoreCheckpoint={onRestoreCheckpoint}
      />
    </div>
    )
  }, [direction, handleToolGroupExpandedChange, onApprovalRequestSubmit, onAskUserQuestionSubmit, onExitPlanModeConfirm, onRestoreCheckpoint, toolGroupExpanded, transcriptAppearanceClassName, transcriptAppearanceStyle])

  const listHeader = (
    <div className="mx-auto w-full max-w-[800px]" dir={direction} style={{ paddingTop: `${headerOffsetPx}px` }}>
      {isHistoryLoading ? (
        <div className="flex justify-center pb-4">
          <span className="text-sm translate-y-[-0.5px]">
            <AnimatedShinyText
              animate
              shimmerWidth={Math.max(20, t.chat.loadingMoreMessages.length * 3)}
            >
              {t.chat.loadingMoreMessages}
            </AnimatedShinyText>
          </span>
        </div>
      ) : null}
    </div>
  )

  const processingStatus = getProcessingStatus(messages, runtimeStatus ?? undefined)
  const listFooter = (
    <div className={cn("mx-auto w-full max-w-[800px]", transcriptAppearanceClassName)} dir={direction} style={transcriptAppearanceStyle}>
      {isProcessing && !hasTmuxRuntime ? <ProcessingMessage status={processingStatus} /> : null}
      {queuedMessages.map((message) => (
        <QueuedUserMessage
          key={message.id}
          message={message}
          onRemove={() => onRemoveQueuedMessage(message.id)}
          onSteer={() => onSteerQueuedMessage(message.id)}
          onEdit={() => onEditQueuedMessage(message.id)}
        />
      ))}
      {!isProcessing && isDraining ? (
        <DrainingIndicator onStop={() => void onStopDraining()} />
      ) : null}
      {commandError ? (
        <div className="rounded-xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          {commandError}
        </div>
      ) : null}
    </div>
  )
  const showFloatingReader = Boolean(floatingReaderDocument) && !readerDocument

  return (
    <>
      <OpenLocalLinkProvider onOpenLocalLink={handleOpenLocalLinkClick}>
        <PromptCheckpointProvider
          checkpointByMessageId={promptCheckpointByMessageId}
          onRestoreCheckpoint={onRestoreCheckpoint}
        >
          <div className="relative flex min-h-0 flex-1">
            <LegendList<TranscriptViewportRow>
              ref={listRef}
              data={rowsWithCheckpoints}
              extraData={listExtraData}
              keyExtractor={keyExtractor}
              renderItem={renderItem}
              estimatedItemSize={96}
              initialScrollAtEnd
              maintainScrollAtEnd
              maintainScrollAtEndThreshold={0.1}
              maintainVisibleContentPosition
              onScroll={handleScroll}
              onStartReached={handleStartReached}
              onStartReachedThreshold={0.1}
              className="h-full flex-1 overflow-x-hidden overscroll-y-contain px-3 scroll-pt-[72px] [direction:ltr] [scrollbar-gutter:auto]"
              contentContainerStyle={{ paddingBottom: transcriptPaddingBottom + 10 }}
              ListHeaderComponent={listHeader}
              ListFooterComponent={listFooter}
            />
            {(minimapItems.length > 0 || hasOlderHistory) && onMinimapScrollToMessage ? (
              <ConversationMinimap
                items={minimapItems}
                hasOlderItems={hasOlderHistory}
                onScrollToMessage={onMinimapScrollToMessage}
                side={direction === "rtl" ? "left" : "right"}
                topOffsetPx={headerOffsetPx + 10}
                bottomOffsetPx={Math.max(12, transcriptPaddingBottom + 10)}
              />
            ) : null}
          </div>
        </PromptCheckpointProvider>
      </OpenLocalLinkProvider>

      <Dialog open={Boolean(filePreviewTarget)} onOpenChange={(open) => {
        if (!open) {
          setFilePreviewTarget(null)
        }
      }}>
        <DialogContent hideClose className="w-[min(92vw,1040px)] max-w-none rounded-3xl p-0">
          <DialogBody className="p-0">
            {filePreviewLoading ? (
              <div className="flex min-h-[360px] items-center justify-center gap-3 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>{t.common.loading}…</span>
              </div>
            ) : filePreviewError ? (
              <div className="m-4 rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                {filePreviewError}
              </div>
            ) : filePreview ? (
              <FilePreviewPanel
                preview={filePreview}
                compact
                showOpenRoute
                onClose={() => setFilePreviewTarget(null)}
                className="border-0"
              />
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>

      <ReaderDialog
        open={Boolean(readerDocument)}
        title={readerDocument?.title ?? "Reader"}
        content={readerDocument?.content ?? ""}
        onOpenChange={(open) => {
          if (!open) setReaderDocument(null)
        }}
      />

      <ContextMenu onOpenChange={(open) => {
        if (!open) {
          setLocalLinkMenuTarget(null)
        }
      }}>
        <ContextMenuTrigger asChild>
          <span
            ref={localLinkMenuTriggerRef}
            aria-hidden="true"
            className="pointer-events-none fixed size-px opacity-0"
            style={{
              left: localLinkMenuTarget?.clientX ?? 0,
              top: localLinkMenuTarget?.clientY ?? 0,
            }}
          />
        </ContextMenuTrigger>
        {localLinkMenuTarget ? (
          <OpenExternalContextMenuContent
            isMac={isMac}
            editorPreset={editorPreset}
            editorCommandTemplate={editorCommandTemplate}
            includeFinder
            includePreview
            includeDefault
            onOpenExternal={(action, editor) => {
              void onOpenLocalLink(localLinkMenuTarget, action, editor)
            }}
          />
        ) : null}
      </ContextMenu>

      {showEmptyState ? (
        <div
          className="pointer-events-none absolute inset-x-4 animate-fade-in"
          style={{
            top: headerOffsetPx,
            bottom: transcriptPaddingBottom,
          }}
        >
          <div className="mx-auto flex h-full max-w-[800px] items-center justify-center">
            <div className="flex flex-col items-center justify-center gap-4 text-muted-foreground opacity-70">
              <AbolqasemLogo className="abolqasem-empty-state-flower size-8 text-muted-foreground" />
              <div
                className="abolqasem-empty-state-text flex max-w-xs items-center text-center text-base font-normal text-muted-foreground"
                aria-label={emptyStateText}
              >
                <span className="relative inline-grid place-items-start">
                  <span className="invisible col-start-1 row-start-1 flex items-center whitespace-pre">
                    <span>{emptyStateText}</span>
                    <span className="abolqasem-typewriter-cursor-slot" aria-hidden="true" />
                  </span>
                  <span className="col-start-1 row-start-1 flex items-center whitespace-pre">
                    <span>{typedEmptyStateText}</span>
                    <span className="abolqasem-typewriter-cursor-slot" aria-hidden="true">
                      <span
                        className="abolqasem-typewriter-cursor"
                        data-typing-complete={isEmptyStateTypingComplete ? "true" : "false"}
                      />
                    </span>
                  </span>
                </span>
              </div>
            </div>
          </div>
        </div>
      ) : null}

      {isPageFileDragActive ? (
        <div className="pointer-events-none absolute inset-0 z-30">
          <div className="absolute inset-0 backdrop-blur-sm" />
          <div className="absolute inset-6 ">
            <div className="flex h-full items-center justify-center">
              <div className="flex flex-col items-center justify-center gap-3 text-center">
                <Upload className="mx-auto size-14 text-foreground" strokeWidth={1.75} />
                <div className="text-xl font-medium text-foreground">{t.chat.dropFiles}</div>
              </div>
            </div>
          </div>
        </div>
      ) : null}

      <div
        className={cn(
          "pointer-events-none absolute end-5 top-1/2 z-20 -translate-y-1/2 transition-all duration-200",
          showTranscriptJumpHint ? "translate-x-0 opacity-100" : "translate-x-2 opacity-0",
        )}
        aria-hidden={!showTranscriptJumpHint}
      >
        <div className="rounded-lg border border-border/80 bg-background/88 px-2.5 py-2 text-[11px] text-muted-foreground shadow-lg backdrop-blur-md" dir={direction}>
          <div className="flex items-center gap-1.5 whitespace-nowrap">
            <kbd className="rounded border border-border bg-muted/60 px-1 font-mono text-[10px] text-foreground">Shift</kbd>
            <ArrowUp className="size-3" />
            <ArrowDown className="size-3" />
          </div>
          <div className="mt-1">{direction === "rtl" ? "پرش سریع در گفت‌وگو" : "Jump through the conversation"}</div>
        </div>
      </div>

      <div
        style={{ bottom: transcriptPaddingBottom - 20 }}
        className={cn(
          "absolute left-1/2 z-10 -translate-x-1/2 transition-all",
          showScrollButton
            ? "scale-100 duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)]"
            : "pointer-events-none scale-60 opacity-0 blur-sm duration-300 ease-out",
        )}
      >
        <button
          onClick={scrollToBottom}
          className="relative flex aspect-square cursor-pointer items-center gap-1.5 rounded-full border border-border bg-white px-2 text-sm text-primary transition-colors hover:bg-muted hover:text-foreground dark:border-slate-600 dark:bg-slate-700 dark:text-slate-100 dark:hover:bg-slate-600"
        >
          <ArrowDown className="h-5 w-5" />
          {showUnreadDot ? (
            <span
              aria-hidden="true"
              className="absolute end-1 top-1 size-2 rounded-full bg-primary shadow-[0_0_0_2px_theme(colors.white)] dark:shadow-[0_0_0_2px_theme(colors.slate.700)]"
            />
          ) : null}
        </button>
      </div>

      <div
        style={{
          bottom: transcriptPaddingBottom - 20,
          left: "max(14px, calc(50% - 456px))",
        }}
        className={cn(
          "absolute z-10 transition-all duration-300 ease-[cubic-bezier(0.2,1.35,0.32,1)]",
          showFloatingReader
            ? "scale-100 opacity-100 blur-0"
            : "pointer-events-none translate-y-2 scale-75 opacity-0 blur-sm",
        )}
      >
        <button
          type="button"
          onClick={() => {
            if (floatingReaderDocument) setReaderDocument(floatingReaderDocument)
          }}
          className="relative flex h-10 w-10 cursor-pointer items-center justify-center rounded-full border border-border bg-background/95 text-muted-foreground shadow-[0_16px_44px_rgba(0,0,0,0.18)] transition-colors before:absolute before:inset-1 before:-z-10 before:rounded-full before:bg-primary/15 before:blur-lg hover:text-foreground dark:bg-slate-800/95"
          aria-label="Open reader"
        >
          <BookOpen className="h-4 w-4" />
        </button>
      </div>
    </>
  )
})

function keyExtractor(item: TranscriptViewportRow) {
  return item.id
}
