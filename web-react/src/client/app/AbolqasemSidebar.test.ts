import { describe, expect, test } from "bun:test"
import type { SidebarChatRow, SidebarData } from "../../shared/types"
import { getUsageOrderedSidebarChats } from "./AbolqasemSidebar"

function chat(chatId: string, lastMessageAt: number, unread: boolean): SidebarChatRow {
  return {
    _id: chatId,
    _creationTime: 1,
    chatId,
    title: chatId,
    status: "idle",
    unread,
    localPath: "/tmp/project",
    provider: "codex",
    lastMessageAt,
    hasAutomation: false,
  }
}

function sidebarData(chats: SidebarChatRow[]): SidebarData {
  return {
    projectGroups: [{
      groupKey: "project",
      title: "Project",
      realTitle: "Project",
      localPath: "/tmp/project",
      chats,
      previewChats: [],
      olderChats: [],
      defaultCollapsed: false,
    }],
  }
}

describe("getUsageOrderedSidebarChats", () => {
  test("keeps chat placement based on activity when unread is cleared", () => {
    const olderUnread = chat("older", 100, true)
    const newerRead = chat("newer", 200, false)

    expect(getUsageOrderedSidebarChats(sidebarData([olderUnread, newerRead])).map(({ chat }) => chat.chatId))
      .toEqual(["newer", "older"])

    olderUnread.unread = false
    expect(getUsageOrderedSidebarChats(sidebarData([olderUnread, newerRead])).map(({ chat }) => chat.chatId))
      .toEqual(["newer", "older"])
  })
})
