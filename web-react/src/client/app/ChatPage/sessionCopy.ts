import type { HydratedTranscriptMessage } from "../../../shared/types"

export interface SessionCopyLabels {
  user: string
  assistant: string
}

interface SessionCopyTurn {
  user: string
  assistantMessages: string[]
}

function visibleText(value: string, hidden: boolean | undefined) {
  const text = value.trim()
  return hidden || !text ? "" : text
}

export function collectSessionCopyTurns(messages: HydratedTranscriptMessage[]): SessionCopyTurn[] {
  const turns: SessionCopyTurn[] = []
  let currentTurn: SessionCopyTurn | null = null

  for (const message of messages) {
    if (message.kind === "user_prompt") {
      const content = visibleText(message.content, message.hidden)
      if (!content) continue
      currentTurn = { user: content, assistantMessages: [] }
      turns.push(currentTurn)
      continue
    }

    if (message.kind !== "assistant_text" || !currentTurn) continue
    const text = visibleText(message.text, message.hidden)
    if (text) currentTurn.assistantMessages.push(text)
  }

  return turns
}

export function buildSessionCopyText(
  messages: HydratedTranscriptMessage[],
  requestedTurnCount: number,
  labels: SessionCopyLabels,
) {
  const turnCount = Math.max(1, Math.floor(requestedTurnCount))
  const turns = collectSessionCopyTurns(messages).slice(-turnCount)

  return turns.map((turn) => {
    const parts = [`${labels.user}:\n${turn.user}`]
    if (turn.assistantMessages.length > 0) {
      parts.push(`${labels.assistant}:\n${turn.assistantMessages.join("\n\n")}`)
    }
    return parts.join("\n\n")
  }).join("\n\n---\n\n")
}
