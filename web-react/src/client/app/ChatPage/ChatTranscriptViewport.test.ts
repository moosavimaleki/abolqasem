import { describe, expect, test } from "bun:test"
import { shouldShowTranscriptJumpHint, transcriptJumpDistance } from "./ChatTranscriptViewport"

describe("transcript jump hint", () => {
  test("only surfaces the hint for a quick meaningful scroll", () => {
    expect(shouldShowTranscriptJumpHint(100, 380, 80)).toBe(true)
    expect(shouldShowTranscriptJumpHint(100, 180, 80)).toBe(false)
    expect(shouldShowTranscriptJumpHint(100, 380, 280)).toBe(false)
  })

  test("uses a readable viewport-relative shift jump", () => {
    expect(transcriptJumpDistance(800, "down")).toBe(576)
    expect(transcriptJumpDistance(120, "up")).toBe(-280)
  })
})
