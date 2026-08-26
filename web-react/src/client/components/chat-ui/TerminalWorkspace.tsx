import { Fragment, memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { Eraser, Play, Plus, ScrollText, X } from "lucide-react"
import type { ProjectRunnableScript } from "../../../shared/protocol"
import type { SocketStatus, AbolqasemSocket } from "../../app/socket"
import { Button } from "../ui/button"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "../ui/resizable"
import { HotkeyTooltip, HotkeyTooltipContent, HotkeyTooltipTrigger } from "../ui/tooltip"
import type { ProjectTerminalLayout } from "../../stores/terminalLayoutStore"
import { TerminalPane } from "./TerminalPane"
import { getMinimumTerminalWidth } from "./TerminalWorkspaceLayout"
import { useI18n } from "../../i18n/context"

interface Props {
  projectId: string
  layout: ProjectTerminalLayout
  socket: AbolqasemSocket
  connectionStatus: SocketStatus
  scrollback: number
  minColumnWidth: number
  focusRequestVersion?: number
  pendingCommandsByTerminalId?: Record<string, string>
  splitTerminalShortcut?: string[]
  onAddTerminal: (projectId: string, afterTerminalId?: string) => void
  onRemoveTerminal: (projectId: string, terminalId: string) => void
  onTerminalLayout: (projectId: string, sizes: number[]) => void
  onTerminalCommandSent?: () => void
  onInitialTerminalCommandSent?: (terminalId: string) => void
}

interface TerminalWorkspacePaneProps {
  projectId: string
  terminalId: string
  size: number
  isLast: boolean
  minTerminalWidth: number
  path: string | null
  socket: AbolqasemSocket
  scrollback: number
  connectionStatus: SocketStatus
  clearVersion: number
  focusRequestVersion: number
  initialCommand?: string
  splitTerminalShortcut?: string[]
  onAddTerminal: (projectId: string, afterTerminalId?: string) => void
  onRemoveTerminal: (projectId: string, terminalId: string) => void
  onClearTerminal: (terminalId: string) => void
  onPathChange: (terminalId: string, path: string | null) => void
  onCommandSent?: () => void
  onInitialCommandSent?: (terminalId: string) => void
  setPaneElement: (terminalId: string, element: HTMLDivElement | null) => void
  scripts: ProjectRunnableScript[]
  showScripts: boolean
  scriptCommand: { id: number; command: string } | null
  onRunScript: (command: string) => void
}

const TerminalWorkspacePane = memo(function TerminalWorkspacePane({
  projectId,
  terminalId,
  size,
  isLast,
  minTerminalWidth,
  path,
  socket,
  scrollback,
  connectionStatus,
  clearVersion,
  focusRequestVersion,
  initialCommand,
  splitTerminalShortcut,
  onAddTerminal,
  onRemoveTerminal,
  onClearTerminal,
  onPathChange,
  onCommandSent,
  onInitialCommandSent,
  setPaneElement,
  scripts,
  showScripts,
  scriptCommand,
  onRunScript,
}: TerminalWorkspacePaneProps) {
  const { t, direction } = useI18n()
  const handleSetPaneElement = useCallback((element: HTMLDivElement | null) => {
    setPaneElement(terminalId, element)
  }, [setPaneElement, terminalId])

  const handleClearTerminal = useCallback(() => {
    onClearTerminal(terminalId)
    onPathChange(terminalId, null)
  }, [onClearTerminal, onPathChange, terminalId])

  const handleAddTerminal = useCallback(() => {
    onAddTerminal(projectId, terminalId)
  }, [onAddTerminal, projectId, terminalId])

  const handleRemoveTerminal = useCallback(() => {
    onRemoveTerminal(projectId, terminalId)
  }, [onRemoveTerminal, projectId, terminalId])

  const handlePathChange = useCallback((nextPath: string | null) => {
    onPathChange(terminalId, nextPath)
  }, [onPathChange, terminalId])

  return (
    <Fragment>
      <ResizablePanel
        id={terminalId}
        defaultSize={`${size}%`}
        minSize={`${minTerminalWidth}px`}
        className="min-h-0 overflow-hidden"
        style={{ minWidth: minTerminalWidth, maxWidth: "100%" }}
      >
        <div
          ref={handleSetPaneElement}
          className="flex h-full min-h-0 min-w-0 flex-col border-r border-border bg-transparent last:border-r-0"
          style={{ minWidth: minTerminalWidth, maxWidth: "100%" }}
        >
          <div className="flex items-center gap-2 px-3 pr-2 pt-2 pb-1">
            <div className="min-w-0 flex-1 text-left">
              <div className="flex min-w-0 items-center gap-2">
                <div className="shrink-0 text-sm font-medium">{t.terminal.terminal}</div>
                <div className="min-w-0 truncate text-xs text-muted-foreground">
                  {path ?? ""}
                </div>
              </div>
            </div>
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t.terminal.clear}
                onClick={handleClearTerminal}
              >
                <Eraser className="size-3.5" />
              </Button>
              <HotkeyTooltip>
                <HotkeyTooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t.terminal.addRight}
                    onClick={handleAddTerminal}
                  >
                    <Plus className="size-3.5" />
                  </Button>
                </HotkeyTooltipTrigger>
                <HotkeyTooltipContent side="bottom" shortcut={splitTerminalShortcut} />
              </HotkeyTooltip>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t.terminal.archive}
                onClick={handleRemoveTerminal}
              >
                <X className="size-3.5" />
              </Button>
            </div>
          </div>

          {showScripts && scripts.length > 0 ? (
            <div className="flex min-w-0 items-center gap-2 overflow-x-auto px-3 pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" dir="ltr">
              <span className="flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground" dir={direction}>
                <ScrollText className="size-3" />
                {t.terminal.projectScripts}
              </span>
              {scripts.map((script) => (
                <Button
                  key={script.id}
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => onRunScript(script.command)}
                  title={script.command}
                  className="h-6 shrink-0 gap-1 rounded-md border-border/70 px-2 font-mono text-[11px] text-foreground hover:bg-muted/50"
                  aria-label={`${t.terminal.runScript}: ${script.label}`}
                >
                  <Play className="size-3" />
                  <span>{script.label}</span>
                </Button>
              ))}
            </div>
          ) : null}

          <TerminalPane
            projectId={projectId}
            terminalId={terminalId}
            socket={socket}
            scrollback={scrollback}
            connectionStatus={connectionStatus}
            clearVersion={clearVersion}
            focusRequestVersion={focusRequestVersion}
            initialCommand={initialCommand}
            scriptCommand={scriptCommand}
            onCommandSent={onCommandSent}
            onInitialCommandSent={onInitialCommandSent}
            onPathChange={handlePathChange}
          />
        </div>
      </ResizablePanel>
      {!isLast ? <ResizableHandle withHandle orientation="horizontal" /> : null}
    </Fragment>
  )
})

function TerminalWorkspaceImpl({
  projectId,
  layout,
  socket,
  connectionStatus,
  scrollback,
  minColumnWidth,
  focusRequestVersion = 0,
  pendingCommandsByTerminalId,
  splitTerminalShortcut,
  onAddTerminal,
  onRemoveTerminal,
  onTerminalLayout,
  onTerminalCommandSent,
  onInitialTerminalCommandSent,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const paneRefs = useRef<Record<string, HTMLDivElement | null>>({})
  const previousTerminalIdsRef = useRef<string[]>([])
  const [viewportWidth, setViewportWidth] = useState(0)
  const [pathsByTerminalId, setPathsByTerminalId] = useState<Record<string, string | null>>({})
  const [clearVersionsByTerminalId, setClearVersionsByTerminalId] = useState<Record<string, number>>({})
  const [scripts, setScripts] = useState<ProjectRunnableScript[]>([])
  const [scriptCommand, setScriptCommand] = useState<{ id: number; command: string } | null>(null)

  useEffect(() => {
    let cancelled = false
    setScripts([])
    void socket.command<ProjectRunnableScript[]>({ type: "project.readRunnableScripts", projectId })
      .then((nextScripts) => {
        if (!cancelled) setScripts(nextScripts)
      })
      .catch(() => {
        if (!cancelled) setScripts([])
      })
    return () => {
      cancelled = true
    }
  }, [projectId, socket])

  const handleRunScript = useCallback((command: string) => {
    setScriptCommand((current) => ({ id: (current?.id ?? 0) + 1, command }))
  }, [])

  useLayoutEffect(() => {
    const element = containerRef.current
    if (!element) return

    const updateWidth = () => {
      setViewportWidth(element.getBoundingClientRect().width)
    }

    const observer = new ResizeObserver(updateWidth)
    observer.observe(element)
    updateWidth()

    return () => observer.disconnect()
  }, [])

  const paneCount = layout.terminals.length
  const minTerminalWidth = getMinimumTerminalWidth(minColumnWidth)
  const effectiveMinTerminalWidth = viewportWidth > 0 ? Math.min(minTerminalWidth, viewportWidth) : minTerminalWidth
  const requiredWidth = Math.max(1, paneCount) * effectiveMinTerminalWidth
  const innerWidth = Math.max(viewportWidth, requiredWidth)
  const panelGroupKey = useMemo(
    () => layout.terminals.map((terminal) => terminal.id).join(":"),
    [layout.terminals]
  )
  const handleSetPaneElement = useCallback((terminalId: string, element: HTMLDivElement | null) => {
    paneRefs.current[terminalId] = element
  }, [])

  const handlePathChange = useCallback((terminalId: string, path: string | null) => {
    setPathsByTerminalId((current) => {
      if (current[terminalId] === path) return current
      return {
        ...current,
        [terminalId]: path,
      }
    })
  }, [])

  const handleClearTerminal = useCallback((terminalId: string) => {
    setClearVersionsByTerminalId((current) => ({
      ...current,
      [terminalId]: (current[terminalId] ?? 0) + 1,
    }))
  }, [])

  const handleLayoutChanged = useCallback((nextLayout: Record<string, number>) => {
    onTerminalLayout(
      projectId,
      layout.terminals.map((terminal) => nextLayout[terminal.id] ?? terminal.size),
    )
  }, [layout.terminals, onTerminalLayout, projectId])

  useLayoutEffect(() => {
    const previousIds = previousTerminalIdsRef.current
    const currentIds = layout.terminals.map((terminal) => terminal.id)
    const addedTerminalId = currentIds.find((id) => !previousIds.includes(id))

    previousTerminalIdsRef.current = currentIds

    if (!addedTerminalId || previousIds.length === 0) {
      return
    }

    const element = paneRefs.current[addedTerminalId]
    if (!element) {
      return
    }

    element.scrollIntoView({
      behavior: "smooth",
      block: "nearest",
      inline: "nearest",
    })
  }, [layout.terminals])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div ref={containerRef} className="min-h-0 flex-1 overflow-x-auto overflow-y-hidden">
        <div className="h-full min-h-0" style={{ width: innerWidth || "100%" }}>
          <ResizablePanelGroup
            key={panelGroupKey}
            orientation="horizontal"
            className="h-full min-h-0"
            onLayoutChanged={handleLayoutChanged}
          >
            {layout.terminals.map((terminalPane, index) => (
              <TerminalWorkspacePane
                key={terminalPane.id}
                projectId={projectId}
                terminalId={terminalPane.id}
                size={terminalPane.size}
                isLast={index === layout.terminals.length - 1}
                minTerminalWidth={effectiveMinTerminalWidth}
                path={pathsByTerminalId[terminalPane.id] ?? null}
                socket={socket}
                scrollback={scrollback}
                connectionStatus={connectionStatus}
                clearVersion={clearVersionsByTerminalId[terminalPane.id] ?? 0}
                focusRequestVersion={index === 0 ? focusRequestVersion : 0}
                initialCommand={pendingCommandsByTerminalId?.[terminalPane.id]}
                scripts={scripts}
                showScripts={layout.terminals.length === 1}
                scriptCommand={scriptCommand}
                splitTerminalShortcut={splitTerminalShortcut}
                onAddTerminal={onAddTerminal}
                onRemoveTerminal={onRemoveTerminal}
                onClearTerminal={handleClearTerminal}
                onPathChange={handlePathChange}
                onCommandSent={onTerminalCommandSent}
                onInitialCommandSent={onInitialTerminalCommandSent}
                onRunScript={handleRunScript}
                setPaneElement={handleSetPaneElement}
              />
            ))}
          </ResizablePanelGroup>
        </div>
      </div>
    </div>
  )
}

export const TerminalWorkspace = memo(TerminalWorkspaceImpl)
