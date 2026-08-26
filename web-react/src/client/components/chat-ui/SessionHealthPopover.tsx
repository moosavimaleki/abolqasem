import { useMemo } from "react"
import { Mail } from "lucide-react"
import type { RateLimitSnapshot, RateLimitWindowSnapshot } from "../../../shared/types"
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover"
import { useI18n } from "../../i18n/context"
import { type ContextWindowSnapshot, formatContextWindowTokens } from "../../lib/contextWindow"
import {
  formatLocalizedPercent,
  formatRateLimitDurationLocalized,
  formatRelativeResetTime,
  selectLongRateLimitWindow,
  selectRateLimitWindows,
} from "../../lib/usage"

interface SessionHealthPopoverProps {
  snapshot?: RateLimitSnapshot | null
  contextUsage?: ContextWindowSnapshot | null
  accountEmail?: string | null
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, value))
}

export function getQuotaWindowComparison(window: RateLimitWindowSnapshot, nowMs = Date.now()) {
  const quotaRemaining = clampPercent(100 - window.usedPercent)
  const durationMs = (window.windowDurationMins ?? 0) * 60_000
  const resetMs = (window.resetsAt ?? 0) * 1000
  const timeRemaining = durationMs > 0 && resetMs > 0
    ? clampPercent(((resetMs - nowMs) / durationMs) * 100)
    : null
  return { quotaRemaining, timeRemaining }
}

function ProgressRow({
  label,
  value,
  detail,
  marker,
}: {
  label: string
  value: number
  detail: string
  marker?: "quota" | "time" | "context"
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-3 text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium tabular-nums text-foreground">{detail}</span>
      </div>
      <div
        className="h-1.5 overflow-hidden rounded-full bg-muted"
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(value)}
      >
        <div
          className={marker === "quota" && value <= 10
            ? "h-full rounded-full bg-destructive/70 transition-transform duration-300 motion-reduce:transition-none"
            : marker === "time"
              ? "h-full rounded-full bg-muted-foreground/55 transition-transform duration-300 motion-reduce:transition-none"
              : "h-full rounded-full bg-foreground/65 transition-transform duration-300 motion-reduce:transition-none"}
          style={{ transform: `translateX(${value - 100}%)` }}
        />
      </div>
    </div>
  )
}

export function SessionHealthPanel({
  snapshot,
  contextUsage,
  accountEmail,
}: SessionHealthPopoverProps) {
  const { locale } = useI18n()
  const fa = locale === "fa"
  const quotaWindows = selectRateLimitWindows(snapshot ?? null)
  const contextRemaining = contextUsage?.remainingPercentage == null
    ? null
    : clampPercent(contextUsage.remainingPercentage)
  return (
    <div className="space-y-3" dir={fa ? "rtl" : "ltr"}>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-sm font-medium text-foreground">{fa ? "وضعیت نشست" : "Session status"}</div>
          {accountEmail ? (
            <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground">
              <Mail className="h-3 w-3 shrink-0" aria-hidden="true" />
              <span dir="ltr" className="truncate">{accountEmail}</span>
            </div>
          ) : null}
        </div>
        {snapshot?.planType ? (
          <span className="shrink-0 rounded-full border border-border/70 px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
            {snapshot.planType}
          </span>
        ) : null}
      </div>

      {quotaWindows.length > 0 ? (
        <section className="space-y-2 rounded-xl border border-border/65 bg-muted/20 p-2.5" aria-label={fa ? "بازه‌های سهمیه" : "Quota windows"}>
          {quotaWindows.map((quotaWindow, index) => {
            const comparison = getQuotaWindowComparison(quotaWindow)
            const resetAfter = formatRelativeResetTime(quotaWindow.resetsAt, locale)
            const duration = formatRateLimitDurationLocalized(quotaWindow.windowDurationMins, locale)
            return (
              <div
                key={`${quotaWindow.windowDurationMins ?? "unknown"}-${quotaWindow.resetsAt ?? index}`}
                className={index > 0 ? "space-y-2 border-t border-border/55 pt-2.5" : "space-y-2"}
                data-session-health-comparison="quota-time"
              >
                <div className="flex items-center justify-between gap-3 text-[11px]">
                  <span className="font-medium text-foreground">{duration}</span>
                  {resetAfter ? <span className="tabular-nums text-muted-foreground">{resetAfter}</span> : null}
                </div>
                <ProgressRow
                  marker="quota"
                  label={fa ? "سهمیهٔ باقی‌مانده" : "Quota remaining"}
                  value={comparison.quotaRemaining}
                  detail={formatLocalizedPercent(comparison.quotaRemaining, locale)}
                />
              </div>
            )
          })}
        </section>
      ) : null}

      {contextUsage ? (
        <div className="space-y-1.5 border-t border-border/60 pt-3">
          {contextRemaining !== null && contextUsage.maxTokens !== undefined ? (
            <ProgressRow
              marker="context"
              label={fa ? "کانتکست این چت" : "Chat context"}
              value={contextRemaining}
              detail={fa
                ? `${formatLocalizedPercent(contextRemaining, locale)} باقی`
                : `${formatLocalizedPercent(contextRemaining, locale)} left`}
            />
          ) : (
            <div className="flex items-baseline justify-between gap-3 text-xs">
              <span className="text-muted-foreground">{fa ? "اندازهٔ کانتکست" : "Context size"}</span>
              <span className="tabular-nums text-foreground">{formatContextWindowTokens(contextUsage.usedTokens)}</span>
            </div>
          )}
          {contextUsage.maxTokens !== undefined ? (
            <div className="text-[10px] text-muted-foreground">
              {fa
                ? `${formatContextWindowTokens(contextUsage.usedTokens)} از ${formatContextWindowTokens(contextUsage.maxTokens)} توکن استفاده شده`
                : `${formatContextWindowTokens(contextUsage.usedTokens)} of ${formatContextWindowTokens(contextUsage.maxTokens)} tokens used`}
            </div>
          ) : null}
        </div>
      ) : null}

    </div>
  )
}

export function SessionHealthPopover(props: SessionHealthPopoverProps) {
  const { locale } = useI18n()
  const fa = locale === "fa"
  const quotaWindow = selectLongRateLimitWindow(props.snapshot ?? null)
  const comparison = useMemo(() => quotaWindow ? getQuotaWindowComparison(quotaWindow) : null, [quotaWindow])
  const fallback = props.contextUsage?.remainingPercentage ?? 0
  const quota = comparison?.quotaRemaining ?? clampPercent(fallback)
  const time = comparison?.timeRemaining
  const outerRadius = 10
  const innerRadius = 6.5
  const outerCircumference = 2 * Math.PI * outerRadius
  const innerCircumference = 2 * Math.PI * innerRadius
  const label = comparison && typeof time === "number"
    ? (fa
      ? `سهمیهٔ باقی‌مانده ${formatLocalizedPercent(quota, locale)}، زمان باقی‌مانده ${formatLocalizedPercent(time, locale)}`
      : `${formatLocalizedPercent(quota, locale)} quota and ${formatLocalizedPercent(time, locale)} time remaining`)
    : (fa ? "وضعیت نشست" : "Session status")

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="group relative inline-flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-full text-muted-foreground transition-colors duration-200 after:absolute after:-inset-1 hover:bg-muted/60 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          aria-label={label}
          title={label}
        >
          <span className="relative flex h-7 w-7 items-center justify-center" data-session-health-chart="dual-radial">
            <svg viewBox="0 0 24 24" className="absolute inset-0 h-full w-full -rotate-90 transform-gpu" aria-hidden="true">
              <circle cx="12" cy="12" r={outerRadius} fill="none" stroke="currentColor" strokeWidth="1.5" className="text-muted-foreground/20" />
              <circle cx="12" cy="12" r={outerRadius} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeDasharray={outerCircumference} strokeDashoffset={outerCircumference - quota / 100 * outerCircumference} className={quota <= 10 ? "text-destructive/75" : "text-foreground/65"} />
              {time !== null && time !== undefined ? (
                <>
                  <circle cx="12" cy="12" r={innerRadius} fill="none" stroke="currentColor" strokeWidth="1.5" className="text-muted-foreground/15" />
                  <circle cx="12" cy="12" r={innerRadius} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeDasharray={innerCircumference} strokeDashoffset={innerCircumference - time / 100 * innerCircumference} className="text-muted-foreground/65" />
                </>
              ) : null}
            </svg>
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent side="top" align="center" sideOffset={8} className="w-[min(calc(100vw-1.5rem),320px)] p-3 shadow-xl">
        <SessionHealthPanel {...props} />
      </PopoverContent>
    </Popover>
  )
}
