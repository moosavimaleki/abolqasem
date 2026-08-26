import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nProvider } from "../../i18n/context"
import { getQuotaWindowComparison, SessionHealthPanel, SessionHealthPopover } from "./SessionHealthPopover"

describe("SessionHealthPopover", () => {
  test("compares quota remaining with time remaining in the same seven-day window", () => {
    const now = Date.UTC(2026, 7, 26, 0, 0, 0)
    const comparison = getQuotaWindowComparison({
      usedPercent: 68,
      windowDurationMins: 10_080,
      resetsAt: (now + 5 * 24 * 60 * 60 * 1000) / 1000,
    }, now)

    expect(comparison.quotaRemaining).toBe(32)
    expect(Math.round(comparison.timeRemaining ?? 0)).toBe(71)
  })

  test("renders quota, context, active account, and copyable Codex session details", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="fa">
        <SessionHealthPanel
          snapshot={{
            planType: "plus",
            secondary: {
              usedPercent: 68,
              windowDurationMins: 10_080,
              resetsAt: Math.floor(Date.now() / 1000) + 5 * 24 * 60 * 60,
            },
          }}
          contextUsage={{
            usedTokens: 96_000,
            maxTokens: 128_000,
            remainingTokens: 32_000,
            usedPercentage: 75,
            remainingPercentage: 25,
            updatedAt: new Date().toISOString(),
            compactsAutomatically: false,
          }}
          accountEmail="active@example.com"
          sessionId="01a-session-id"
          sessionPath="/home/test/.codex/sessions/rollout-01a-session-id.jsonl"
        />
      </I18nProvider>,
    )

    expect(html).toContain("سهمیهٔ باقی‌مانده")
    expect(html).toContain("زمان تا بازنشانی")
    expect(html).toContain("کانتکست این چت")
    expect(html).toContain("96k")
    expect(html).toContain("active@example.com")
    expect(html).toContain("codex resume 01a-session-id")
    expect(html).toContain("/home/test/.codex/sessions/rollout-01a-session-id.jsonl")
    expect(html).toContain('data-session-health-comparison="quota-time"')
  })

  test("uses one restrained dual-ring trigger instead of separate quota and context buttons", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <SessionHealthPopover
          snapshot={{ secondary: { usedPercent: 20, windowDurationMins: 10_080, resetsAt: Math.floor(Date.now() / 1000) + 86_400 } }}
          contextUsage={{ usedTokens: 10, remainingTokens: 90, maxTokens: 100, usedPercentage: 10, remainingPercentage: 90, updatedAt: "now", compactsAutomatically: false }}
        />
      </I18nProvider>,
    )

    expect(html).toContain('data-session-health-chart="dual-radial"')
    expect(html.match(/<button/g)).toHaveLength(1)
  })
})
