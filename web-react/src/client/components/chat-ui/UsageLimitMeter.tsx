import { Gauge } from "lucide-react"
import type { RateLimitSnapshot } from "../../../shared/types"
import { formatRateLimitDuration, selectLongRateLimitWindow } from "../../lib/usage"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"

export function UsageLimitMeter({ snapshot }: { snapshot: RateLimitSnapshot }) {
  const window = selectLongRateLimitWindow(snapshot)
  if (!window) return null
  const used = Math.max(0, Math.min(100, window.usedPercent))
  const reset = window.resetsAt ? new Date(window.resetsAt * 1000) : null
  return (
    <Tooltip delayDuration={0}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="inline-flex h-6 items-center gap-1 rounded-full px-1.5 text-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          aria-label={`${formatRateLimitDuration(window.windowDurationMins)} usage ${Math.round(used)}%`}
        >
          <Gauge className="size-3" />
          <span>{Math.round(used)}%</span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" className="space-y-1 px-3 py-2 text-xs">
        <div>{formatRateLimitDuration(window.windowDurationMins)} usage · {Math.round(used)}%</div>
        {snapshot.planType ? <div className="text-muted-foreground">{snapshot.planType}</div> : null}
        {reset ? <div className="text-muted-foreground">Resets {reset.toLocaleString()}</div> : null}
      </TooltipContent>
    </Tooltip>
  )
}
