import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"
import { Button } from "./button"
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./dialog"
import { Input } from "./input"

interface ConfirmDialogOptions {
  title: string
  description?: string
  confirmLabel?: string
  cancelLabel?: string
  confirmVariant?: "default" | "destructive" | "secondary"
  dir?: "ltr" | "rtl"
}

interface PromptDialogOptions {
  title: string
  description?: string
  placeholder?: string
  initialValue?: string
  allowEmpty?: boolean
  resetLabel?: string
  resetValue?: string
  confirmLabel?: string
  cancelLabel?: string
  dir?: "ltr" | "rtl"
}

interface AlertDialogOptions {
  title: string
  description?: string
  closeLabel?: string
  dir?: "ltr" | "rtl"
}

interface AppDialogContextValue {
  confirm: (options: ConfirmDialogOptions) => Promise<boolean>
  prompt: (options: PromptDialogOptions) => Promise<string | null>
  alert: (options: AlertDialogOptions) => Promise<void>
}

type DialogState =
  | {
      kind: "confirm"
      options: ConfirmDialogOptions
      resolve: (value: boolean) => void
    }
  | {
      kind: "prompt"
      options: PromptDialogOptions
      resolve: (value: string | null) => void
    }
  | {
      kind: "alert"
      options: AlertDialogOptions
      resolve: () => void
    }

const AppDialogContext = createContext<AppDialogContextValue | null>(null)

function resolveDialogDirection(value: string | undefined, fallback: "ltr" | "rtl" = "ltr") {
  const text = value ?? ""
  for (const char of text) {
    if ((char >= "؀" && char <= "ۿ") || (char >= "ݐ" && char <= "ݿ") || (char >= "ࢠ" && char <= "ࣿ")) {
      return "rtl"
    }
  }
  return fallback
}

export function AppDialogProvider({ children }: { children: ReactNode }) {
  const [dialogState, setDialogState] = useState<DialogState | null>(null)
  const [inputValue, setInputValue] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (dialogState?.kind !== "prompt") return
    setInputValue(dialogState.options.initialValue ?? "")
    setTimeout(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    }, 0)
  }, [dialogState])

  const closeDialog = useCallback(() => {
    setDialogState(null)
    setInputValue("")
  }, [])

  const resolveCancel = useCallback(() => {
    if (!dialogState) return
    if (dialogState.kind === "confirm") {
      dialogState.resolve(false)
    } else if (dialogState.kind === "prompt") {
      dialogState.resolve(null)
    } else {
      dialogState.resolve()
    }
    closeDialog()
  }, [closeDialog, dialogState])

  const resolveConfirm = useCallback(() => {
    if (!dialogState) return
    if (dialogState.kind === "confirm") {
      dialogState.resolve(true)
    } else if (dialogState.kind === "prompt") {
      const trimmed = inputValue.trim()
      dialogState.resolve(trimmed || (dialogState.options.allowEmpty ? "" : null))
    } else {
      dialogState.resolve()
    }
    closeDialog()
  }, [closeDialog, dialogState, inputValue])

  const confirm = useCallback((options: ConfirmDialogOptions) => {
    return new Promise<boolean>((resolve) => {
      setDialogState({ kind: "confirm", options, resolve })
    })
  }, [])

  const prompt = useCallback((options: PromptDialogOptions) => {
    return new Promise<string | null>((resolve) => {
      setDialogState({ kind: "prompt", options, resolve })
    })
  }, [])

  const alert = useCallback((options: AlertDialogOptions) => {
    return new Promise<void>((resolve) => {
      setDialogState({ kind: "alert", options, resolve })
    })
  }, [])

  const value = useMemo<AppDialogContextValue>(() => ({ confirm, prompt, alert }), [alert, confirm, prompt])

  const dialogDirection = dialogState?.options.dir
    ?? resolveDialogDirection(`${dialogState?.options.title ?? ""}
${dialogState?.options.description ?? ""}`, (typeof document !== "undefined" && document.documentElement.dir === "rtl") ? "rtl" : "ltr")

  return (
    <AppDialogContext.Provider value={value}>
      {children}
      <Dialog
        open={dialogState !== null}
        onOpenChange={(open) => {
          if (open || !dialogState) return
          resolveCancel()
        }}
      >
        <DialogContent
          size="sm"
          dir={dialogDirection}
          onKeyDown={(event) => {
            if (event.key !== "Enter" || event.shiftKey || !dialogState || dialogState.kind !== "confirm") return
            event.preventDefault()
            resolveConfirm()
          }}
        >
          {dialogState ? (
            <>
              <DialogBody className="space-y-4">
                <DialogTitle className="pe-8 leading-snug">{dialogState.options.title}</DialogTitle>
                {dialogState.options.description ? (
                  <DialogDescription className="whitespace-pre-line text-start leading-6">{dialogState.options.description}</DialogDescription>
                ) : null}
                {dialogState.kind === "prompt" ? (
                  <Input
                    ref={inputRef}
                    type="text"
                    value={inputValue}
                    onChange={(event) => setInputValue(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.preventDefault()
                        resolveConfirm()
                      }
                    }}
                    placeholder={dialogState.options.placeholder}
                  />
                ) : null}
              </DialogBody>
              <DialogFooter>
                {dialogState.kind === "prompt" && dialogState.options.resetLabel ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setInputValue(dialogState.options.resetValue ?? "")}
                    className="mr-auto"
                  >
                    {dialogState.options.resetLabel}
                  </Button>
                ) : null}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={resolveCancel}
                  className={dialogState.kind === "alert" ? "hidden" : undefined}
                >
                  {"cancelLabel" in dialogState.options ? (dialogState.options.cancelLabel ?? "Cancel") : "Cancel"}
                </Button>
                <Button
                  variant={dialogState.kind === "confirm" ? (dialogState.options.confirmVariant ?? "default") : "secondary"}
                  size="sm"
                  onClick={resolveConfirm}
                  disabled={dialogState.kind === "prompt" && !dialogState.options.allowEmpty && !inputValue.trim()}
                >
                  {dialogState.kind === "alert"
                    ? (dialogState.options.closeLabel ?? "OK")
                    : "confirmLabel" in dialogState.options
                      ? (dialogState.options.confirmLabel ?? "Confirm")
                      : "Confirm"}
                </Button>
              </DialogFooter>
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </AppDialogContext.Provider>
  )
}

export function useAppDialog() {
  const context = useContext(AppDialogContext)
  if (!context) {
    throw new Error("useAppDialog must be used within AppDialogProvider")
  }
  return context
}
