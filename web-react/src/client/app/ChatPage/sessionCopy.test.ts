import { describe, expect, test } from "bun:test"
import type { HydratedTranscriptMessage } from "../../../shared/types"
import { buildSessionCopyText, collectSessionCopyTurns } from "./sessionCopy"

function user(id: string, content: string): HydratedTranscriptMessage {
  return { kind: "user_prompt", id, timestamp: "2026-01-01T00:00:00.000Z", content }
}

function assistant(id: string, text: string): HydratedTranscriptMessage {
  return { kind: "assistant_text", id, timestamp: "2026-01-01T00:00:00.000Z", text }
}

describe("sessionCopy", () => {
  test("counts a user prompt and all of its assistant replies as one turn", () => {
    const messages = [
      user("user-1", "اولین پیام"),
      assistant("assistant-1", "پاسخ اول"),
      assistant("assistant-2", "پاسخ دوم"),
      user("user-2", "دومین پیام"),
      assistant("assistant-3", "پاسخ سوم"),
    ]

    expect(collectSessionCopyTurns(messages)).toHaveLength(2)
    expect(buildSessionCopyText(messages, 1, { user: "کاربر", assistant: "AI" })).toBe(
      "کاربر:\nدومین پیام\n\nAI:\nپاسخ سوم",
    )
    expect(buildSessionCopyText(messages, 5, { user: "کاربر", assistant: "AI" })).toContain(
      "AI:\nپاسخ اول\n\nپاسخ دوم",
    )
  })

  test("does not copy hidden or empty messages", () => {
    const messages = [
      { ...user("hidden", "نباید کپی شود"), hidden: true },
      user("user-1", "پیام واقعی"),
      { ...assistant("hidden-answer", "پاسخ مخفی"), hidden: true },
      assistant("assistant-1", "پاسخ واقعی"),
    ]

    expect(buildSessionCopyText(messages, 5, { user: "کاربر", assistant: "AI" })).toBe(
      "کاربر:\nپیام واقعی\n\nAI:\nپاسخ واقعی",
    )
  })
})
