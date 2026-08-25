import { describe, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import type { SidebarChatRow, SidebarData } from "../../shared/types"
import { I18nProvider } from "../i18n/context"
import { getUsageOrderedSidebarChats, SidebarPrimaryControls } from "./AbolqasemSidebar"

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

describe("SidebarPrimaryControls", () => {
  test("places primary actions and search before the view filter", () => {
    const html = renderToStaticMarkup(createElement(
      I18nProvider,
      { locale: "fa" },
      createElement(SidebarPrimaryControls, {
        data: sidebarData([]),
        locale: "fa",
        sidebarView: "chats",
        onChangeView: () => undefined,
        onNewChat: () => undefined,
        onAddProject: () => undefined,
        onSelectChat: () => undefined,
      }),
    ))

    const actionsIndex = html.indexOf('data-sidebar-control="actions"')
    const searchIndex = html.indexOf('data-sidebar-control="search"')
    const filterIndex = html.indexOf('data-sidebar-control="view-filter"')

    expect(actionsIndex).toBeGreaterThanOrEqual(0)
    expect(searchIndex).toBeGreaterThan(actionsIndex)
    expect(filterIndex).toBeGreaterThan(searchIndex)
    expect(html.match(/>چت‌ها</g)).toHaveLength(1)
    expect(html).toContain('data-sidebar-action="new-chat"')
    expect(html).toContain('data-sidebar-action="add-project"')
    expect(html).not.toContain('bg-muted/55')
  })
})
