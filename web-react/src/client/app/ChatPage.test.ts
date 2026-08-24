import { describe, expect, test } from "bun:test"
import {
  getTerminalPanelDefaultSizes,
  getTranscriptTailVersion,
  getOrderedRightSidebarLayout,
  getRightSidebarPanelDefaultSizes,
  getRightSidebarSizePercent,
  getRightSidebarSizePx,
  getIgnoreFolderEntryFromDiffPath,
  hasFileDragTypes,
  shouldUseMobileRightSidebarOverlay,
  shouldAutoFollowTranscriptResize,
  shouldShowTranscriptUnreadIndicator,
} from "./ChatPage"

describe("hasFileDragTypes", () => {
  test("returns true when file drags are present", () => {
    expect(hasFileDragTypes(["text/plain", "Files"])).toBe(true)
  })

  test("returns false for non-file drags", () => {
    expect(hasFileDragTypes(["text/plain", "text/uri-list"])).toBe(false)
  })
})

describe("getIgnoreFolderEntryFromDiffPath", () => {
  test("returns the parent folder with a trailing slash", () => {
    expect(getIgnoreFolderEntryFromDiffPath("tmp/cache/output.log")).toBe("tmp/cache/")
  })

  test("normalizes repeated separators before deriving the folder", () => {
    expect(getIgnoreFolderEntryFromDiffPath("tmp//cache/output.log")).toBe("tmp/cache/")
  })

  test("returns null for repo root files", () => {
    expect(getIgnoreFolderEntryFromDiffPath("scratch.log")).toBeNull()
  })
})

describe("shouldAutoFollowTranscriptResize", () => {
  test("keeps auto-follow enabled while the scroll button is hidden", () => {
    expect(shouldAutoFollowTranscriptResize(false, 0, 1_000)).toBe(true)
  })

  test("keeps auto-follow enabled briefly after chat selection", () => {
    expect(shouldAutoFollowTranscriptResize(true, 2_000, 1_500)).toBe(true)
  })

  test("stops forcing auto-follow after the selection window expires", () => {
    expect(shouldAutoFollowTranscriptResize(true, 2_000, 2_000)).toBe(false)
  })
})

describe("shouldShowTranscriptUnreadIndicator", () => {
  test("lights the indicator only for appended messages while scrolled away from the end", () => {
    expect(shouldShowTranscriptUnreadIndicator(2, "msg-2", [{ id: "msg-1" }, { id: "msg-2" }, { id: "msg-3" }], false)).toBe(true)
  })

  test("does not light the indicator for prepended history or when already at the end", () => {
    expect(shouldShowTranscriptUnreadIndicator(2, "msg-2", [{ id: "older-1" }, { id: "older-2" }, { id: "msg-2" }], false)).toBe(false)
    expect(shouldShowTranscriptUnreadIndicator(2, "msg-2", [{ id: "msg-1" }, { id: "msg-2" }, { id: "msg-3" }], true)).toBe(false)
  })
})

describe("getTranscriptTailVersion", () => {
  test("changes when the last tmux capture text changes without appending a row", () => {
    const first = getTranscriptTailVersion([{ id: "tmux-capture-chat-1", kind: "assistant_text", text: "hello" }])
    const second = getTranscriptTailVersion([{ id: "tmux-capture-chat-1", kind: "assistant_text", text: "hello\nnew output" }])

    expect(second).not.toBe(first)
  })
})

describe("shouldUseMobileRightSidebarOverlay", () => {
  test("enables the overlay below the mobile breakpoint", () => {
    expect(shouldUseMobileRightSidebarOverlay(767)).toBe(true)
  })

  test("keeps the desktop split layout at and above the breakpoint", () => {
    expect(shouldUseMobileRightSidebarOverlay(768)).toBe(false)
    expect(shouldUseMobileRightSidebarOverlay(1280)).toBe(false)
  })
})

describe("getTerminalPanelDefaultSizes", () => {
  test("uses persisted terminal sizes while the terminal is visible", () => {
    expect(getTerminalPanelDefaultSizes(true, [68, 32])).toEqual([68, 32])
  })

  test("collapses the terminal panel defaults while the terminal is hidden", () => {
    expect(getTerminalPanelDefaultSizes(false, [68, 32])).toEqual([100, 0])
  })
})


describe("right sidebar pixel sizing", () => {
  test("converts the saved pixel width to a panel percentage", () => {
    expect(getRightSidebarSizePercent(420, 1_200)).toBe(35)
  })

  test("keeps the panel at the minimum pixel width", () => {
    expect(getRightSidebarSizePercent(100, 1_000)).toBe(37)
  })

  test("caps the panel so the workspace keeps its minimum share", () => {
    expect(getRightSidebarSizePercent(1_200, 1_000)).toBe(80)
  })

  test("converts the panel percentage back to pixels", () => {
    expect(getRightSidebarSizePx(35, 1_200)).toBe(420)
  })

  test("collapses the hidden desktop sidebar instead of reserving layout space", () => {
    expect(getRightSidebarPanelDefaultSizes(false, 28)).toEqual({
      workspace: 100,
      rightSidebar: 0,
    })
  })

  test("preserves the visible desktop sidebar size", () => {
    expect(getRightSidebarPanelDefaultSizes(true, 28)).toEqual({
      workspace: 72,
      rightSidebar: 28,
    })
  })

  test("orders right-sidebar layout to match the panel DOM order in RTL", () => {
    expect(Object.keys(getOrderedRightSidebarLayout(72, 28, "rtl"))).toEqual(["rightSidebar", "workspace"])
  })

  test("keeps right-sidebar layout order LTR by default", () => {
    expect(Object.keys(getOrderedRightSidebarLayout(72, 28, "ltr"))).toEqual(["workspace", "rightSidebar"])
  })
})
