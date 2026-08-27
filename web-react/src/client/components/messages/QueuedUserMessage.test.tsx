import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import type { QueuedChatMessage } from "../../../shared/types"
import { QueuedUserMessage } from "./QueuedUserMessage"

describe("QueuedUserMessage", () => {
  test("renders a compact queued message row without an inline editor", () => {
    const message: QueuedChatMessage = {
      id: "queued-1",
      content: "Queued follow-up",
      attachments: [],
      createdAt: Date.now(),
    }

    const html = renderToStaticMarkup(
      <QueuedUserMessage
        message={message}
        onRemove={() => Promise.resolve()}
        onSteer={() => Promise.resolve()}
        onEdit={() => Promise.resolve()}
      />
    )

    expect(html).toContain("Queued follow-up")
    expect(html).toContain("text-start")
    expect(html).toContain('dir="auto"')
    expect(html).not.toContain("text-right")
    expect(html).not.toContain("textarea")
    expect(html).toContain("ویرایش")
    expect(html).toContain("ارسال")
    expect(html).toContain("متن این پیامِ در صف")
    expect(html).toContain("نخستین فرصت")
    expect(html).toContain("متوقف می‌کند")
  })

  test("renders a legacy steering record as a non-interactive transitional row", () => {
    const html = renderToStaticMarkup(
      <QueuedUserMessage
        message={{ id: "queued-steering", content: "Steered follow-up", attachments: [], createdAt: Date.now(), deliveryState: "steering" }}
        onRemove={() => Promise.resolve()}
        onSteer={() => Promise.resolve()}
        onEdit={() => Promise.resolve()}
      />
    )

    expect(html).toContain('data-delivery-state="steering"')
    expect(html).toContain("Steered follow-up")
    expect(html).toContain("در حال تحویل")
    expect(html).not.toContain(">Steer<")
  })
})
