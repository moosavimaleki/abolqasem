import { useState } from "react"
import type { QueuedChatMessage } from "../../../shared/types"
import { Button } from "../ui/button"
import { Loader2, MessageSquare, Paperclip, Pencil, Send, Trash2 } from "lucide-react"

interface QueuedUserMessageProps {
  message: QueuedChatMessage
  onRemove: () => Promise<void>
  onSteer: () => Promise<void>
  onEdit: () => Promise<void>
}

export function QueuedUserMessage({ message, onRemove, onSteer, onEdit }: QueuedUserMessageProps) {
  const [pendingAction, setPendingAction] = useState<"edit" | "steer" | "remove" | null>(null)
  const isSteering = message.deliveryState === "steering"

  async function runAction(action: "edit" | "steer" | "remove", operation: () => Promise<void>) {
    if (pendingAction || isSteering) return
    setPendingAction(action)
    try {
      await operation()
    } catch {
      // The owning chat state renders the command error. Keep the row available for retry.
    } finally {
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
          {isSteering ? <span className="inline-flex items-center gap-1 text-xs text-muted-foreground"><Loader2 className="size-3.5 animate-spin" />Delivering</span> : <>
            <Button type="button" variant="outline" size="sm" disabled={pendingAction !== null} onClick={() => void runAction("edit", onEdit)}>{pendingAction === "edit" ? <Loader2 className="size-3.5 animate-spin" /> : <Pencil className="size-3.5" />}Edit</Button>
            <Button type="button" variant="outline" size="sm" disabled={pendingAction !== null} onClick={() => void runAction("steer", onSteer)}>{pendingAction === "steer" ? <Loader2 className="size-3.5 animate-spin" /> : <Send className="size-3.5" />}Steer</Button>
            <Button type="button" variant="ghost" size="icon" disabled={pendingAction !== null} aria-label="Delete queued message" onClick={() => void runAction("remove", onRemove)}>{pendingAction === "remove" ? <Loader2 className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />}</Button>
          </>}
        </div>
      </div>
    </div>
  )
}
