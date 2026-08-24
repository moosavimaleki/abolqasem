import type { HydratedTranscriptMessage, TurnActivityEntry } from "../../../shared/types"

type TurnActivity = TurnActivityEntry["activity"]

export function getProcessingStatus(
  messages: HydratedTranscriptMessage[],
  runtimeStatus?: string,
): TurnActivity | "waiting_for_user" | "failed" | undefined {
  if (runtimeStatus === "waiting_for_user" || runtimeStatus === "failed") {
    return runtimeStatus
  }
  if (runtimeStatus !== "starting" && runtimeStatus !== "running") {
    return undefined
  }

  let turnBoundary = -1
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].kind === "user_prompt" || messages[index].kind === "result") {
      turnBoundary = index
      break
    }
  }
  for (let index = messages.length - 1; index > turnBoundary; index -= 1) {
    const message = messages[index]
    if (message.kind === "turn_activity") {
      return message.activity
    }
  }
  return "thinking"
}
