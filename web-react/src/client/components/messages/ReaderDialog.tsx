import { Dialog, DialogContent, DialogTitle } from "../ui/dialog"
import { ReaderView } from "./ReaderView"

export function ReaderDialog({
  open,
  title,
  content,
  onOpenChange,
}: {
  open: boolean
  title: string
  content: string
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        hideClose
        aria-describedby={undefined}
        className="left-0 top-0 h-[100dvh] max-h-none w-screen max-w-none translate-x-0 translate-y-0 overflow-hidden rounded-none border-0 bg-background p-0 shadow-none duration-0 data-[state=closed]:animate-none data-[state=open]:animate-none"
      >
        <DialogTitle className="sr-only">{title}</DialogTitle>
        <ReaderView title={title} content={content} variant="dialog" onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  )
}
