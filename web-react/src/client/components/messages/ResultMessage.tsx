import { useState } from "react"
import { Loader2, RotateCcw } from "lucide-react"
import type { ProcessedResultMessage } from "./types"
import { MetaRow, MetaLabel } from "./shared"
import { useI18n } from "../../i18n/context"
import { Button } from "../ui/button"

interface Props {
  message: ProcessedResultMessage
  onRetry?: () => Promise<void>
}

export function ResultMessage({ message, onRetry }: Props) {
  const { t, locale } = useI18n()
  const [retrying, setRetrying] = useState(false)
  const [retryError, setRetryError] = useState<string | null>(null)
  const formatDuration = (ms: number) => {
    if (ms < 1000) {
      return `${ms}ms`
    }

    const totalSeconds = Math.floor(ms / 1000)
    const hours = Math.floor(totalSeconds / 3600)
    const minutes = Math.floor((totalSeconds % 3600) / 60)
    const seconds = totalSeconds % 60

    if (hours > 0) {
      return `${hours}h${minutes > 0 ? ` ${minutes}m` : ""}`
    }

    if (minutes > 0) {
      return `${minutes}m${seconds > 0 ? ` ${seconds}s` : ""}`
    }

    return `${seconds}s`
  }

  if (!message.success) {
    const retry = async () => {
      if (!onRetry || retrying) return
      setRetrying(true)
      setRetryError(null)
      try {
        await onRetry()
      } catch (error) {
        setRetryError(error instanceof Error ? error.message : String(error))
      } finally {
        setRetrying(false)
      }
    }
    return (
      <div role="alert" className="mx-2 my-1 rounded-lg border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive">
        <div className="whitespace-pre-wrap">
          {message.result || t.messages.unknownError}
        </div>
        {onRetry && !message.cancelled ? (
          <div className="mt-3 flex flex-wrap items-center gap-2" dir={locale === "fa" ? "rtl" : "ltr"}>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void retry()}
              disabled={retrying}
              aria-label={t.common.retry}
            >
              {retrying ? <Loader2 className="size-3.5 animate-spin" /> : <RotateCcw className="size-3.5" />}
              {t.common.retry}
            </Button>
            <span className="text-xs text-destructive/80">
              {locale === "fa"
                ? "یک پیام «ادامه بده» در همین نشست فرستاده می‌شود."
                : "Sends “Continue” in this same session."}
            </span>
          </div>
        ) : null}
        {retryError ? (
          <p className="mt-2 text-xs" aria-live="polite">
            {retryError}
          </p>
        ) : null}
      </div>
    )
  }

  return (
    <MetaRow className={`px-0.5 text-xs tracking-wide ${message.durationMs > 60000 ? '' : 'hidden'}`}>
      <div className="w-full h-[1px] bg-border"></div>
      <MetaLabel className="whitespace-nowrap text-[11px] tracking-widest text-muted-foreground/60 uppercase flex-shrink-0">{t.messages.workedFor(formatDuration(message.durationMs))}</MetaLabel>
      <div className="w-full h-[1px] bg-border"></div>
    </MetaRow>
  )
}
