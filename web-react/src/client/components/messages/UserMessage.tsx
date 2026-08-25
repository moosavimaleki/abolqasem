import { useMemo, useState, type ComponentType } from "react"
import type { ChatAttachment, ChatCheckpointSummary, CheckpointRestoreMode, CheckpointRestoreResult } from "../../../shared/types"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { Braces, ChevronRight, Code2, CornerUpLeft, FileText, Layers2, Loader2, RotateCcw } from "lucide-react"
import { createMarkdownComponents, MessageCopyButton } from "./shared"
import { classifyAttachmentPreview } from "./attachmentPreview"
import { AttachmentFileCard, AttachmentImageCard } from "./AttachmentCard"
import { AttachmentPreviewModal } from "./AttachmentPreviewModal"
import { useTranscriptRenderOptions } from "./render-context"
import { useI18n } from "../../i18n/context"
import { extractInternalSystemPayload } from "../../lib/parseTranscript"
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover"

interface Props {
  content: string
  attachments?: ChatAttachment[]
  steered?: boolean
  checkpoint?: ChatCheckpointSummary | null
  onRestoreCheckpoint?: (
    checkpointId: string,
    mode: CheckpointRestoreMode,
    promptContent: string
  ) => Promise<CheckpointRestoreResult | null>
}

interface RestoreOptionProps {
  icon: ComponentType<{ className?: string }>
  label: string
  disabled?: boolean
  loading?: boolean
  onClick: () => void
}

function RestoreOption({ icon: Icon, label, disabled, loading, onClick }: RestoreOptionProps) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled || loading}
      className="flex h-9 min-w-[4.25rem] items-center justify-center gap-1.5 rounded-md px-2 text-xs font-semibold text-foreground outline-none transition-colors hover:bg-muted focus-visible:bg-muted disabled:pointer-events-none disabled:opacity-40"
      onClick={onClick}
    >
      {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" /> : <Icon className="h-3.5 w-3.5 text-muted-foreground" />}
      <span className="whitespace-nowrap">{label}</span>
    </button>
  )
}

function parseSystemMessage(content: string) {
  const match = content.match(/^<system-message>\s*([\s\S]*?)\s*<\/system-message>\s*([\s\S]*)$/)
  if (!match) {
    return { systemMessage: null, body: content }
  }

  return {
    systemMessage: match[1]?.trim() || null,
    body: match[2] ?? "",
  }
}

function getCollapsedSystemPayload(content: string, wrappedSystemMessage: string | null, isPersian: boolean) {
  const trimmed = content.trim()
  if (wrappedSystemMessage) {
    return {
      label: isPersian ? "پیام سیستمی" : "System message",
      content: wrappedSystemMessage,
    }
  }

  const internalPayload = extractInternalSystemPayload(trimmed)
  if (internalPayload?.kind === "environment_context") {
    return {
      label: isPersian ? "زمینهٔ محیط اجرا" : "Environment context",
      content: internalPayload.payload,
    }
  }

  if (internalPayload?.kind === "turn_aborted") {
    return {
      label: isPersian ? "رخداد سیستمی: توقف turn" : "System event: turn aborted",
      content: internalPayload.payload,
    }
  }

  return null
}

function CollapsedSystemPayload({ label, content, direction }: { label: string; content: string; direction: "ltr" | "rtl" }) {
  return (
    <details className="group w-full max-w-[85%] overflow-hidden rounded-xl border border-border/70 bg-muted/25 text-xs text-muted-foreground sm:max-w-[80%]">
      <summary
        dir={direction}
        className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-start transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <ChevronRight className="size-3.5 shrink-0 transition-transform duration-200 group-open:rotate-90 rtl:rotate-180 rtl:group-open:rotate-90" />
        <Braces className="size-3.5 shrink-0" />
        <span className="font-medium">{label}</span>
        <span className="ms-auto text-[11px] text-muted-foreground/70">{direction === "rtl" ? "نمایش" : "Show"}</span>
      </summary>
      <pre dir="ltr" className="max-h-80 overflow-auto border-t border-border/60 bg-background/20 px-3 py-2.5 text-left text-[11px] leading-5 whitespace-pre-wrap break-words text-muted-foreground">
        {content}
      </pre>
    </details>
  )
}

export function UserMessage({ content, attachments = [], steered = false, checkpoint, onRestoreCheckpoint }: Props) {
  const { t, direction } = useI18n()
  const [selectedAttachmentId, setSelectedAttachmentId] = useState<string | null>(null)
  const [restoreOpen, setRestoreOpen] = useState(false)
  const [pendingRestoreMode, setPendingRestoreMode] = useState<CheckpointRestoreMode | null>(null)
  const renderOptions = useTranscriptRenderOptions()
  const parsedContent = useMemo(() => parseSystemMessage(content), [content])
  const collapsedSystemPayload = useMemo(
    () => getCollapsedSystemPayload(content, parsedContent.systemMessage, direction === "rtl"),
    [content, direction, parsedContent.systemMessage],
  )
  const shouldShowImagePlaceholders = renderOptions.attachmentMode === "metadata"
  const canInteractWithAttachments = !renderOptions.readonly || renderOptions.attachmentMode === "bundle"
  const canShowRestoreControl = Boolean(checkpoint && onRestoreCheckpoint)
  const codeSnapshotReady = checkpoint?.codeStatus === "ready" && checkpoint.codeKind !== "none"
  const imageAttachments = useMemo(
    () => attachments.filter((attachment) => attachment.kind === "image" && (attachment.contentUrl || shouldShowImagePlaceholders)),
    [attachments, shouldShowImagePlaceholders],
  )
  const fileAttachments = useMemo(
    () => attachments.filter((attachment) => attachment.kind !== "image" || (!attachment.contentUrl && !shouldShowImagePlaceholders)),
    [attachments, shouldShowImagePlaceholders],
  )
  const selectedAttachment = attachments.find((attachment) => attachment.id === selectedAttachmentId) ?? null

  function handleAttachmentClick(attachment: ChatAttachment) {
    if (!canInteractWithAttachments || !attachment.contentUrl) {
      return
    }

    const target = classifyAttachmentPreview(attachment)
    if (target.openInNewTab) {
      if (typeof window !== "undefined") {
        window.open(new URL(attachment.contentUrl, document.baseURI || window.location.href).toString(), "_blank", "noopener,noreferrer")
      }
      return
    }

    setSelectedAttachmentId(attachment.id)
  }

  async function handleRestore(mode: CheckpointRestoreMode) {
    if (!checkpoint || !onRestoreCheckpoint || pendingRestoreMode) return
    setPendingRestoreMode(mode)
    try {
      await onRestoreCheckpoint(checkpoint.id, mode, content)
      setRestoreOpen(false)
    } finally {
      setPendingRestoreMode(null)
    }
  }

  const restoreControl = canShowRestoreControl ? (
    <Popover open={restoreOpen} onOpenChange={setRestoreOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t.messages.restoreCheckpoint}
          title={t.messages.restoreCheckpoint}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-primary/30 bg-primary/10 text-primary shadow-sm backdrop-blur transition-colors hover:border-primary/45 hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        >
          <RotateCcw className="h-3.5 w-3.5" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        dir={direction}
        side="left"
        align="center"
        sideOffset={8}
        className="w-auto p-1.5"
      >
        <div className="grid grid-cols-3 gap-1">
          <RestoreOption
            icon={Code2}
            label={t.messages.restoreCodeOnly}
            disabled={pendingRestoreMode !== null || !codeSnapshotReady}
            loading={pendingRestoreMode === "code"}
            onClick={() => void handleRestore("code")}
          />
          <RestoreOption
            icon={FileText}
            label={t.messages.restoreChatOnly}
            disabled={pendingRestoreMode !== null}
            loading={pendingRestoreMode === "chat"}
            onClick={() => void handleRestore("chat")}
          />
          <RestoreOption
            icon={Layers2}
            label={t.messages.restoreCodeAndChat}
            disabled={pendingRestoreMode !== null || !codeSnapshotReady}
            loading={pendingRestoreMode === "code_and_chat"}
            onClick={() => void handleRestore("code_and_chat")}
          />
        </div>
      </PopoverContent>
    </Popover>
  ) : null

  return (
    <>
      <div className="flex flex-col items-end gap-2">
        {collapsedSystemPayload ? (
          <CollapsedSystemPayload {...collapsedSystemPayload} direction={direction} />
        ) : null}
        {imageAttachments.length > 0 ? (
          <div className="flex max-w-[85%] sm:max-w-[80%] flex-wrap justify-end gap-3">
            {imageAttachments.map((attachment) => (
              <AttachmentImageCard
                key={attachment.id}
                attachment={attachment}
                onClick={canInteractWithAttachments ? () => handleAttachmentClick(attachment) : undefined}
              />
            ))}
          </div>
        ) : null}
        {fileAttachments.length > 0 ? (
          <div className="flex max-w-[85%] sm:max-w-[80%] flex-wrap justify-end gap-2">
            {fileAttachments.map((attachment) => (
              <AttachmentFileCard
                key={attachment.id}
                attachment={attachment}
                onClick={canInteractWithAttachments ? () => handleAttachmentClick(attachment) : undefined}
              />
            ))}
          </div>
        ) : null}
        {(parsedContent.body || (!parsedContent.body && attachments.length === 0 && content && !parsedContent.systemMessage)) ? (
          <div className="group/message relative flex max-w-[85%] items-center gap-2 sm:max-w-[80%]" dir="ltr">
            <MessageCopyButton
              text={parsedContent.body}
              label={t.common.copyMessage}
              copiedLabel={t.common.copied}
              className="absolute -left-8 top-1/2 z-10 -translate-y-1/2"
            />
            {restoreControl}
            {steered ? (
              <span
                aria-label={t.messages.sentMidTurn}
                role="img"
                title={t.messages.sentMidTurn}
                className="shrink-0 text-muted-foreground"
              >
                <CornerUpLeft className="h-4 w-4" />
              </span>
            ) : null}
            <div className="min-w-0 flex-1 rounded-[20px] border border-border bg-muted px-3.5 py-1.5 text-primary prose prose-sm prose-invert [&_p]:whitespace-pre-line">
              <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source: parsedContent.body })}>{parsedContent.body}</Markdown>
            </div>
          </div>
        ) : null}
      </div>
      <AttachmentPreviewModal attachment={selectedAttachment} onOpenChange={(open) => !open && setSelectedAttachmentId(null)} />
    </>
  )
}
