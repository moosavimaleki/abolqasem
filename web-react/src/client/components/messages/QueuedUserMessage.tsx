import { useState } from "react"
import type { QueuedChatMessage } from "../../../shared/types"
import { Button } from "../ui/button"
import { Loader2, MessageSquare, Paperclip, Pencil, Send, Square, Trash2 } from "lucide-react"
import { useI18n } from "../../i18n/context"

interface QueuedUserMessageProps {
  message: QueuedChatMessage
  onRemove: () => Promise<void>
  onSteer: () => Promise<void>
  onInterrupt: () => Promise<void>
  onEdit: () => Promise<void>
}

export function QueuedUserMessage({ message, onRemove, onSteer, onInterrupt, onEdit }: QueuedUserMessageProps) {
  const { t } = useI18n()
  const [pendingAction, setPendingAction] = useState<"edit" | "steer" | "interrupt" | "remove" | null>(null)
  const isSteering = message.deliveryState === "steering"
  const isSubmitting = message.deliveryState === "submitting"
  const steerPending = pendingAction === "steer"
  const interruptPending = pendingAction === "interrupt"

  async function runAction(action: "edit" | "steer" | "interrupt" | "remove", operation: () => Promise<void>) {
    if (pendingAction || isSteering || isSubmitting) return
    setPendingAction(action)
    try {
      await operation()
    } catch {
      // The owning chat state renders the command error. Keep the row available for retry.
      setPendingAction(null)
      return
    }
    // Steer is acknowledged before the next snapshot arrives. Keep the
    // controls disabled and show a spinner until the server marks delivery as
    // steering (or removes the row after transcript reconciliation).
    if (action !== "steer") {
      setPendingAction(null)
    }
  }

  const preview = message.content.trim() || message.attachments.map((attachment) => attachment.displayName).join(", ") || "Queued message"
  return (
    <div className="flex min-h-11 w-full justify-center border-b border-border bg-muted/60 px-3 py-1.5 first:rounded-t-2xl" data-delivery-state={message.deliveryState ?? "queued"}>
      <div className="flex min-w-0 w-full items-center gap-2 text-sm">
        <MessageSquare className="size-4 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-start text-foreground" dir="auto">{preview}</span>
        {message.attachments.length > 0 ? <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground"><Paperclip className="size-3.5" />{message.attachments.length}</span> : null}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {isSubmitting || interruptPending ? <span className="inline-flex items-center gap-1 text-xs text-muted-foreground"><Loader2 className="size-3.5 animate-spin" />{t.chat.queueSubmitting}</span> : isSteering || steerPending ? <span className="inline-flex items-center gap-1 text-xs text-muted-foreground"><Loader2 className="size-3.5 animate-spin" />{t.chat.queueDelivering}</span> : <>
            <Button type="button" variant="outline" size="sm" disabled={pendingAction !== null} onClick={() => void runAction("edit", onEdit)}>{pendingAction === "edit" ? <Loader2 className="size-3.5 animate-spin" /> : <Pencil className="size-3.5" />}Edit</Button>
            <Button type="button" variant="outline" size="sm" disabled={pendingAction !== null} onClick={() => void runAction("steer", onSteer)}>{steerPending ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}{t.chat.queueSteer}</Button>
            <Button type="button" variant="outline" size="sm" disabled={pendingAction !== null} title={t.chat.queueInterrupt} onClick={() => void runAction("interrupt", onInterrupt)}>{interruptPending ? <Loader2 className="size-3.5 animate-spin" /> : <Square className="size-3.5" />}{t.chat.queueInterrupt}</Button>
            <Button type="button" variant="ghost" size="icon" disabled={pendingAction !== null} aria-label="Delete queued message" onClick={() => void runAction("remove", onRemove)}>{pendingAction === "remove" ? <Loader2 className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />}</Button>
          </>}
        </div>
      </div>
    </div>
  )
}
