import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nProvider } from "../../i18n/context"
import { TooltipProvider } from "../ui/tooltip"
import { ContextWindowMeter } from "./ContextWindowMeter"

describe("ContextWindowMeter", () => {
  test("uses remaining context language and a compact radial chart in Persian", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="fa"><TooltipProvider><ContextWindowMeter usage={{
        usedTokens: 102_400,
        maxTokens: 128_000,
        remainingTokens: 25_600,
        usedPercentage: 80,
        remainingPercentage: 20,
        updatedAt: new Date().toISOString(),
        compactsAutomatically: false,
      }} /></TooltipProvider></I18nProvider>,
    )

    expect(html).toContain('data-context-window-chart="radial"')
    expect(html).toContain("باقی‌ماندهٔ کانتکست")
    expect(html).toContain("۲۰٪")
    expect(html).not.toContain("مصرف شده")
  })
})
