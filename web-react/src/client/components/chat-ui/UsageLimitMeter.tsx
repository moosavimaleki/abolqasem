import type { RateLimitSnapshot } from "../../../shared/types"
import { formatLocalizedPercent, formatRateLimitDurationLocalized, formatRelativeResetTime, selectLongRateLimitWindow } from "../../lib/usage"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { useI18n } from "../../i18n/context"

export function UsageLimitMeter({ snapshot }: { snapshot: RateLimitSnapshot }) {
  const { locale } = useI18n()
  const fa = locale === "fa"
  const window = selectLongRateLimitWindow(snapshot)
  if (!window) return null
  const used = Math.max(0, Math.min(100, window.usedPercent))
  const remaining = 100 - used
  const resetAfter = formatRelativeResetTime(window.resetsAt, locale)
  const windowLabel = formatRateLimitDurationLocalized(window.windowDurationMins, locale)
  const radius = 10
  const circumference = 2 * Math.PI * radius
  const dashOffset = circumference - (remaining / 100) * circumference
  const progressTone = remaining <= 5 ? "text-destructive/80" : "text-muted-foreground"
  return (
    <Tooltip delayDuration={0}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="group inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-full text-muted-foreground transition-colors duration-200 hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          aria-label={fa ? `باقی‌ماندهٔ سهمیهٔ ${windowLabel}: ${formatLocalizedPercent(remaining, locale)}` : `${formatLocalizedPercent(remaining, locale)} quota remaining for ${windowLabel}`}
        >
            <span className="relative flex h-7 w-7 items-center justify-center" data-usage-limit-chart="radial">
            <svg
              viewBox="0 0 24 24"
              className="absolute inset-0 h-full w-full -rotate-90 transform-gpu"
              aria-hidden="true"
            >
              <circle
                cx="12"
                cy="12"
                r={radius}
                fill="none"
                stroke="currentColor"
                strokeWidth="1.75"
                className="text-muted-foreground/20"
              />
              <circle
                cx="12"
                cy="12"
                r={radius}
                fill="none"
                stroke="currentColor"
                strokeWidth="1.75"
                strokeLinecap="round"
                strokeDasharray={circumference}
                strokeDashoffset={dashOffset}
                className={`${progressTone} transition-[stroke-dashoffset] duration-500 ease-out motion-reduce:transition-none`}
              />
            </svg>
            <span className="relative text-[8px] font-medium tabular-nums leading-none text-muted-foreground group-hover:text-foreground">
              {Math.round(remaining)}
            </span>
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" dir={fa ? "rtl" : "ltr"} className="w-52 space-y-2 px-3 py-2.5 text-xs shadow-lg">
        <div className="flex items-baseline justify-between gap-3">
          <span className="font-medium text-foreground">{fa ? "باقی‌ماندهٔ سهمیه" : "Quota remaining"}</span>
          <span className="tabular-nums text-foreground">{formatLocalizedPercent(remaining, locale)}</span>
        </div>
        <div className="text-muted-foreground">{windowLabel}{snapshot.planType ? ` · ${snapshot.planType}` : ""}</div>
        {resetAfter ? <div className="border-t border-border/70 pt-2 text-muted-foreground">{fa ? `بازنشانی پس از ${resetAfter}` : `Resets in ${resetAfter}`}</div> : null}
      </TooltipContent>
    </Tooltip>
  )
}
