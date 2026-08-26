import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nProvider } from "../../i18n/context"
import { TooltipProvider } from "../ui/tooltip"
import { UsageLimitMeter } from "./UsageLimitMeter"

function renderMeter(locale: "en" | "fa", snapshot: Parameters<typeof UsageLimitMeter>[0]["snapshot"]) {
  return renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <TooltipProvider>
        <UsageLimitMeter snapshot={snapshot} />
      </TooltipProvider>
    </I18nProvider>,
  )
}

describe("UsageLimitMeter", () => {
  test("renders weekly usage as a compact accessible radial chart", () => {
    const html = renderMeter("fa", {
      planType: "plus",
      secondary: {
        usedPercent: 89,
        windowDurationMins: 10_080,
        resetsAt: Math.floor(Date.now() / 1000) + 24 * 60 * 60,
      },
    })

    expect(html).toContain('data-usage-limit-chart="radial"')
    expect(html.match(/<circle/g)).toHaveLength(2)
    expect(html).toContain("stroke-dasharray")
    expect(html).toContain("89")
    expect(html).toContain("باقی‌ماندهٔ سهمیه")
    expect(html).toContain("۱۱٪")
    expect(html).toContain('aria-label="باقی‌ماندهٔ سهمیهٔ ۱ هفته: ۱۱٪"')
  })

  test("uses a restrained danger tone only near exhaustion", () => {
    const html = renderMeter("en", { primary: { usedPercent: 97, windowDurationMins: 300 } })

    expect(html).toContain("text-destructive/80")
    expect(html).toContain("97")
  })
})
