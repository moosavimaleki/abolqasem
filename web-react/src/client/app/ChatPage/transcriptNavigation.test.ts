import { describe, expect, test } from "bun:test"
import type { HydratedTranscriptMessage } from "../../../shared/types"
import {
  findLoadedSearchMessage,
  findPreviousUserPromptMessage,
  findTranscriptRowTarget,
} from "./transcriptNavigation"

function user(id: string, content = `User ${id}`): HydratedTranscriptMessage {
  return {
    kind: "user_prompt",
    id,
    timestamp: "2026-01-01T00:00:00.000Z",
    content,
  }
}

function assistant(id: string, text = `Assistant ${id}`, messageId?: string): HydratedTranscriptMessage {
  return {
    kind: "assistant_text",
    id,
    messageId,
    timestamp: "2026-01-01T00:00:00.000Z",
    text,
  }
}

const latestToolIds = {
  AskUserQuestion: null,
  ExitPlanMode: null,
  TodoWrite: null,
}

describe("transcriptNavigation", () => {
  test("walks to older user prompts from the last jumped prompt", () => {
    const messages = [
      user("user-1"),
      assistant("assistant-1"),
      user("user-2"),
      assistant("assistant-2"),
      user("user-3"),
    ]

    expect(findPreviousUserPromptMessage(messages)?.id).toBe("user-3")
    expect(findPreviousUserPromptMessage(messages, "user-3")?.id).toBe("user-2")
    expect(findPreviousUserPromptMessage(messages, "user-2")?.id).toBe("user-1")
    expect(findPreviousUserPromptMessage(messages, "user-1")).toBeNull()
  })

  test("ignores empty and hidden user prompts", () => {
    const messages = [
      user("user-1", "First"),
      { ...user("user-2", "Hidden"), hidden: true },
      user("user-3", "   "),
    ]

    expect(findPreviousUserPromptMessage(messages)?.id).toBe("user-1")
  })

  test("matches search results by transcript entry id and provider message id", () => {
    const messages = [
      user("user-1"),
      assistant("assistant-1", "First", "provider-message-1"),
    ]

    expect(findLoadedSearchMessage(messages, { entry_id: "assistant-1" })?.id).toBe("assistant-1")
    expect(findLoadedSearchMessage(messages, { message_id: "provider-message-1" })?.id).toBe("assistant-1")
  })

  test("resolves the rendered row for a loaded transcript message", () => {
    const messages = [
      user("user-1"),
      assistant("assistant-1"),
    ]

    const target = findTranscriptRowTarget(messages, latestToolIds, messages[1]!)

    expect(target?.rowIndex).toBe(1)
    expect(target?.row.id).toBe("assistant-1")
  })
})
