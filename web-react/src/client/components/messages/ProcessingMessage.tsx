import { Loader2, X } from "lucide-react"
import { MetaRow, MetaContent } from "./shared"
import { AnimatedShinyText } from "../ui/animated-shiny-text"
import { useI18n } from "../../i18n/context"
import type { AgentProvider } from "../../../shared/types"

interface ProcessingMessageProps {
  status?: string
  provider?: AgentProvider | null
}

export function ProcessingMessage({ status, provider }: ProcessingMessageProps) {
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
    running_mcp_tool: "Running MCP tool",
    applying_changes: "Applying changes",
    writing_response: "Writing response",
  }
  const label = (status ? statusLabels[status] : undefined) || t.messages.processing
  const apiLabel = status === "connecting" || status === "acquiring_sandbox" || status === "initializing"
    ? "Abolqasem API"
    : provider === "opencode"
      ? "OpenCode"
      : provider === "claude"
        ? "Claude"
        : "Codex app-server"
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
          <span className="inline-flex items-center gap-1.5"><span className="text-[10px] uppercase tracking-wide text-muted-foreground/70">{apiLabel}</span><span>·</span><span>{label}</span></span>
        </AnimatedShinyText>
      </MetaContent>
    </MetaRow>
  )
}
