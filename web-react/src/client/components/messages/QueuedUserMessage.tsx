import Markdown from "react-markdown"
import { useState } from "react"
import remarkGfm from "remark-gfm"
import type { QueuedChatMessage } from "../../../shared/types"
import { Button } from "../ui/button"
import { createMarkdownComponents } from "./shared"
import { Check, MessageSquare, Pencil, Send, Trash2, X } from "lucide-react"

interface QueuedUserMessageProps {
  message: QueuedChatMessage
  onRemove: () => void
  onSteer: () => void
  onEdit: (content: string) => Promise<void>
}

export function QueuedUserMessage({ message, onRemove, onSteer, onEdit }: QueuedUserMessageProps) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(message.content)
  const [saving, setSaving] = useState(false)

  async function saveEdit() {
    setSaving(true)
    try { await onEdit(draft); setEditing(false) } finally { setSaving(false) }
  }
  return (
    <div className="flex w-full justify-center border-b border-border bg-muted/60 px-3 py-2 first:rounded-t-2xl">
      <div className="flex w-full items-center gap-2 text-sm">
        <MessageSquare className="size-4 shrink-0 text-muted-foreground" />
        {message.attachments.length > 0 ? (
          <div className="flex shrink-0 flex-wrap gap-1">
            {message.attachments.map((attachment) => (
              <div
                key={attachment.id}
                className="max-w-[160px] rounded-md border border-border bg-background px-2 py-1 text-left"
              >
                <div className="truncate text-[13px] font-medium text-foreground">{attachment.displayName}</div>
                <div className="truncate text-[11px] text-muted-foreground">{attachment.mimeType}</div>
              </div>
            ))}
          </div>
        ) : null}
        {editing ? <textarea autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} className="min-h-9 min-w-0 flex-1 resize-y rounded-md border border-border bg-background px-2 py-1 outline-none focus:ring-2 focus:ring-ring" /> : message.content ? (
          <div className="min-w-0 flex-1 truncate">
            <div className="prose prose-sm prose-invert max-w-none truncate text-left text-primary [&_p]:m-0 [&_p]:truncate">
              <div>
                <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source: message.content })}>
                  {message.content}
                </Markdown>
              </div>
            </div>
          </div>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {editing ? <><Button type="button" variant="outline" size="sm" disabled={saving} onClick={() => void saveEdit()}><Check className="size-3.5" />Save</Button><Button type="button" variant="ghost" size="sm" onClick={() => { setDraft(message.content); setEditing(false) }}><X className="size-3.5" /></Button></> : <Button type="button" variant="outline" size="sm" onClick={() => setEditing(true)}><Pencil className="size-3.5" />Edit</Button>}
          <Button type="button" variant="outline" size="sm" onClick={onSteer}><Send className="size-3.5" />Steer</Button>
          <Button type="button" variant="ghost" size="icon" aria-label="Delete queued message" onClick={onRemove}><Trash2 className="size-3.5" /></Button>
        </div>
      </div>
    </div>
  )
}
