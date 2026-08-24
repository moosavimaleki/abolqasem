import { useMemo, useState } from "react"
import { Check, ChevronDown, ChevronRight, Circle, FileCode2, Loader2, TerminalSquare, X } from "lucide-react"
import type { CodexFileUpdateChange, HydratedTranscriptMessage } from "../../../shared/types"
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

export function CodexCommandMessage({ message }: { message: CommandMessage }) {
  const [expanded, setExpanded] = useState(Boolean(message.aggregatedOutput && message.status !== "completed"))
  const title = message.command.replace(/^\/usr\/bin\/(?:zsh|bash)\s+-lc\s+['"]?/, "").replace(/['"]$/, "")
  return (
    <div className="my-1 w-full overflow-hidden rounded-lg border border-white/10 bg-zinc-900/70 font-mono text-xs" dir="ltr">
      <button type="button" onClick={() => setExpanded((value) => !value)} className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-white/5">
        {statusIcon(message.status)}
        <span className="shrink-0 text-[11px] text-muted-foreground">{message.status === "inProgress" ? "Running" : message.status}</span>
        <span className="min-w-0 flex-1 truncate text-zinc-200">{title}</span>
        {message.exitCode !== undefined && message.exitCode !== null ? <span className={cn("shrink-0", message.exitCode === 0 ? "text-emerald-400" : "text-red-400")}>exit {message.exitCode}</span> : null}
        {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
      </button>
      {expanded ? (
        <div className="border-t border-white/10">
          {message.cwd ? <div className="border-b border-white/10 px-3 py-1.5 text-zinc-500">cwd: {message.cwd}</div> : null}
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap px-3 py-2 text-zinc-300">{message.aggregatedOutput || "No output yet"}</pre>
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
          return <button key={change.path} type="button" onClick={() => setSelected(change)} className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-white/5" dir="ltr">
            <span className="rounded-full bg-sky-950 px-2 py-0.5 text-[10px] uppercase text-sky-300">{change.kind}</span>
            <span className="min-w-0 flex-1 truncate font-mono text-sky-300">{change.path}</span>
            <span className="text-emerald-400">+{counts.additions}</span><span className="text-red-400">-{counts.deletions}</span>
          </button>
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
