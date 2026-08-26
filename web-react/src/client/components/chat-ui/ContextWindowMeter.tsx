import { cn } from "../../lib/utils"
import { type ContextWindowSnapshot, formatContextWindowTokens } from "../../lib/contextWindow"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { useI18n } from "../../i18n/context"
import { formatLocalizedPercent } from "../../lib/usage"

function formatPercentage(value: number | null): string | null {
  if (value === null || !Number.isFinite(value)) {
    return null
  }
  if (value < 10) {
    return `${value.toFixed(1).replace(/\.0$/, "")}%`
  }
  return `${Math.round(value)}%`
}

export function ContextWindowMeter({ usage }: { usage: ContextWindowSnapshot }) {
	const { locale } = useI18n()
	const fa = locale === "fa"
  const remainingPercentage = formatPercentage(usage.remainingPercentage)
  const normalizedPercentage = Math.max(0, Math.min(100, usage.remainingPercentage ?? 0))
  const radius = 9.75
  const circumference = 2 * Math.PI * radius
  const dashOffset = circumference - (normalizedPercentage / 100) * circumference

  return (
    <Tooltip delayDuration={0}>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="group inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-full text-muted-foreground transition-colors duration-200 hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          aria-label={
            usage.maxTokens !== undefined && remainingPercentage
              ? (fa ? `باقی‌ماندهٔ کانتکست: ${formatLocalizedPercent(usage.remainingPercentage ?? 0, locale)}` : `${formatLocalizedPercent(usage.remainingPercentage ?? 0, locale)} context remaining`)
              : (fa ? `اندازهٔ کانتکست جاری: ${formatContextWindowTokens(usage.usedTokens)} توکن` : `Current context size: ${formatContextWindowTokens(usage.usedTokens)} tokens`)
          }
        >
          <span className="relative flex h-7 w-7 items-center justify-center" data-context-window-chart="radial">
            <svg
              viewBox="0 0 24 24"
              className="-rotate-90 absolute inset-0 h-full w-full transform-gpu"
              aria-hidden="true"
            >
              <circle
                cx="12"
                cy="12"
                r={radius}
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                className="text-muted-foreground/20"
              />
              <circle
                cx="12"
                cy="12"
                r={radius}
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeDasharray={circumference}
                strokeDashoffset={dashOffset}
                className="text-muted-foreground transition-[stroke-dashoffset] duration-500 ease-out"
              />
            </svg>
            <span
              className={cn(
                "relative flex h-[17px] w-[17px] items-center justify-center rounded-full bg-background text-[8px] font-medium tabular-nums",
                "text-muted-foreground",
              )}
            >
              {usage.remainingPercentage !== null
                ? Math.round(usage.remainingPercentage)
                : formatContextWindowTokens(usage.usedTokens)}
            </span>
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" align="center" dir={fa ? "rtl" : "ltr"} className="w-52 space-y-2 px-3 py-2.5 shadow-lg">
        <div className="space-y-1.5 leading-tight">
          {usage.maxTokens !== undefined && remainingPercentage ? (
            <>
              <div className="flex items-baseline justify-between gap-3 text-xs">
                <span className="font-medium text-foreground">{fa ? "باقی‌ماندهٔ کانتکست" : "Context remaining"}</span>
                <span className="tabular-nums text-foreground">{formatLocalizedPercent(usage.remainingPercentage ?? 0, locale)}</span>
              </div>
              <div className="text-muted-foreground">{fa ? `${formatContextWindowTokens(usage.remainingTokens)} توکن از ${formatContextWindowTokens(usage.maxTokens)}` : `${formatContextWindowTokens(usage.remainingTokens)} of ${formatContextWindowTokens(usage.maxTokens)} tokens`}</div>
            </>
          ) : (
            <div className="text-xs text-foreground">
              {fa ? `اندازهٔ کانتکست جاری: ${formatContextWindowTokens(usage.usedTokens)} توکن` : `Current context size: ${formatContextWindowTokens(usage.usedTokens)} tokens`}
            </div>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}
