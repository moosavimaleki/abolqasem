import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nProvider } from "../../i18n/context"
import { ChatNavbar } from "./ChatNavbar"

describe("ChatNavbar", () => {
  test("uses an icon-only copy entry even when only Codex session tools are available", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="fa">
        <ChatNavbar
          sidebarCollapsed={false}
          onOpenSidebar={() => undefined}
          onExpandSidebar={() => undefined}
          onNewChat={() => undefined}
          onAddProject={() => undefined}
          sessionId="01a-session-id"
          sessionPath="/home/test/.codex/sessions/rollout-01a-session-id.jsonl"
        />
      </I18nProvider>,
    )

    expect(html).toContain('aria-label="کپی"')
    expect(html).not.toContain("کپی سشن")
  })
})
