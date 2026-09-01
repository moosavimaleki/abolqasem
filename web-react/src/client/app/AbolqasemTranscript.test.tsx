import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { CollapsedToolGroup } from "../components/messages/CollapsedToolGroup"
import { I18nProvider } from "../i18n/context"
import type { HydratedTranscriptMessage } from "../../shared/types"
import {
  buildResolvedTranscriptRows,
  computeStableResolvedTranscriptRows,
  AbolqasemTranscript,
  type StableResolvedTranscriptRowsState,
} from "./AbolqasemTranscript"

const ROW_WRAPPER_CLASS = "mx-auto max-w-[800px] pb-5"

function renderTranscript(
  messages: HydratedTranscriptMessage[],
  latestToolIds = { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null },
  onRetryTurn?: () => Promise<void>,
) {
  return renderToStaticMarkup(
    <I18nProvider locale="en">
      <AbolqasemTranscript
        messages={messages}
        isLoading={false}
        latestToolIds={latestToolIds}
        onOpenLocalLink={() => undefined}
        onAskUserQuestionSubmit={() => undefined}
        onApprovalRequestSubmit={() => undefined}
        onExitPlanModeConfirm={() => undefined}
        onRetryTurn={onRetryTurn}
      />
    </I18nProvider>
  )
}

function countRowWrappers(html: string) {
  return html.split(ROW_WRAPPER_CLASS).length - 1
}

function createToolMessage(id: string, toolId = id): HydratedTranscriptMessage {
  return {
    id,
    kind: "tool",
    toolKind: "bash",
    toolName: "Bash",
    toolId,
    input: {
      command: `echo ${id}`,
      description: `Run ${id}`,
    },
    timestamp: new Date().toISOString(),
  }
}

function createCommandMessage(id: string, command: string, output = "ok"): HydratedTranscriptMessage {
  return {
    id,
    kind: "command_execution",
    itemId: id,
    command,
    cwd: "/work",
    status: "completed",
    aggregatedOutput: output,
    exitCode: 0,
    timestamp: new Date().toISOString(),
  }
}

describe("AbolqasemTranscript", () => {
  test("offers a retry for a failed result only when the current turn is retryable", () => {
    const failure: HydratedTranscriptMessage = {
      id: "turn-error-1",
      kind: "result",
      success: false,
      cancelled: false,
      result: "Request timed out",
      durationMs: 4028,
      timestamp: new Date().toISOString(),
    }

    expect(renderTranscript([failure])).not.toContain(">Retry<")

    const html = renderTranscript([failure], undefined, async () => undefined)
    expect(html).toContain(">Retry<")
    expect(html).toContain("Sends “Continue” in this same session.")
  })

  test("renders native Codex command collapsed, plus file-change and live plan cards", () => {
    const html = renderTranscript([
      { id: "cmd", kind: "command_execution", itemId: "cmd-1", command: "go test ./...", cwd: "/work", status: "completed", aggregatedOutput: "ok", exitCode: 0, timestamp: new Date().toISOString() },
      { id: "files", kind: "file_change", itemId: "files-1", status: "completed", changes: [{ path: "/work/main.go", kind: "update", diff: "@@ -88,2 +89,3 @@\n-old\n+new" }], output: "", timestamp: new Date().toISOString() },
      { id: "plan", kind: "turn_plan", turnId: "turn-1", explanation: "Implement safely", plan: [{ step: "Run tests", status: "inProgress" }], timestamp: new Date().toISOString() },
    ])
    expect(html).toContain("go test ./...")
    expect(html).toContain("Completed")
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain("No output yet")
    expect(html).toContain("1 files changed")
    expect(html).not.toContain("/work/main.go")
    expect(html).not.toContain('href="/work/main.go:89"')
    expect(html).toContain('data-live-plan="true"')
    expect(html).toContain("Updating")
    expect(html).toContain("Implement safely")
    expect(html).toContain("Run tests")
  })

  test("renders a pending Codex approval with explicit one-time and session choices", () => {
    const html = renderTranscript([
      {
        id: "approval-1",
        kind: "tool",
        toolKind: "approval_request",
        toolName: "ApprovalRequest",
        toolId: "rpc-17",
        input: {
          approvalKind: "command_execution",
          command: "printf changed > README.md",
          cwd: "/work",
          reason: "Write the requested note",
        },
        timestamp: new Date().toISOString(),
      },
    ], { AskUserQuestion: null, ApprovalRequest: "approval-1", ExitPlanMode: null, TodoWrite: null })

    expect(html).toContain('data-approval-request="true"')
    expect(html).toContain("Run this command?")
    expect(html).toContain("printf changed &gt; README.md")
    expect(html).toContain("Allow once")
    expect(html).toContain("Allow for session")
    expect(html).toContain("Deny")
  })

  test("renders proposed plans as compact collapsed cards without protocol tags", () => {
    const html = renderTranscript([
      { id: "proposed", kind: "proposed_plan", turnId: "turn-1", plan: "# Refactor plan\n\n- Run tests", timestamp: new Date().toISOString() },
    ])
    expect(html).toContain('data-proposed-plan="true"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain("border-border bg-card/50")
    expect(html).toContain("Refactor plan")
    expect(html).not.toContain("Run tests")
    expect(html).not.toContain("proposed_plan")
  })

  test("groups consecutive native commands behind a Codex Mobile style summary", () => {
    const messages = [
      createCommandMessage("cmd-1", "/usr/bin/zsh -lc 'rtk git diff --check'", "first private output"),
      createCommandMessage("cmd-2", "/usr/bin/zsh -lc 'rtk npm test'", "second private output"),
    ]
    const rows = buildResolvedTranscriptRows(messages, {
      isLoading: false,
      latestToolIds: { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null },
    })
    const html = renderTranscript(messages)

    expect(rows).toHaveLength(1)
    expect(rows[0]?.kind).toBe("tool-group")
    expect(html).toContain("2 commands · latest: rtk npm test")
    expect(html).toContain('data-command-group="true"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain("first private output")
    expect(html).not.toContain("second private output")
  })

  test("renders user attachment cards outside the user bubble", () => {
    const html = renderTranscript([
      {
        id: "user-1",
        kind: "user_prompt",
        content: "What are these files about?",
        attachments: [{
          id: "file-1",
          kind: "file",
          displayName: "spec.pdf",
          absolutePath: "/tmp/project/.abolqasem/uploads/spec.pdf",
          relativePath: "./.abolqasem/uploads/spec.pdf",
          contentUrl: "/api/projects/project-1/uploads/spec.pdf/content",
          mimeType: "application/pdf",
          size: 1234,
        }],
        timestamp: new Date().toISOString(),
      },
    ])

    expect(html).toContain("spec.pdf")
    expect(html).toContain("application/pdf")
    expect(html).toContain("What are these files about?")
  })

  test("renders uploaded image attachments using the server content URL", () => {
    const html = renderTranscript([
      {
        id: "user-2",
        kind: "user_prompt",
        content: "",
        attachments: [{
          id: "image-1",
          kind: "image",
          displayName: "mock.png",
          absolutePath: "/tmp/project/.abolqasem/uploads/mock.png",
          relativePath: "./.abolqasem/uploads/mock.png",
          contentUrl: "/api/projects/project-1/uploads/mock.png/content",
          mimeType: "image/png",
          size: 512,
        }],
        timestamp: new Date().toISOString(),
      },
    ])

    expect(html).toContain("/api/projects/project-1/uploads/mock.png/content")
    expect(html).toContain("mock.png")
    expect(html).toContain("max-h-[300px]")
    expect(html).toContain("min-w-[200px]")
  })

  test("renders images before file attachments and user text", () => {
    const html = renderTranscript([
      {
        id: "user-3",
        kind: "user_prompt",
        content: "Please review these.",
        attachments: [
          {
            id: "image-2",
            kind: "image",
            displayName: "mock.png",
            absolutePath: "/tmp/project/.abolqasem/uploads/mock.png",
            relativePath: "./.abolqasem/uploads/mock.png",
            contentUrl: "/api/projects/project-1/uploads/mock.png/content",
            mimeType: "image/png",
            size: 512,
          },
          {
            id: "file-2",
            kind: "file",
            displayName: "spec.pdf",
            absolutePath: "/tmp/project/.abolqasem/uploads/spec.pdf",
            relativePath: "./.abolqasem/uploads/spec.pdf",
            contentUrl: "/api/projects/project-1/uploads/spec.pdf/content",
            mimeType: "application/pdf",
            size: 1234,
          },
        ],
        timestamp: new Date().toISOString(),
      },
    ])

    expect(html).toContain("justify-end gap-3")
    expect(html).toContain("justify-end gap-2")
    expect(html).toContain("Please review these.")
  })

  test("collapses steer system-message text once and renders a steer icon beside the user bubble", () => {
    const html = renderTranscript([
      {
        id: "user-steer-1",
        kind: "user_prompt",
        content: `<system-message>
The user would like you to know the following. Please address the message as you see fit then continue with what you were doing
</system-message>

Please check the latest error first.`,
        steered: true,
        attachments: [],
        timestamp: new Date().toISOString(),
      },
    ])

    expect(html.match(/The user would like you to know the following\./g)).toHaveLength(1)
    expect(html).toContain("System message")
    expect(html).toContain("Please check the latest error first.")
    expect(html).toContain('aria-label="Sent mid-turn"')
  })

  test("renders native Codex turn failures as an accessible inline error", () => {
    const html = renderTranscript([
      {
        id: "turn-error-1",
        kind: "result",
        success: false,
        result: "Your access token could not be refreshed because your refresh token was revoked. Please log out and sign in again.",
        durationMs: 4028,
        timestamp: new Date().toISOString(),
      },
    ])

    expect(html).toContain('role="alert"')
    expect(html).toContain("refresh token was revoked")
  })

  test("renders a live Plan Mode question from the native tool request", () => {
    const html = renderTranscript([
      {
        id: "pending-plan-question",
        kind: "tool",
        toolKind: "ask_user_question",
        toolName: "AskUserQuestion",
        toolId: "ask-1",
        input: {
          questions: [{
            id: "scope",
            header: "Scope",
            question: "Which refactor scope should I use?",
            options: [{ label: "Focused", description: "Only the targeted package" }],
          }],
        },
        timestamp: new Date().toISOString(),
      },
    ], { AskUserQuestion: "pending-plan-question", ExitPlanMode: null, TodoWrite: null })

    expect(html).toContain("Which refactor scope should I use?")
    expect(html).toContain("Focused")
    expect(html).toContain("Scope")
    expect(html).toContain('role="radiogroup"')
    expect(html).toContain('role="radio"')
    expect(html).toContain('aria-checked="false"')
  })

  test("does not render wrappers for context window updates", () => {
    const html = renderTranscript([
      {
        id: "context-window-1",
        kind: "context_window_updated",
        usage: { usedTokens: 100, maxTokens: 1000, compactsAutomatically: false },
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(0)
  })

  test("does not render wrappers for rate limit updates", () => {
    const html = renderTranscript([
      {
        id: "rate-limit-1",
        kind: "rate_limit_updated",
        rateLimits: {
          limitId: "codex",
          primary: { usedPercent: 50, windowDurationMins: 300 },
          secondary: { usedPercent: 10, windowDurationMins: 10080 },
          credits: { balance: "12.5" },
        },
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(0)
  })

  test("renders only the final status row", () => {
    const html = renderTranscript([
      {
        id: "status-1",
        kind: "status",
        status: "working",
        timestamp: new Date().toISOString(),
      },
      {
        id: "status-2",
        kind: "status",
        status: "done",
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(1)
    expect(html).toContain("done")
    expect(html).not.toContain("working")
  })

  test("does not render a wrapper for results hidden by context cleared", () => {
    const html = renderTranscript([
      {
        id: "result-1",
        kind: "result",
        success: true,
        result: "Completed",
        durationMs: 100,
        timestamp: new Date().toISOString(),
      },
      {
        id: "context-cleared-1",
        kind: "context_cleared",
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(1)
    expect(html).toContain("Context Cleared")
    expect(html).not.toContain("Completed")
  })

  test("does not render wrappers for short successful result rows", () => {
    const html = renderTranscript([
      {
        id: "result-short-1",
        kind: "result",
        success: true,
        cancelled: false,
        result: "Hey! 👋",
        durationMs: 2562,
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(0)
  })

  test("renders wrappers for long successful result rows", () => {
    const html = renderTranscript([
      {
        id: "result-long-1",
        kind: "result",
        success: true,
        cancelled: false,
        result: "Done",
        durationMs: 61000,
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(1)
  })

  test("does not render wrappers for duplicate system and account rows", () => {
    const html = renderTranscript([
      {
        id: "system-1",
        kind: "system_init",
        provider: "codex",
        model: "gpt-5",
        tools: [],
        agents: [],
        slashCommands: [],
        mcpServers: [],
        timestamp: new Date().toISOString(),
      },
      {
        id: "system-2",
        kind: "system_init",
        provider: "codex",
        model: "gpt-5",
        tools: [],
        agents: [],
        slashCommands: [],
        mcpServers: [],
        timestamp: new Date().toISOString(),
      },
      {
        id: "account-1",
        kind: "account_info",
        accountInfo: { email: "a@example.com", subscriptionType: "Pro" },
        timestamp: new Date().toISOString(),
      },
      {
        id: "account-2",
        kind: "account_info",
        accountInfo: { email: "a@example.com", subscriptionType: "Pro" },
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(2)
  })

  test("renders one wrapper for visible transcript rows", () => {
    const html = renderTranscript([
      {
        id: "assistant-1",
        kind: "assistant_text",
        text: "Visible text",
        timestamp: new Date().toISOString(),
      },
    ])

    expect(countRowWrappers(html)).toBe(1)
    expect(html).toContain("Visible text")
  })

  test("keeps tool-group row ids stable when the grouped run grows", () => {
    const latestToolIds = { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null }
    const initialRows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      createToolMessage("tool-2"),
    ], {
      isLoading: true,
      latestToolIds,
    })
    const updatedRows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      createToolMessage("tool-2"),
      createToolMessage("tool-3"),
    ], {
      isLoading: true,
      latestToolIds,
    })

    expect(initialRows).toHaveLength(1)
    expect(updatedRows).toHaveLength(1)
    expect(initialRows[0]?.kind).toBe("tool-group")
    expect(updatedRows[0]?.kind).toBe("tool-group")
    expect(initialRows[0]?.id).toBe("tool-group:tool-1")
    expect(updatedRows[0]?.id).toBe("tool-group:tool-1")
  })

  test("groups collapsible tools across hidden context window updates", () => {
    const rows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      {
        id: "context-window-1",
        kind: "context_window_updated",
        usage: { usedTokens: 100, maxTokens: 1000, compactsAutomatically: false },
        timestamp: new Date().toISOString(),
      },
      createToolMessage("tool-2"),
    ], {
      isLoading: true,
      latestToolIds: { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null },
    })

    expect(rows).toHaveLength(1)
    expect(rows[0]?.kind).toBe("tool-group")
    if (rows[0]?.kind !== "tool-group") throw new Error("unexpected row kind")
    expect(rows[0].messages.map((message) => message.id)).toEqual(["tool-1", "tool-2"])
  })

  test("groups collapsible tools across hidden non-final status rows", () => {
    const rows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      {
        id: "status-1",
        kind: "status",
        status: "working",
        timestamp: new Date().toISOString(),
      },
      createToolMessage("tool-2"),
      {
        id: "status-2",
        kind: "status",
        status: "done",
        timestamp: new Date().toISOString(),
      },
    ], {
      isLoading: true,
      latestToolIds: { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null },
    })

    expect(rows).toHaveLength(2)
    expect(rows[0]?.kind).toBe("tool-group")
    if (rows[0]?.kind !== "tool-group") throw new Error("unexpected row kind")
    expect(rows[0].messages.map((message) => message.id)).toEqual(["tool-1", "tool-2"])
    expect(rows[1]?.kind).toBe("single")
  })

  test("groups collapsible tools across hidden short result rows", () => {
    const rows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      {
        id: "result-short-1",
        kind: "result",
        success: true,
        cancelled: false,
        result: "Done",
        durationMs: 1000,
        timestamp: new Date().toISOString(),
      },
      createToolMessage("tool-2"),
    ], {
      isLoading: true,
      latestToolIds: { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null },
    })

    expect(rows).toHaveLength(1)
    expect(rows[0]?.kind).toBe("tool-group")
    if (rows[0]?.kind !== "tool-group") throw new Error("unexpected row kind")
    expect(rows[0].messages.map((message) => message.id)).toEqual(["tool-1", "tool-2"])
  })

  test("does not group collapsible tools across visible transcript rows", () => {
    const rows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      {
        id: "assistant-1",
        kind: "assistant_text",
        text: "Visible text",
        timestamp: new Date().toISOString(),
      },
      createToolMessage("tool-2"),
    ], {
      isLoading: true,
      latestToolIds: { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null },
    })

    expect(rows).toHaveLength(3)
    expect(rows[0]?.kind).toBe("single")
    expect(rows[1]?.kind).toBe("single")
    expect(rows[2]?.kind).toBe("single")
  })

  test("renders grouped tools as expanded across rerenders while streaming when controlled", () => {
    const initialHtml = renderToStaticMarkup(
      <CollapsedToolGroup
        messages={[
          createToolMessage("tool-1"),
          createToolMessage("tool-2"),
        ]}
        isLoading
        expanded
        onExpandedChange={() => undefined}
      />
    )

    const updatedHtml = renderToStaticMarkup(
      <CollapsedToolGroup
        messages={[
          createToolMessage("tool-1"),
          createToolMessage("tool-2"),
          createToolMessage("tool-3"),
        ]}
        isLoading
        expanded
        onExpandedChange={() => undefined}
      />
    )

    expect(initialHtml).toContain("Run tool-1")
    expect(initialHtml).toContain("Run tool-2")
    expect(updatedHtml).toContain("Run tool-1")
    expect(updatedHtml).toContain("Run tool-2")
    expect(updatedHtml).toContain("Run tool-3")
  })

  test("reuses unchanged single row objects across streaming updates", () => {
    const latestToolIds = { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null }
    const previousRows = buildResolvedTranscriptRows([
      {
        id: "user-1",
        kind: "user_prompt",
        content: "Hello",
        timestamp: new Date().toISOString(),
      },
      {
        id: "assistant-1",
        kind: "assistant_text",
        text: "Response",
        timestamp: new Date().toISOString(),
      },
    ], {
      isLoading: true,
      latestToolIds,
    })
    const previousState: StableResolvedTranscriptRowsState = {
      byId: new Map(previousRows.map((row) => [row.id, row])),
      result: previousRows,
    }
    const nextRows = buildResolvedTranscriptRows([
      {
        id: "user-1",
        kind: "user_prompt",
        content: "Hello",
        timestamp: new Date().toISOString(),
      },
      {
        id: "assistant-1",
        kind: "assistant_text",
        text: "Response",
        timestamp: new Date().toISOString(),
      },
      createToolMessage("tool-1"),
    ], {
      isLoading: true,
      latestToolIds,
    })

    const stableState = computeStableResolvedTranscriptRows(nextRows, previousState)

    expect(stableState.result[0]).toBe(previousRows[0])
  })

  test("replaces a user row when attachment content changes", () => {
    const latestToolIds = { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null }
    const previousRows = buildResolvedTranscriptRows([
      {
        id: "user-attachment",
        kind: "user_prompt",
        content: "Check this",
        attachments: [{
          id: "file-1",
          kind: "file",
          displayName: "spec-a.pdf",
          absolutePath: "/tmp/spec-a.pdf",
          relativePath: "./spec-a.pdf",
          contentUrl: "/files/spec-a.pdf",
          mimeType: "application/pdf",
          size: 10,
        }],
        timestamp: new Date().toISOString(),
      },
    ], {
      isLoading: false,
      latestToolIds,
    })
    const previousState: StableResolvedTranscriptRowsState = {
      byId: new Map(previousRows.map((row) => [row.id, row])),
      result: previousRows,
    }
    const nextRows = buildResolvedTranscriptRows([
      {
        id: "user-attachment",
        kind: "user_prompt",
        content: "Check this",
        attachments: [{
          id: "file-1",
          kind: "file",
          displayName: "spec-b.pdf",
          absolutePath: "/tmp/spec-b.pdf",
          relativePath: "./spec-b.pdf",
          contentUrl: "/files/spec-b.pdf",
          mimeType: "application/pdf",
          size: 10,
        }],
        timestamp: new Date().toISOString(),
      },
    ], {
      isLoading: false,
      latestToolIds,
    })

    const stableState = computeStableResolvedTranscriptRows(nextRows, previousState)

    expect(stableState.result[0]).not.toBe(previousRows[0])
  })

  test("reuses unchanged tool-group rows across grouped run growth elsewhere", () => {
    const latestToolIds = { AskUserQuestion: null, ExitPlanMode: null, TodoWrite: null }
    const previousRows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      createToolMessage("tool-2"),
      {
        id: "assistant-1",
        kind: "assistant_text",
        text: "Done",
        timestamp: new Date().toISOString(),
      },
    ], {
      isLoading: true,
      latestToolIds,
    })
    const previousState: StableResolvedTranscriptRowsState = {
      byId: new Map(previousRows.map((row) => [row.id, row])),
      result: previousRows,
    }
    const nextRows = buildResolvedTranscriptRows([
      createToolMessage("tool-1"),
      createToolMessage("tool-2"),
      {
        id: "assistant-1",
        kind: "assistant_text",
        text: "Done",
        timestamp: new Date().toISOString(),
      },
      createToolMessage("tool-3"),
    ], {
      isLoading: true,
      latestToolIds,
    })

    const stableState = computeStableResolvedTranscriptRows(nextRows, previousState)

    expect(stableState.result[0]).toBe(previousRows[0])
  })
})
