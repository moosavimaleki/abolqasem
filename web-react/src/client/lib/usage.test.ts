import { describe, expect, test } from "bun:test"
import { deriveLatestRateLimitSnapshot, formatBytes, formatLocalizedPercent, formatRateLimitDurationLocalized, formatRelativeResetTime, selectLongRateLimitWindow, selectRateLimitWindows } from "./usage"

describe("usage", () => {
  test("derives the latest app-server rate limit and selects the longest window", () => {
    const latest = deriveLatestRateLimitSnapshot([
      { _id: "1", kind: "rate_limit_updated", timestamp: "2026-01-01", rateLimits: { primary: { usedPercent: 5, windowDurationMins: 300 } } },
      { _id: "2", kind: "rate_limit_updated", timestamp: "2026-01-02", rateLimits: { primary: { usedPercent: 20, windowDurationMins: 300 }, secondary: { usedPercent: 51, windowDurationMins: 10_080 } } },
    ])
    expect(latest?.secondary?.usedPercent).toBe(51)
    expect(selectLongRateLimitWindow(latest)?.windowDurationMins).toBe(10_080)
    expect(selectRateLimitWindows(latest).map((window) => window.windowDurationMins)).toEqual([10_080, 300])
  })

  test("keeps every rate-limit window that a newer app-server exposes", () => {
    expect(selectRateLimitWindows({
      primary: { usedPercent: 20, windowDurationMins: 300 },
      windows: [
        { usedPercent: 20, windowDurationMins: 300 },
        { usedPercent: 42, windowDurationMins: 1_440 },
        { usedPercent: 61, windowDurationMins: 10_080 },
      ],
    }).map((window) => window.windowDurationMins)).toEqual([10_080, 1_440, 300])
  })

  test("formats disk usage", () => {
    expect(formatBytes(0)).toBe("0 B")
    expect(formatBytes(1_048_576)).toBe("1.0 MB")
  })

  test("formats quota labels and reset times for Persian without a raw timestamp", () => {
    expect(formatRateLimitDurationLocalized(10_080, "fa")).toBe("۱ هفته")
    expect(formatLocalizedPercent(11, "fa")).toBe("۱۱٪")
    expect(formatRelativeResetTime(1_800_000_000, "fa", 1_799_880_000_000)).toBe("۱ روز و ۹ ساعت")
  })

  test("formats a compact English relative reset time", () => {
    expect(formatRelativeResetTime(1_800_000_000, "en", 1_799_997_000_000)).toBe("50 minutes")
  })
})
