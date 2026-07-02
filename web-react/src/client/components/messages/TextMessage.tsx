import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { ProcessedTextMessage } from "./types"
import { createMarkdownComponents, MessageCopyButton } from "./shared"
import { cn } from "../../lib/utils"
import { useI18n } from "../../i18n/context"

interface Props {
  message: ProcessedTextMessage
}

export function TextMessage({ message }: Props) {
  const { t, direction } = useI18n()
  const isTmuxMessage = message.id.startsWith("tmux-capture-")

  return (
    // <VerticalLineContainer className="w-full">
      <div
        className={cn(
          "group/message relative w-full max-w-full space-y-4",
          isTmuxMessage
            ? "rounded-lg border border-cyan-400/10 bg-slate-950/35 px-4 py-3 text-[0.9rem] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] prose prose-sm dark:prose-invert [&_.code-frame]:border-cyan-400/15 [&_.code-frame]:bg-slate-950/80 [&_.code-frame_pre]:text-[12px]"
            : "text-pretty prose prose-sm dark:prose-invert px-0.5"
        )}
      >
        <MessageCopyButton
          text={message.text}
          label={t.common.copyMessage}
          copiedLabel={t.common.copied}
          className={cn(
            "absolute top-0 z-10",
            direction === "rtl" ? "-right-8" : "-left-8"
          )}
        />
        <Markdown remarkPlugins={[remarkGfm]} components={createMarkdownComponents({ source: message.text })}>{message.text}</Markdown>
      </div>
    // </VerticalLineContainer>
  )
}
