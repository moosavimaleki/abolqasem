import type { FilePreviewResponse } from "../file-preview/FilePreviewPanel"
import type { ProjectFileEntry } from "./projectFilesData"

export interface ProjectFileActionHandlers {
  onOpenFile?: (path: string) => void
  onOpenInFinder?: (path: string) => void
  onCopyFilePath?: (path: string) => void
  onCopyRelativePath?: (path: string) => void
  onOpenFullPage?: (path: string) => void
}

export function createProjectFileActions(
  entry: ProjectFileEntry | null,
  preview: FilePreviewResponse | null,
  handlers: ProjectFileActionHandlers,
) {
  const path = entry?.path ?? ""
  const isFile = entry?.type === "file"
  const canOpenInEditor = Boolean(isFile && handlers.onOpenFile)
  const canRevealInFinder = Boolean(entry && handlers.onOpenInFinder)
  const canCopyAbsolutePath = Boolean(entry && handlers.onCopyFilePath)
  const canCopyRelativePath = Boolean(entry && handlers.onCopyRelativePath)
  const canOpenFullPage = Boolean(preview && handlers.onOpenFullPage)

  return {
    canOpenInEditor,
    canRevealInFinder,
    canCopyAbsolutePath,
    canCopyRelativePath,
    canOpenFullPage,
    openInEditor: () => {
      if (canOpenInEditor) handlers.onOpenFile?.(path)
    },
    revealInFinder: () => {
      if (canRevealInFinder) handlers.onOpenInFinder?.(path)
    },
    copyAbsolutePath: () => {
      if (canCopyAbsolutePath) handlers.onCopyFilePath?.(path)
    },
    copyRelativePath: () => {
      if (canCopyRelativePath) handlers.onCopyRelativePath?.(path)
    },
    openFullPage: () => {
      if (preview) handlers.onOpenFullPage?.(preview.path)
    },
  }
}
