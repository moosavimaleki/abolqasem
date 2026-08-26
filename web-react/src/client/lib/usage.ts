import type { RateLimitSnapshot, RateLimitWindowSnapshot, TranscriptEntry } from "../../shared/types"

export interface UsageSnapshot {
  codex: {
    rate_limits: RateLimitSnapshot
    account?: { type?: string; email?: string; plan_type?: string }
    updated_at: string
  } | null
}

export interface ResourceUsageSnapshot {
  storage: {
    total_bytes: number
    cache_bytes: number
    upload_bytes: number
    workspace_bytes: number
    checkpoint_bytes?: number
    archive_bytes?: number
    archive_count?: number
  }
}

export function deriveLatestRateLimitSnapshot(entries: TranscriptEntry[]): RateLimitSnapshot | null {
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index]
    if (entry?.kind === "rate_limit_updated") return entry.rateLimits
  }
  return null
}

export function selectLongRateLimitWindow(snapshot: RateLimitSnapshot | null): RateLimitWindowSnapshot | null {
  return selectRateLimitWindows(snapshot)[0] ?? null
}

/**
 * The Codex app-server can expose a short rolling limit alongside a weekly
 * limit, and may add/remove either at runtime. Keep the UI data-driven rather
 * than assuming that primary or secondary has a fixed duration.
 */
export function selectRateLimitWindows(snapshot: RateLimitSnapshot | null): RateLimitWindowSnapshot[] {
  if (!snapshot) return []
  const seen = new Set<string>()
  return [...(snapshot.windows ?? []), snapshot.primary, snapshot.secondary]
    .filter((window): window is RateLimitWindowSnapshot => Boolean(window))
    .filter((window) => {
      const key = `${window.windowDurationMins ?? ""}:${window.resetsAt ?? ""}:${window.usedPercent}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
    .sort((left, right) => (right.windowDurationMins ?? 0) - (left.windowDurationMins ?? 0))
}

export function formatRateLimitDuration(minutes?: number | null) {
  if (!minutes) return "Limit"
  if (minutes % 10_080 === 0) return `${minutes / 10_080}w`
  if (minutes % 1_440 === 0) return `${minutes / 1_440}d`
  if (minutes % 60 === 0) return `${minutes / 60}h`
  return `${minutes}m`
}

export function formatRateLimitDurationLocalized(minutes: number | null | undefined, locale: "en" | "fa"): string {
  const total = Math.max(0, Math.round(minutes ?? 0))
  const value = (number: number) => new Intl.NumberFormat(locale === "fa" ? "fa-IR" : "en-US").format(number)
  const unit = (number: number, en: string, fa: string) => locale === "fa"
    ? `${value(number)} ${fa}`
    : `${value(number)} ${en}${number === 1 ? "" : "s"}`
  if (total === 0) return locale === "fa" ? "بازهٔ سهمیه" : "Quota window"
  if (total % 10_080 === 0) return unit(total / 10_080, "week", "هفته")
  if (total % 1_440 === 0) return unit(total / 1_440, "day", "روز")
  if (total % 60 === 0) return unit(total / 60, "hour", "ساعت")
  return unit(total, "minute", "دقیقه")
}

/** A compact, locale-aware duration for quota reset affordances. */
export function formatRelativeResetTime(resetsAtSeconds: number | null | undefined, locale: "en" | "fa", nowMs = Date.now()): string | null {
  if (!Number.isFinite(resetsAtSeconds)) return null
  let remainingMinutes = Math.max(0, Math.ceil(((resetsAtSeconds as number) * 1000 - nowMs) / 60_000))
  if (remainingMinutes === 0) return locale === "fa" ? "اکنون" : "now"

  const value = (number: number) => new Intl.NumberFormat(locale === "fa" ? "fa-IR" : "en-US").format(number)
  const unit = (number: number, en: string, fa: string) => locale === "fa"
    ? `${value(number)} ${fa}`
    : `${value(number)} ${en}${number === 1 ? "" : "s"}`
  const parts: string[] = []
  const days = Math.floor(remainingMinutes / 1_440)
  if (days > 0) {
    parts.push(unit(days, "day", "روز"))
    remainingMinutes -= days * 1_440
  }
  const hours = Math.floor(remainingMinutes / 60)
  if (hours > 0 && parts.length < 2) {
    parts.push(unit(hours, "hour", "ساعت"))
    remainingMinutes -= hours * 60
  }
  if (parts.length < 2 && remainingMinutes > 0) parts.push(unit(remainingMinutes, "minute", "دقیقه"))
  return parts.join(locale === "fa" ? " و " : " ")
}

export function formatLocalizedPercent(value: number, locale: "en" | "fa"): string {
  const percent = Math.max(0, Math.min(100, Math.round(value)))
  return `${new Intl.NumberFormat(locale === "fa" ? "fa-IR" : "en-US").format(percent)}${locale === "fa" ? "٪" : "%"}`
}

export function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / 1024 ** index
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}
