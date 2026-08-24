import { useId, useMemo, useState } from "react"
import { Check, ChevronDown, ChevronRight, Circle, FileCode2, FolderOpen, Loader2, TerminalSquare, X } from "lucide-react"
import type { CodexFileUpdateChange, HydratedTranscriptMessage } from "../../../shared/types"
import { fileRouteHref } from "../file-preview/FilePreviewPanel"
import { formatBashCommandTitle } from "../../lib/formatters"
import { cn } from "../../lib/utils"
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog"

type CommandMessage = Extract<HydratedTranscriptMessage, { kind: "command_execution" }>
type FileChangeMessage = Extract<HydratedTranscriptMessage, { kind: "file_change" }>
type PlanMessage = Extract<HydratedTranscriptMessage, { kind: "turn_plan" }>

function statusIcon(status: CommandMessage["status"]) {
  if (status === "inProgress") return <Loader2 className="size-3.5 animate-spin text-sky-400" />
  if (status === "completed") return <Check className="size-3.5 text-emerald-400" />
  return <X className="size-3.5 text-red-400" />
}

function commandStatusLabel(message: CommandMessage) {
  if (message.status === "inProgress") return "Running"
  if (message.status === "failed") return message.exitCode == null ? "Failed" : `Exit ${message.exitCode}`
  if (message.status === "declined") return "Declined"
  return message.exitCode != null && message.exitCode !== 0 ? `Exit ${message.exitCode}` : "Completed"
}

export function CodexCommandMessage({ message }: { message: CommandMessage }) {
  const [expanded, setExpanded] = useState(false)
  const detailsId = useId()
  const title = formatBashCommandTitle(message.command) || "Command"
  return (
    <div className="my-1 w-full overflow-hidden rounded-lg border border-white/10 bg-zinc-900/70 font-mono text-xs" dir="ltr">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={() => setExpanded((value) => !value)}
        className="flex min-h-9 w-full cursor-pointer items-center gap-2 px-2.5 py-1.5 text-left transition-colors hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-500/70"
        title={title}
      >
        {expanded ? <ChevronDown className="size-3.5 shrink-0 text-zinc-400" /> : <ChevronRight className="size-3.5 shrink-0 text-zinc-400" />}
        {statusIcon(message.status)}
        <span className="min-w-0 flex-1 truncate text-zinc-200">{title}</span>
        <span className={cn("shrink-0 text-[11px] font-medium", message.status === "inProgress" ? "text-sky-400" : message.status === "completed" && (message.exitCode == null || message.exitCode === 0) ? "text-emerald-400" : "text-red-400")}>{commandStatusLabel(message)}</span>
      </button>
      {expanded ? (
        <div id={detailsId} className="border-t border-white/10">
          {message.cwd ? <div className="border-b border-white/10 px-3 py-1.5 text-zinc-500">cwd: {message.cwd}</div> : null}
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap px-3 py-2 text-zinc-300">{message.aggregatedOutput || "No output yet"}</pre>
        </div>
      ) : null}
    </div>
  )
}

export function CodexCommandGroup({
  messages,
  expanded,
  onExpandedChange,
}: {
  messages: CommandMessage[]
  expanded: boolean
  onExpandedChange: (next: boolean) => void
}) {
  const detailsId = useId()
  const latest = messages[messages.length - 1]
  const running = messages.some((message) => message.status === "inProgress")
  const failed = messages.find((message) => message.status === "failed" || (message.exitCode != null && message.exitCode !== 0))
  const statusMessage = running
    ? messages.find((message) => message.status === "inProgress") ?? latest
    : failed ?? latest
  const latestTitle = formatBashCommandTitle(latest?.command ?? "") || "Command"
  const label = `${messages.length} commands · latest: ${latestTitle}`

  return (
    <div className="my-1 w-full font-mono text-xs" dir="ltr" data-command-group="true">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={() => onExpandedChange(!expanded)}
        className="flex min-h-9 w-full cursor-pointer items-center gap-2 rounded-lg border border-dashed border-white/15 bg-zinc-800/80 px-2.5 py-1.5 text-left transition-colors hover:bg-zinc-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-500/70"
        title={label}
      >
        {expanded ? <ChevronDown className="size-3.5 shrink-0 text-zinc-400" /> : <ChevronRight className="size-3.5 shrink-0 text-zinc-400" />}
        {statusMessage ? statusIcon(statusMessage.status) : null}
        <span className="min-w-0 flex-1 truncate text-zinc-200">{label}</span>
        {statusMessage ? <span className={cn("shrink-0 text-[11px] font-medium", running ? "text-sky-400" : failed ? "text-red-400" : "text-emerald-400")}>{commandStatusLabel(statusMessage)}</span> : null}
      </button>
      {expanded ? (
        <div id={detailsId} className="mt-1.5 border-l border-white/10 pl-2">
          {messages.map((message) => <CodexCommandMessage key={message.id} message={message} />)}
        </div>
      ) : null}
    </div>
  )
}

function diffCounts(diff: string) {
  let additions = 0
  let deletions = 0
  for (const line of diff.split("\n")) {
    if (line.startsWith("+") && !line.startsWith("+++")) additions += 1
    if (line.startsWith("-") && !line.startsWith("---")) deletions += 1
  }
  return { additions, deletions }
}

function DiffBody({ change }: { change: CodexFileUpdateChange }) {
  return <pre className="min-h-0 flex-1 overflow-auto bg-[#07090c] p-4 font-mono text-xs leading-6" dir="ltr">{change.diff.split("\n").map((line, index) => (
    <div key={index} className={cn("min-w-max px-2", line.startsWith("+") && !line.startsWith("+++") && "bg-emerald-950/60 text-emerald-300", line.startsWith("-") && !line.startsWith("---") && "bg-red-950/60 text-red-300", line.startsWith("@@") && "bg-sky-950/60 text-sky-300")}>{line || " "}</div>
  ))}</pre>
}

export function CodexFileChangeMessage({ message }: { message: FileChangeMessage }) {
  const [expanded, setExpanded] = useState(true)
  const [selected, setSelected] = useState<CodexFileUpdateChange | null>(null)
  const totals = useMemo(() => message.changes.reduce((sum, change) => {
    const count = diffCounts(change.diff || "")
    return { additions: sum.additions + count.additions, deletions: sum.deletions + count.deletions }
  }, { additions: 0, deletions: 0 }), [message.changes])
  return (
    <>
      <div className="my-2 overflow-hidden rounded-xl border border-white/10 bg-zinc-950/60 text-xs">
        <button type="button" onClick={() => setExpanded((value) => !value)} className="flex w-full items-center gap-2 bg-zinc-800/80 px-3 py-2 text-left">
          <FileCode2 className="size-4 text-sky-400" />
          <span className="font-medium text-zinc-200">{message.changes.length} files changed</span>
          <span className="text-emerald-400">+{totals.additions}</span>
          <span className="text-red-400">-{totals.deletions}</span>
          <span className="ml-auto text-zinc-500">{message.status}</span>
          {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
        </button>
        {expanded ? <div className="divide-y divide-white/10">{message.changes.map((change) => {
          const counts = diffCounts(change.diff || "")
          const targetPath = change.movedToPath || change.path
          return <div key={change.path} className="flex items-stretch hover:bg-white/5" dir="ltr">
            <button type="button" onClick={() => setSelected(change)} className="flex min-w-0 flex-1 items-center gap-2 px-3 py-2 text-left">
              <span className="rounded-full bg-sky-950 px-2 py-0.5 text-[10px] uppercase text-sky-300">{change.kind}</span>
              <span className="min-w-0 flex-1 truncate font-mono text-sky-300">{change.path}{change.movedToPath ? ` → ${change.movedToPath}` : ""}</span>
              <span className="text-emerald-400">+{counts.additions}</span><span className="text-red-400">-{counts.deletions}</span>
            </button>
            <a href={fileRouteHref(targetPath)} target="_blank" rel="noreferrer" aria-label={`Open ${targetPath} in file manager`} title="Open in file manager" className="flex w-10 shrink-0 items-center justify-center border-s border-white/10 text-sky-300 transition-colors hover:bg-sky-950/60 hover:text-sky-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-sky-500">
              <FolderOpen className="size-4" />
            </a>
          </div>
        })}</div> : null}
      </div>
      <Dialog open={selected !== null} onOpenChange={(open) => { if (!open) setSelected(null) }}>
        <DialogContent size="lg" className="h-[85vh] max-w-[min(96vw,1200px)] overflow-hidden p-0">
          <div className="border-b border-border p-4 pe-12"><DialogTitle className="truncate font-mono text-sm" dir="ltr">{selected?.path}</DialogTitle></div>
          {selected ? <DiffBody change={selected} /> : null}
        </DialogContent>
      </Dialog>
    </>
  )
}

export function CodexPlanMessage({ message }: { message: PlanMessage }) {
  const updating = message.plan.some((step) => step.status === "inProgress")
  return <div className="my-3 overflow-hidden rounded-2xl border border-sky-700 bg-slate-950/60">
    <div className="flex items-center justify-between border-b border-sky-900/70 px-4 py-3">
      <span className="rounded-full bg-sky-700 px-2.5 py-0.5 text-[11px] text-white">{updating ? "Updating" : "Plan"}</span>
      <span className="font-medium text-sky-100">Plan</span>
    </div>
    {message.explanation ? <p className="px-4 pt-3 text-sm leading-7 text-slate-300">{message.explanation}</p> : null}
    <div className="space-y-2 p-4">{message.plan.map((step, index) => <div key={`${index}:${step.step}`} className={cn("flex items-center gap-3 rounded-xl border px-3 py-2.5 text-sm", step.status === "inProgress" ? "border-amber-700 bg-amber-950/30 text-amber-100" : "border-white/10 bg-white/5 text-slate-200")}>
      {step.status === "completed" ? <Check className="size-4 shrink-0 text-emerald-400" /> : step.status === "inProgress" ? <Loader2 className="size-4 shrink-0 animate-spin text-amber-400" /> : <Circle className="size-4 shrink-0 text-slate-500" />}
      <span className="flex-1">{step.step}</span>
    </div>)}</div>
  </div>
}

export function CodexActivityLabel({ activity }: { activity: Extract<HydratedTranscriptMessage, { kind: "turn_activity" }>["activity"] }) {
  const labels = { thinking: "Thinking", running_command: "Running command", applying_changes: "Applying changes", writing_response: "Writing response" }
  return <span className="inline-flex items-center gap-2"><TerminalSquare className="size-4" />{labels[activity]}</span>
}
