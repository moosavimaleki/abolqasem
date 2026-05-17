import { describe, expect, test } from "bun:test"
import type { HydratedTranscriptMessage } from "../../../shared/types"
import { buildAssistantReaderDocument } from "./readerBlocks"

function user(id: string): HydratedTranscriptMessage {
  return {
    kind: "user_prompt",
    id,
    timestamp: "2026-01-01T00:00:00.000Z",
    content: `User ${id}`,
  }
}

function assistant(id: string, text: string, hidden = false): HydratedTranscriptMessage {
  return {
    kind: "assistant_text",
    id,
    timestamp: "2026-01-01T00:00:00.000Z",
    text,
    hidden,
  }
}

function status(id: string): HydratedTranscriptMessage {
  return {
    kind: "status",
    id,
    timestamp: "2026-01-01T00:00:00.000Z",
    status: "running",
  }
}

describe("buildAssistantReaderDocument", () => {
  test("collects all assistant text messages in the same user-bounded block", () => {
    const document = buildAssistantReaderDocument([
      user("user-1"),
      assistant("assistant-1", "First chunk"),
      status("status-1"),
      assistant("assistant-2", "Second chunk"),
      user("user-2"),
      assistant("assistant-3", "Next turn"),
    ], "assistant-2")

    expect(document?.messageIds).toEqual(["assistant-1", "assistant-2"])
    expect(document?.content).toBe("First chunk\n\n---\n\nSecond chunk")
  })

  test("collects the final assistant block when no next user prompt exists yet", () => {
    const document = buildAssistantReaderDocument([
      user("user-1"),
      assistant("assistant-1", "Older turn"),
      user("user-2"),
      assistant("assistant-2", "Tail chunk one"),
      status("status-1"),
      assistant("assistant-3", "Tail chunk two"),
    ], "assistant-2")

    expect(document?.messageIds).toEqual(["assistant-2", "assistant-3"])
    expect(document?.content).toBe("Tail chunk one\n\n---\n\nTail chunk two")
  })

  test("skips hidden and empty assistant chunks in the reader content", () => {
    const document = buildAssistantReaderDocument([
      user("user-1"),
      assistant("assistant-1", "Visible chunk"),
      assistant("assistant-2", "Hidden chunk", true),
      assistant("assistant-3", "   "),
    ], "assistant-1")

    expect(document?.messageIds).toEqual(["assistant-1"])
    expect(document?.content).toBe("Visible chunk")
  })
})
