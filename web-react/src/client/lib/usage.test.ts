import { describe, expect, test } from "bun:test"
import { deriveLatestRateLimitSnapshot, formatBytes, selectLongRateLimitWindow } from "./usage"

describe("usage", () => {
  test("derives the latest app-server rate limit and selects the longest window", () => {
    const latest = deriveLatestRateLimitSnapshot([
      { _id: "1", kind: "rate_limit_updated", timestamp: "2026-01-01", rateLimits: { primary: { usedPercent: 5, windowDurationMins: 300 } } },
      { _id: "2", kind: "rate_limit_updated", timestamp: "2026-01-02", rateLimits: { primary: { usedPercent: 20, windowDurationMins: 300 }, secondary: { usedPercent: 51, windowDurationMins: 10_080 } } },
    ])
    expect(latest?.secondary?.usedPercent).toBe(51)
    expect(selectLongRateLimitWindow(latest)?.windowDurationMins).toBe(10_080)
  })

  test("formats disk usage", () => {
    expect(formatBytes(0)).toBe("0 B")
    expect(formatBytes(1_048_576)).toBe("1.0 MB")
  })
})
