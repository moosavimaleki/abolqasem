import { Loader2, X } from "lucide-react"
import { MetaRow, MetaContent } from "./shared"
import { AnimatedShinyText } from "../ui/animated-shiny-text"
import { useI18n } from "../../i18n/context"

interface ProcessingMessageProps {
  status?: string
}

export function ProcessingMessage({ status }: ProcessingMessageProps) {
  const { t } = useI18n()
  const statusLabels: Record<string, string> = {
    connecting: t.messages.status.connecting,
    acquiring_sandbox: t.messages.status.acquiring_sandbox,
    initializing: t.messages.status.initializing,
    starting: t.messages.status.starting,
    running: t.messages.status.running,
    waiting_for_user: t.messages.status.waitingForUser,
    failed: t.messages.status.failed,
    thinking: "Thinking",
    running_command: "Running command",
    applying_changes: "Applying changes",
    writing_response: "Writing response",
  }
  const label = (status ? statusLabels[status] : undefined) || t.messages.processing
  const isFailed = status === "failed"

  return (
    <MetaRow className="ml-[1px]">
      <MetaContent>
        {isFailed ? (
          <X className="size-4.5 text-red-500" />
        ) : (
          <Loader2 className="size-4.5 animate-spin text-muted-icon" />
        )}
        <AnimatedShinyText className="ml-[1px] text-sm" shimmerWidth={44}>
          {label}
        </AnimatedShinyText>
      </MetaContent>
    </MetaRow>
  )
}
