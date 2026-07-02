import { describe, expect, test } from "bun:test"
import { filterTerminalInputForTmux, getMacOptionInputSequence, getTerminalOptions, getTerminalRowDirection } from "./TerminalPane"

describe("getTerminalOptions", () => {
  test("treats Option as Meta on macOS", () => {
    const options = getTerminalOptions(1_000, { foreground: "#fff" }, "MacIntel")

    expect(options.macOptionIsMeta).toBe(true)
    expect(options.scrollback).toBe(1_000)
    expect(options.lineHeight).toBe(1)
  })

  test("does not enable macOS Option behavior on non-mac platforms", () => {
    const options = getTerminalOptions(500, { foreground: "#fff" }, "Linux x86_64")

    expect(options.macOptionIsMeta).toBe(false)
    expect(options.scrollback).toBe(500)
  })
})

describe("getMacOptionInputSequence", () => {
  test("maps plain arrow keys to standard escape sequences", () => {
    expect(getMacOptionInputSequence({
      key: "ArrowUp",
      altKey: false,
      ctrlKey: false,
      metaKey: false,
    }, "MacIntel")).toBe("\x1b[A")

    expect(getMacOptionInputSequence({
      key: "ArrowDown",
      altKey: false,
      ctrlKey: false,
      metaKey: false,
    }, "MacIntel")).toBe("\x1b[B")
  })

  test("maps Option+Left and Option+Right to shell word motion on macOS", () => {
    expect(getMacOptionInputSequence({
      key: "ArrowLeft",
      altKey: true,
      ctrlKey: false,
      metaKey: false,
    }, "MacIntel")).toBe("\x1bb")

    expect(getMacOptionInputSequence({
      key: "ArrowRight",
      altKey: true,
      ctrlKey: false,
      metaKey: false,
    }, "MacIntel")).toBe("\x1bf")
  })

  test("maps Option+Backspace to backward delete word on macOS", () => {
    expect(getMacOptionInputSequence({
      key: "Backspace",
      altKey: true,
      ctrlKey: false,
      metaKey: false,
    }, "MacIntel")).toBe("\x1b\x7f")
  })

  test("maps Command+Delete to kill backward and forward by line on macOS", () => {
    expect(getMacOptionInputSequence({
      key: "Backspace",
      altKey: false,
      ctrlKey: false,
      metaKey: true,
    }, "MacIntel")).toBe("\x15")

    expect(getMacOptionInputSequence({
      key: "Delete",
      altKey: false,
      ctrlKey: false,
      metaKey: true,
    }, "MacIntel")).toBe("\x0b")
  })

  test("ignores non-mac or modified key combinations", () => {
    expect(getMacOptionInputSequence({
      key: "ArrowLeft",
      altKey: true,
      ctrlKey: false,
      metaKey: false,
    }, "Linux x86_64")).toBeNull()

    expect(getMacOptionInputSequence({
      key: "ArrowLeft",
      altKey: true,
      ctrlKey: true,
      metaKey: false,
    }, "MacIntel")).toBeNull()
  })
})

describe("filterTerminalInputForTmux", () => {
  test("removes terminal device-attribute responses before forwarding input to tmux", () => {
    expect(filterTerminalInputForTmux("\x1b[>0;276;0c")).toBe("")
    expect(filterTerminalInputForTmux("سلام\x1b[?1;2c\r")).toBe("سلام\r")
  })

  test("keeps ordinary terminal key sequences", () => {
    expect(filterTerminalInputForTmux("\x1b[A\r")).toBe("\x1b[A\r")
  })
})

describe("getTerminalRowDirection", () => {
  test("keeps normal mode left-to-right", () => {
    expect(getTerminalRowDirection("سلام", "normal")).toBe("ltr")
  })

  test("forces rtl mode for every row", () => {
    expect(getTerminalRowDirection("npm test", "rtl")).toBe("rtl")
  })

  test("uses rtl in smart mode only for prose-like rtl rows", () => {
    expect(getTerminalRowDirection("سلام آماده‌ام", "smart")).toBe("rtl")
    expect(getTerminalRowDirection("const value = 'سلام'", "smart")).toBe("ltr")
    expect(getTerminalRowDirection("npm test", "smart")).toBe("ltr")
  })
})
