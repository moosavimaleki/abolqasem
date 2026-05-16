import { CircleSlash } from "lucide-react"
import type { ProcessedInterruptedMessage } from "./types"
import { useI18n } from "../../i18n/context"

interface Props {
  message: ProcessedInterruptedMessage
}

export function InterruptedMessage({ message: _message }: Props) {
  const { t } = useI18n()
  return (
    <div className="flex items-center justify-end gap-2 text-sm text-muted-foreground">
      <div className="inline-flex gap-1.5 items-center justify-center whitespace-nowrap text-sm font-medium bg-background text-foreground/60 border border-border h-9 pl-1 pr-4 rounded-full">
        <CircleSlash className="h-4 w-4 ml-1.5" />
        <em>{t.messages.interrupted}</em>
      </div>
    </div>
  )
}
