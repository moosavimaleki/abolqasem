import type { ReactNode } from "react"
import type { AgentProvider } from "../../../../shared/types"
import { Archive, Code, Copy, EyeOff, FolderOpen, Pencil, Split, Trash2 } from "lucide-react"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "../../ui/context-menu"
import { useI18n } from "../../../i18n/context"

export function ProjectSectionMenu({
  editorLabel,
  onRename,
  onCopyPath,
  onShowArchived,
  onOpenInFinder,
  onOpenInEditor,
  onHide,
  children,
}: {
  editorLabel: string
  onRename: () => void
  onCopyPath: () => void
  onShowArchived: () => void
  onOpenInFinder: () => void
  onOpenInEditor: () => void
  onHide: () => void
  children: ReactNode
}) {
  const { t } = useI18n()
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        {children}
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem
          onSelect={(event) => {
            event.preventDefault()
            onRename()
          }}
        >
          <Pencil className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.rename}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.stopPropagation()
            onCopyPath()
          }}
        >
          <Copy className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.copyPath}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.stopPropagation()
            onShowArchived()
          }}
        >
          <Archive className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.sidebar.archivedChats}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.stopPropagation()
            onOpenInFinder()
          }}
        >
          <FolderOpen className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.openExternal.openIn("Finder")}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.stopPropagation()
            onOpenInEditor()
          }}
        >
          <Code className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.openExternal.openIn(editorLabel)}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.stopPropagation()
            onHide()
          }}
        >
          <EyeOff className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.hide}</span>
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}

export function ChatRowMenu({
  canFork,
  onRename,
  onOpenInFinder,
  onFork,
  onConvert,
  onArchive,
  onDelete,
  children,
}: {
  canFork?: boolean
  onRename: () => void
  onOpenInFinder: () => void
  onFork: () => void
  onConvert: (provider: AgentProvider) => void
  onArchive: () => void
  onDelete: () => void
  children: ReactNode
}) {
  const { t } = useI18n()
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        {children}
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem
          onSelect={(event) => {
            event.preventDefault()
            onRename()
          }}
        >
          <Pencil className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.rename}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.preventDefault()
            onOpenInFinder()
          }}
        >
          <FolderOpen className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.openExternal.openIn("Finder")}</span>
        </ContextMenuItem>
        <ContextMenuItem
          disabled={!canFork}
          onSelect={(event) => {
            event.preventDefault()
            if (!canFork) return
            onFork()
          }}
        >
          <Split className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.fork}</span>
        </ContextMenuItem>
        {(["claude", "codex"] as const).map((provider) => (
          <ContextMenuItem
            key={provider}
            disabled={!canFork}
            onSelect={(event) => {
              event.preventDefault()
              if (!canFork) return
              onConvert(provider)
            }}
          >
            <Split className="h-3.5 w-3.5" />
            <span className="text-xs font-medium">{t.sidebar.forkToProvider(provider === "claude" ? "Claude" : "Codex")}</span>
          </ContextMenuItem>
        ))}
        <ContextMenuItem
          onSelect={(event) => {
            event.preventDefault()
            onArchive()
          }}
        >
          <Archive className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.archive}</span>
        </ContextMenuItem>
        <ContextMenuItem
          onSelect={(event) => {
            event.preventDefault()
            onDelete()
          }}
          className="text-destructive dark:text-red-400 hover:bg-destructive/10 focus:bg-destructive/10 dark:hover:bg-red-500/20 dark:focus:bg-red-500/20"
        >
          <Trash2 className="h-3.5 w-3.5" />
          <span className="text-xs font-medium">{t.common.delete}</span>
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}
