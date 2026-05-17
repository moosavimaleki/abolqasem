import { memo, type RefObject } from "react"
import type { AgentProvider } from "../../../shared/types"
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
  onSubmit: AbolqasemState["handleSend"]
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
  onSubmit,
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
          onCancel={onCancel}
          disabled={!hasSelectedProject}
          canCancel={canCancel}
          chatId={activeChatId}
          projectId={projectId}
          activeProvider={activeProvider}
          availableProviders={availableProviders}
          contextWindowSnapshot={contextWindowSnapshot}
          previousPrompt={previousPrompt}
          onJumpToPreviousUserPrompt={onJumpToPreviousUserPrompt}
        />
      </div>
    </div>
  )
})
