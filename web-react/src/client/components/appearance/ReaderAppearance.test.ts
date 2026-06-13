import { describe, expect, test } from "bun:test"
import { defaultReaderSettings, getAppearanceArticleClassName } from "./ReaderAppearance"

describe("reader appearance defaults", () => {
  test("uses wide reader width by default", () => {
    expect(defaultReaderSettings.width).toBe("wide")
  })

  test("uses the expanded wide article width", () => {
    expect(getAppearanceArticleClassName(defaultReaderSettings)).toContain("max-w-[1280px]")
  })
})
