import { AlertTriangle, LockKeyhole, RefreshCw, UnlockKeyhole } from "lucide-react"
import type { CodexLockStatus } from "../../../shared/types"
import { Button } from "../../components/ui/button"

interface CodexSessionLockNoticeProps {
  lock: CodexLockStatus
  busy?: boolean
  onRefresh: () => void
  onTakeOver: () => void
  onRelease: () => void
}

export function CodexSessionLockNotice({ lock, busy = false, onRefresh, onTakeOver, onRelease }: CodexSessionLockNoticeProps) {
  if (lock.state === "available") return null

  if (lock.state === "owned_by_us") {
    return (
      <section className="mx-3 mb-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm" aria-live="polite">
        <div className="flex min-w-0 items-start gap-3">
          <UnlockKeyhole className="mt-0.5 h-5 w-5 shrink-0 text-emerald-500" aria-hidden="true" />
          <div className="min-w-0">
            <p className="font-medium text-foreground">این نشست در اختیار Abolqasem است</p>
            <p className="mt-1 text-muted-foreground">می‌توانید گفتگو را ادامه دهید، یا پس از پایان turn آن را برای Codex دیگری آزاد کنید.</p>
          </div>
        </div>
        <Button type="button" variant="outline" size="sm" disabled={busy || !lock.canRelease} onClick={onRelease} className="min-h-10 shrink-0">
          <UnlockKeyhole className="me-2 h-4 w-4" aria-hidden="true" />
          آزاد کردن نشست
        </Button>
      </section>
    )
  }

  const isUnknown = lock.state === "unknown"
  return (
    <section className="mx-3 mb-3 rounded-xl border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm" role="status" aria-live="polite">
      <div className="flex items-start gap-3">
        <LockKeyhole className="mt-0.5 h-5 w-5 shrink-0 text-amber-500" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <p className="font-medium text-foreground">{isUnknown ? "وضعیت مالک نشست نامشخص است" : "این نشست در Codex دیگری باز است"}</p>
          <p className="mt-1 text-muted-foreground">{lock.message ?? "تاریخچه فقط‌خواندنی است؛ برای جلوگیری از نوشتن هم‌زمان، کادر ارسال پنهان شده است."}</p>
          {lock.ownerPid ? (
            <p className="mt-2 break-all font-mono text-xs text-muted-foreground" dir="ltr">
              PID {lock.ownerPid}{lock.ownerCommand ? ` · ${lock.ownerCommand}` : ""}
            </p>
          ) : null}
          {lock.otherWritableSessions ? (
            <p className="mt-2 flex items-start gap-2 text-amber-700 dark:text-amber-300">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
              همین process روی {lock.otherWritableSessions} نشست دیگر هم writer دارد؛ takeover ممکن است آن‌ها را هم قطع کند.
            </p>
          ) : null}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={onRefresh} className="min-h-10">
          <RefreshCw className="me-2 h-4 w-4" aria-hidden="true" />
          بررسی دوباره
        </Button>
        {lock.canTakeOver ? (
          <Button type="button" variant="destructive" size="sm" disabled={busy} onClick={onTakeOver} className="min-h-10">
            <LockKeyhole className="me-2 h-4 w-4" aria-hidden="true" />
            گرفتن نشست
          </Button>
        ) : null}
      </div>
    </section>
  )
}
