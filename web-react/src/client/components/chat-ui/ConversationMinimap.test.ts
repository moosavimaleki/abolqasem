import { describe, expect, test } from "bun:test"
import { buildPositionedItems, MAX_MINIMAP_ITEMS, type MessageIndexItem } from "./ConversationMinimap"

function item(id: string, sequence: number, role: MessageIndexItem["role"], loaded = true): MessageIndexItem {
  return {
    id,
    sequence,
    role,
    loaded,
    estimatedHeight: sequence * 100,
  }
}

describe("buildPositionedItems", () => {
  test("indexes only user messages", () => {
    const positioned = buildPositionedItems([
      item("system-1", 0, "system"),
      item("user-1", 1, "user"),
      item("assistant-1", 2, "assistant"),
      item("tool-1", 3, "tool"),
      item("user-2", 4, "user", false),
    ])

    expect(positioned.map((entry) => entry.id)).toEqual(["user-1", "user-2"])
    expect(positioned[1]?.loaded).toBe(false)
  })

  test("uses equal spacing for every user mark", () => {
    const positioned = buildPositionedItems([
      item("user-short", 3, "user"),
      item("assistant-long", 0, "assistant"),
      item("user-long", 1, "user"),
      item("user-middle", 2, "user"),
    ])

    expect(positioned.map((entry) => entry.id)).toEqual(["user-long", "user-middle", "user-short"])
    expect(positioned[0]?.topPercent).toBeCloseTo(16.666, 2)
    expect(positioned[1]?.topPercent).toBeCloseTo(50, 2)
    expect(positioned[2]?.topPercent).toBeCloseTo(83.333, 2)
  })

  test("keeps only the latest one hundred user messages", () => {
    const positioned = buildPositionedItems(
      Array.from({ length: MAX_MINIMAP_ITEMS + 7 }, (_, sequence) => item(`user-${sequence}`, sequence, "user")),
    )

    expect(positioned).toHaveLength(MAX_MINIMAP_ITEMS)
    expect(positioned[0]?.id).toBe("user-7")
    expect(positioned.at(-1)?.id).toBe(`user-${MAX_MINIMAP_ITEMS + 6}`)
  })
})
