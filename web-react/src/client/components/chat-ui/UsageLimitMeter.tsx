import type { RateLimitSnapshot } from "../../../shared/types"
import { formatRateLimitDuration, selectLongRateLimitWindow } from "../../lib/usage"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { useI18n } from "../../i18n/context"

export function UsageLimitMeter({ snapshot }: { snapshot: RateLimitSnapshot }) {
  const { locale } = useI18n()
  const fa = locale === "fa"
  const window = selectLongRateLimitWindow(snapshot)
  if (!window) return null
  const used = Math.max(0, Math.min(100, window.usedPercent))
  const roundedUsed = Math.round(used)
  const reset = window.resetsAt ? new Date(window.resetsAt * 1000) : null
  const radius = 10
  const circumference = 2 * Math.PI * radius
  const dashOffset = circumference - (used / 100) * circumference
  const progressTone = used >= 95 ? "text-destructive/80" : "text-muted-foreground"
  return (
    <Tooltip delayDuration={0}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="group inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-full text-muted-foreground transition-colors duration-200 hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          aria-label={fa ? `مصرف محدودیت ${formatRateLimitDuration(window.windowDurationMins)}، ${roundedUsed} درصد` : `${formatRateLimitDuration(window.windowDurationMins)} usage ${roundedUsed}%`}
        >
          <span className="relative flex h-6 w-6 items-center justify-center" data-usage-limit-chart="radial">
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
              {roundedUsed}
            </span>
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" className="space-y-1 px-3 py-2 text-xs">
        <div>{fa ? `مصرف ${formatRateLimitDuration(window.windowDurationMins)}` : `${formatRateLimitDuration(window.windowDurationMins)} usage`} · {roundedUsed}%</div>
        {snapshot.planType ? <div className="text-muted-foreground">{snapshot.planType}</div> : null}
        {reset ? <div className="text-muted-foreground">{fa ? "بازنشانی" : "Resets"} {reset.toLocaleString(fa ? "fa-IR" : undefined)}</div> : null}
      </TooltipContent>
    </Tooltip>
  )
}
