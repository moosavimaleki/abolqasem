import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { PROVIDERS } from "../../../shared/types"
import { I18nProvider } from "../../i18n/context"
import { ChatPreferenceControls } from "./ChatPreferenceControls"

describe("ChatPreferenceControls", () => {
  test("renders codex-specific controls and can omit plan mode", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <ChatPreferenceControls
          availableProviders={PROVIDERS}
          selectedProvider="codex"
          model="gpt-5.3-codex"
          modelOptions={{ reasoningEffort: "xhigh", fastMode: true, executionMode: "dangerous" }}
          onProviderChange={() => {}}
          onModelChange={() => {}}
          onModelOptionChange={() => {}}
          includePlanMode={false}
        />
      </I18nProvider>
    )

    expect(html).toContain("Codex")
    expect(html).toContain("GPT-5.3 Codex")
    expect(html).toContain("XHigh")
    expect(html).toContain("Fast Mode")
    expect(html).toContain("Unrestricted")
    expect(html).not.toContain("Plan Mode")
  })

  test("renders claude plan mode controls when enabled", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <ChatPreferenceControls
          availableProviders={PROVIDERS}
          selectedProvider="claude"
          model="claude-opus-4-7"
          modelOptions={{ reasoningEffort: "max", contextWindow: "1m" }}
          onProviderChange={() => {}}
          onModelChange={() => {}}
          onModelOptionChange={() => {}}
          planMode
          onPlanModeChange={() => {}}
          includePlanMode
        />
      </I18nProvider>
    )

    expect(html).toContain("Claude")
    expect(html).toContain("Opus 4.7")
    expect(html).toContain("Max")
    expect(html).toContain("1M")
    expect(html).toContain("Plan Mode")
  })

  test("marks app-server defaults as automatic while keeping overrides available", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <ChatPreferenceControls
          availableProviders={PROVIDERS}
          selectedProvider="codex"
          model="gpt-5.5"
          modelMode="auto"
          reasoningEffortMode="auto"
          modelOptions={{ reasoningEffort: "high", fastMode: false }}
          onModelChange={() => {}}
          onModelOptionChange={() => {}}
          onModelModeChange={() => {}}
          onReasoningEffortModeChange={() => {}}
        />
      </I18nProvider>
    )

    expect(html).toContain("Automatic: GPT-5.5")
    expect(html).toContain("Automatic: High")
  })

  test("shows the selected sandboxed Codex execution mode", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <ChatPreferenceControls
          availableProviders={PROVIDERS}
          selectedProvider="codex"
          model="gpt-5.5"
          modelOptions={{ reasoningEffort: "high", fastMode: false, executionMode: "standard" }}
          onModelChange={() => {}}
          onModelOptionChange={() => {}}
        />
      </I18nProvider>
    )

    expect(html).toContain("Sandboxed")
    expect(html).not.toContain(">Unrestricted<")
  })
})
