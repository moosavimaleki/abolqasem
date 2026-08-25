import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { I18nProvider } from "../../../i18n/context"
import { ChatRow } from "./ChatRow"

const baseChat = {
  _id: "chat-row-1",
  _creationTime: 1,
  chatId: "chat-1",
  title: "Test chat",
  status: "idle" as const,
  unread: false,
  localPath: "/tmp/project",
  provider: "codex" as const,
  lastMessageAt: 0,
  hasAutomation: false,
}

describe("ChatRow", () => {
  test("renders the relative age label by default", () => {
    const html = renderToStaticMarkup(
      <ChatRow
        chat={baseChat}
        activeChatId={null}
        nowMs={60_000}
        onSelectChat={() => undefined}
        onRenameChat={() => undefined}
        onOpenInFinder={() => undefined}
        onForkChat={() => undefined}
        onConvertChat={() => undefined}
        onArchiveChat={() => undefined}
        onDeleteChat={() => undefined}
      />
    )

    expect(html).toContain(">1m<")
  })

  test("falls back to the chat creation time for the age label", () => {
    const html = renderToStaticMarkup(
      <ChatRow
        chat={{ ...baseChat, _creationTime: 30_000, lastMessageAt: undefined }}
        activeChatId={null}
        nowMs={60_000}
        onSelectChat={() => undefined}
        onRenameChat={() => undefined}
        onOpenInFinder={() => undefined}
        onForkChat={() => undefined}
        onConvertChat={() => undefined}
        onArchiveChat={() => undefined}
        onDeleteChat={() => undefined}
      />
    )

    expect(html).toContain(">now<")
  })

  test("prefers lastMessageAt over creation time for the age label", () => {
    const html = renderToStaticMarkup(
      <ChatRow
        chat={{ ...baseChat, _creationTime: 59_000, lastMessageAt: 0 }}
        activeChatId={null}
        nowMs={60_000}
        onSelectChat={() => undefined}
        onRenameChat={() => undefined}
        onOpenInFinder={() => undefined}
        onForkChat={() => undefined}
        onConvertChat={() => undefined}
        onArchiveChat={() => undefined}
        onDeleteChat={() => undefined}
      />
    )

    expect(html).toContain(">1m<")
    expect(html).not.toContain(">now<")
  })

  test("renders the shortcut hint when the modifier is held", () => {
    const html = renderToStaticMarkup(
      <ChatRow
        chat={baseChat}
        activeChatId={null}
        nowMs={60_000}
        shortcutHint="1"
        showShortcutHint
        onSelectChat={() => undefined}
        onRenameChat={() => undefined}
        onOpenInFinder={() => undefined}
        onForkChat={() => undefined}
        onConvertChat={() => undefined}
        onArchiveChat={() => undefined}
        onDeleteChat={() => undefined}
      />
    )

    expect(html).toContain(">1<")
    expect(html).toContain("<kbd")
    expect(html).not.toContain(">1m<")
  })

  test("renders a fork action next to the archive action when the chat can fork", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <ChatRow
          chat={{ ...baseChat, canFork: true }}
          activeChatId={null}
          nowMs={60_000}
          onSelectChat={() => undefined}
          onRenameChat={() => undefined}
          onOpenInFinder={() => undefined}
          onForkChat={() => undefined}
          onConvertChat={() => undefined}
          onArchiveChat={() => undefined}
          onDeleteChat={() => undefined}
        />
      </I18nProvider>
    )

    expect(html).toContain("Fork chat")
    expect(html).toContain("Archive chat")
  })

  test("localizes default chat titles in Persian without changing compact age labels", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="fa">
        <ChatRow
          chat={{ ...baseChat, title: "New Chat" }}
          activeChatId={null}
          nowMs={60_000}
          onSelectChat={() => undefined}
          onRenameChat={() => undefined}
          onOpenInFinder={() => undefined}
          onForkChat={() => undefined}
          onConvertChat={() => undefined}
          onArchiveChat={() => undefined}
          onDeleteChat={() => undefined}
        />
      </I18nProvider>
    )

    expect(html).toContain("چت جدید")
    expect(html).toContain(">1m<")
    expect(html).not.toContain("New Chat")
  })

  test("renders the provider icon label beside the chat title", () => {
    const html = renderToStaticMarkup(
      <ChatRow
        chat={{ ...baseChat, provider: "codex" }}
        activeChatId={null}
        nowMs={60_000}
        onSelectChat={() => undefined}
        onRenameChat={() => undefined}
        onOpenInFinder={() => undefined}
        onForkChat={() => undefined}
        onConvertChat={() => undefined}
        onArchiveChat={() => undefined}
        onDeleteChat={() => undefined}
      />
    )

    expect(html).toContain("aria-label=\"Codex\"")
    expect(html).toContain("Test chat")
    expect(html).not.toContain("text-emerald")
  })

  test("reserves the green activity indicator for unread chats", () => {
    const html = renderToStaticMarkup(
      <I18nProvider locale="en">
        <ChatRow
          chat={{ ...baseChat, unread: true }}
          activeChatId={null}
          nowMs={60_000}
          onSelectChat={() => undefined}
          onRenameChat={() => undefined}
          onOpenInFinder={() => undefined}
          onForkChat={() => undefined}
          onConvertChat={() => undefined}
          onArchiveChat={() => undefined}
          onDeleteChat={() => undefined}
        />
      </I18nProvider>
    )

    expect(html).toContain("aria-label=\"Unread\"")
    expect(html).toContain("bg-emerald-400")
  })
})
