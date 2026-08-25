import { useState } from "react"
import { Check, LockKeyhole, ShieldCheck, X } from "lucide-react"
import type { ApprovalDecision, HydratedToolCall } from "../../../shared/types"
import { useI18n } from "../../i18n/context"
import { cn } from "../../lib/utils"
import { useTranscriptRenderOptions } from "./render-context"

interface Props {
  message: Extract<HydratedToolCall, { toolKind: "approval_request" }>
  onSubmit: (toolUseId: string, decision: ApprovalDecision) => void | Promise<void>
}

export function ApprovalRequestMessage({ message, onSubmit }: Props) {
  const { t } = useI18n()
  const renderOptions = useTranscriptRenderOptions()
  const [submittedDecision, setSubmittedDecision] = useState<ApprovalDecision | null>(message.result?.decision ?? null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const input = message.input
  const isFileChange = input.approvalKind === "file_change"
  const preview = input.command || input.grantRoot || input.itemId || ""

  const submit = async (decision: ApprovalDecision) => {
    if (renderOptions.readonly || submittedDecision || isSubmitting) return
    setIsSubmitting(true)
    try {
      await onSubmit(message.toolId, decision)
      setSubmittedDecision(decision)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <section className="mx-auto w-full max-w-2xl overflow-hidden rounded-2xl border border-border/80 bg-card/60 shadow-sm" data-approval-request="true">
      <header className="flex items-start gap-3 border-b border-border/70 bg-muted/20 px-4 py-3.5">
        <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full border border-border bg-background text-muted-foreground">
          <LockKeyhole className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold text-foreground">
            {isFileChange ? t.messages.fileChangeApprovalTitle : t.messages.commandApprovalTitle}
          </h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {input.reason || (isFileChange ? t.messages.fileChangeApprovalDescription : t.messages.commandApprovalDescription)}
          </p>
        </div>
      </header>

      {preview ? (
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap border-b border-border/70 bg-background/55 px-4 py-3 font-mono text-xs leading-5 text-foreground" dir="ltr">
          {preview}
        </pre>
      ) : null}
      {input.cwd ? <div className="border-b border-border/70 px-4 py-2 font-mono text-[11px] text-muted-foreground" dir="ltr">cwd: {input.cwd}</div> : null}

      <footer className="flex flex-wrap items-center justify-end gap-2 px-3 py-3">
        {submittedDecision ? (
          <span className={cn("inline-flex items-center gap-1.5 text-xs", submittedDecision === "accept" || submittedDecision === "acceptForSession" ? "text-emerald-500" : "text-muted-foreground")}>
            {submittedDecision === "accept" || submittedDecision === "acceptForSession" ? <Check className="size-3.5" /> : <X className="size-3.5" />}
            {submittedDecision === "acceptForSession" ? t.messages.approvedForSession : submittedDecision === "accept" ? t.messages.approvedOnce : t.messages.declined}
          </span>
        ) : renderOptions.readonly ? (
          <span className="text-xs text-muted-foreground">{t.messages.awaitingResponse}</span>
        ) : (
          <>
            <button type="button" disabled={isSubmitting} onClick={() => void submit("decline")} className="rounded-lg px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50">
              {t.messages.decline}
            </button>
            <button type="button" disabled={isSubmitting} onClick={() => void submit("accept")} className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-background px-3 py-2 text-xs font-medium text-foreground transition-colors hover:bg-muted disabled:opacity-50">
              <ShieldCheck className="size-3.5" />
              {t.messages.approveOnce}
            </button>
            <button type="button" disabled={isSubmitting} onClick={() => void submit("acceptForSession")} className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-2 text-xs font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50">
              <ShieldCheck className="size-3.5" />
              {t.messages.approveForSession}
            </button>
          </>
        )}
      </footer>
    </section>
  )
}
