import type { HydratedTranscriptMessage } from "../../../shared/types"
import { buildResolvedTranscriptRows, type ResolvedTranscriptRow } from "../AbolqasemTranscript"

interface SearchMatchTarget {
  message_id?: string
  entry_id?: string
}

export interface TranscriptRowTarget {
  message: HydratedTranscriptMessage
  row: ResolvedTranscriptRow
  rowIndex: number
}

function messageMatchesTargetIds(message: HydratedTranscriptMessage, targets: Set<string>) {
  return targets.has(message.id) || (message.messageId ? targets.has(message.messageId) : false)
}

export function findLoadedSearchMessage(messages: HydratedTranscriptMessage[], match: SearchMatchTarget) {
  const targets = new Set([match.entry_id, match.message_id].filter((value): value is string => Boolean(value)))
  if (targets.size === 0) return null
  return messages.find((message) => messageMatchesTargetIds(message, targets)) ?? null
}

export function findLoadedTranscriptMessageById(messages: HydratedTranscriptMessage[], id: string) {
  const target = id.trim()
  if (!target) return null
  return messages.find((message) => (
    message.id === target || message.messageId === target
  )) ?? null
}

export function findPreviousUserPromptMessage(
  messages: HydratedTranscriptMessage[],
  beforeMessageId?: string | null,
) {
  let startIndex = messages.length - 1

  if (beforeMessageId) {
    const beforeIndex = messages.findIndex((message) => (
      message.id === beforeMessageId || message.messageId === beforeMessageId
    ))
    if (beforeIndex >= 0) {
      startIndex = beforeIndex - 1
    }
  }

  for (let index = startIndex; index >= 0; index -= 1) {
    const message = messages[index]
    if (message?.kind === "user_prompt" && !message.hidden && message.content.trim().length > 0) {
      return message
    }
  }

  return null
}

export function findTranscriptRowTarget(
  messages: HydratedTranscriptMessage[],
  latestToolIds: Record<string, string | null>,
  targetMessage: HydratedTranscriptMessage,
): TranscriptRowTarget | null {
  const targetIds = new Set([
    targetMessage.id,
    targetMessage.messageId,
  ].filter((value): value is string => Boolean(value)))
  const rows = buildResolvedTranscriptRows(messages, {
    isLoading: false,
    latestToolIds,
  })

  for (let rowIndex = 0; rowIndex < rows.length; rowIndex += 1) {
    const row = rows[rowIndex]
    if (!row) continue

    if (row.kind === "single" && messageMatchesTargetIds(row.message, targetIds)) {
      return { message: targetMessage, row, rowIndex }
    }

    if (row.kind === "tool-group" && row.messages.some((message) => messageMatchesTargetIds(message, targetIds))) {
      return { message: targetMessage, row, rowIndex }
    }
  }

  return null
}
