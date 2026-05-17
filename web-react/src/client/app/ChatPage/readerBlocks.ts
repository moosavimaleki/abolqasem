import type { HydratedTranscriptMessage } from "../../../shared/types"

type AssistantTextMessage = Extract<HydratedTranscriptMessage, { kind: "assistant_text" }>

export interface AssistantReaderDocument {
  title: string
  content: string
  anchorMessageId: string
  messageIds: string[]
}

const ASSISTANT_READER_DIVIDER = "\n\n---\n\n"

function isAssistantTextMessage(message: HydratedTranscriptMessage): message is AssistantTextMessage {
  return message.kind === "assistant_text"
}

function firstReadableLine(text: string) {
  return text
    .split(/\n+/)
    .map((line) => line.trim())
    .find(Boolean)
}

export function buildAssistantReaderDocument(
  messages: HydratedTranscriptMessage[],
  anchorMessageId: string | null,
): AssistantReaderDocument | null {
  if (!anchorMessageId) return null

  const anchorIndex = messages.findIndex((message) => (
    message.id === anchorMessageId
    && isAssistantTextMessage(message)
    && !message.hidden
  ))
  if (anchorIndex < 0) return null

  let startIndex = anchorIndex
  while (startIndex > 0 && messages[startIndex - 1]?.kind !== "user_prompt") {
    startIndex -= 1
  }

  let endIndex = anchorIndex + 1
  while (endIndex < messages.length && messages[endIndex]?.kind !== "user_prompt") {
    endIndex += 1
  }

  const assistantMessages = messages
    .slice(startIndex, endIndex)
    .filter(isAssistantTextMessage)
    .filter((message) => !message.hidden && message.text.trim())

  if (assistantMessages.length === 0) return null

  const firstTitle = assistantMessages
    .map((message) => firstReadableLine(message.text))
    .find(Boolean)

  return {
    title: firstTitle?.slice(0, 96) || "Reader",
    content: assistantMessages.map((message) => message.text.trim()).join(ASSISTANT_READER_DIVIDER),
    anchorMessageId,
    messageIds: assistantMessages.map((message) => message.id),
  }
}
