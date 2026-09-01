import { describe, expect, test } from "vitest"
import { limitHistoryPoints } from "./UsageHistoryChart"

describe("limitHistoryPoints", () => {
  test("keeps all points when history is already small", () => {
    const input = [1, 2, 3]
    expect(limitHistoryPoints(input, 10)).toEqual(input)
  })

  test("keeps the first and last samples while bounding a large history", () => {
    const input = Array.from({ length: 1000 }, (_, index) => index)
    const output = limitHistoryPoints(input, 25)
    expect(output).toHaveLength(25)
    expect(output[0]).toBe(0)
    expect(output.at(-1)).toBe(999)
    expect(new Set(output).size).toBe(25)
  })
})
