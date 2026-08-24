import { memo, type RefObject } from "react"
import type { AgentProvider, CodexLockStatus, ModelOptions } from "../../../shared/types"
import { ChatInput, type ChatInputHandle } from "../../components/chat-ui/ChatInput"
import type { ContextWindowSnapshot } from "../../lib/contextWindow"
import type { AbolqasemState } from "../useAbolqasemState"

interface ChatInputDockProps {
  inputRef: RefObject<HTMLDivElement | null>
  onLayoutChange: () => void
  chatInputRef: RefObject<ChatInputHandle | null>
  chatInputElementRef: RefObject<HTMLTextAreaElement | null>
  activeChatId: string | null
  previousPrompt: string | null
  onJumpToPreviousUserPrompt?: () => void | Promise<void>
  hasSelectedProject: boolean
  runtimeStatus: string | null
  canCancel: boolean
  projectId: string | null
  activeProvider: AgentProvider | null
  availableProviders: AbolqasemState["availableProviders"]
  contextWindowSnapshot: ContextWindowSnapshot | null
  readOnly?: boolean
  codexLock?: CodexLockStatus | null
  lockBusy?: boolean
  onTakeOverSession?: () => void
  onReleaseSession?: () => void
  onRefreshSessionLock?: () => void
  onSubmit: AbolqasemState["handleSend"]
  onRuntimePreferenceChange?: (preference: { provider: AgentProvider; model: string; modelOptions: ModelOptions }) => Promise<void>
  onCancel: () => void
}

export const ChatInputDock = memo(function ChatInputDock({
  inputRef,
  onLayoutChange,
  chatInputRef,
  chatInputElementRef,
  activeChatId,
  previousPrompt,
  onJumpToPreviousUserPrompt,
  hasSelectedProject,
  runtimeStatus,
  canCancel,
  projectId,
  activeProvider,
  availableProviders,
  contextWindowSnapshot,
  readOnly = false,
  codexLock = null,
  lockBusy = false,
  onTakeOverSession,
  onReleaseSession,
  onRefreshSessionLock,
  onSubmit,
  onRuntimePreferenceChange,
  onCancel,
}: ChatInputDockProps) {
  return (
    <div className="absolute bottom-0 left-0 right-0 z-20 pointer-events-none">
      <div className="bg-gradient-to-t from-background via-background pointer-events-auto" ref={inputRef}>
        <ChatInput
          ref={chatInputRef}
          inputElementRef={chatInputElementRef}
          onLayoutChange={onLayoutChange}
          key={activeChatId ?? "new-chat"}
          onSubmit={onSubmit}
          onRuntimePreferenceChange={onRuntimePreferenceChange}
          onCancel={onCancel}
          disabled={!hasSelectedProject}
          canCancel={canCancel}
          chatId={activeChatId}
          projectId={projectId}
          activeProvider={activeProvider}
          availableProviders={availableProviders}
          showPreferenceControls
          contextWindowSnapshot={contextWindowSnapshot}
          readOnly={readOnly}
          codexLock={codexLock}
          lockBusy={lockBusy}
          onTakeOverSession={onTakeOverSession}
          onReleaseSession={onReleaseSession}
          onRefreshSessionLock={onRefreshSessionLock}
          previousPrompt={previousPrompt}
          onJumpToPreviousUserPrompt={onJumpToPreviousUserPrompt}
        />
      </div>
    </div>
  )
})
