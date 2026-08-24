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
    expect(html).toContain("Edit")
    expect(html).toContain("Steer")
  })
})
