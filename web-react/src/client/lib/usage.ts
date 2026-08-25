import type { RateLimitSnapshot, RateLimitWindowSnapshot, TranscriptEntry } from "../../shared/types"

export interface UsageSnapshot {
  codex: { rate_limits: RateLimitSnapshot; updated_at: string } | null
}

export interface ResourceUsageSnapshot {
  storage: {
    total_bytes: number
    cache_bytes: number
    upload_bytes: number
    workspace_bytes: number
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
  if (!snapshot) return null
  const windows = [snapshot.primary, snapshot.secondary].filter((window): window is RateLimitWindowSnapshot => Boolean(window))
  return windows.sort((left, right) => (right.windowDurationMins ?? 0) - (left.windowDurationMins ?? 0))[0] ?? null
}

export function formatRateLimitDuration(minutes?: number | null) {
  if (!minutes) return "Limit"
  if (minutes % 10_080 === 0) return `${minutes / 10_080}w`
  if (minutes % 1_440 === 0) return `${minutes / 1_440}d`
  if (minutes % 60 === 0) return `${minutes / 60}h`
  return `${minutes}m`
}

export function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / 1024 ** index
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}
