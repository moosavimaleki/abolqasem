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

  return (
    // <VerticalLineContainer className="w-full">
      <div className="group/message relative text-pretty prose prose-sm dark:prose-invert px-0.5 w-full max-w-full space-y-4">
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
