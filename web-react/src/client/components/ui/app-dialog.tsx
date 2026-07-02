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
import { Bot, Code2, PencilLine, ShieldAlert, ShieldCheck, Sparkles, Terminal } from "lucide-react"
import { Button } from "./button"
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./dialog"
import { Input } from "./input"
import { cn } from "../../lib/utils"

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

type ChoiceDialogIcon = "claude" | "codex" | "gemini" | "custom" | "danger" | "safe" | "terminal"

interface ChoiceDialogOption {
  value: string
  label: string
  description?: string
  icon?: ChoiceDialogIcon | string
}

interface ChoiceDialogOptions {
  title: string
  description?: string
  choices: ChoiceDialogOption[]
  initialValue?: string
  cancelLabel?: string
  dir?: "ltr" | "rtl"
}

interface AppDialogContextValue {
  confirm: (options: ConfirmDialogOptions) => Promise<boolean>
  prompt: (options: PromptDialogOptions) => Promise<string | null>
  choice: (options: ChoiceDialogOptions) => Promise<string | null>
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
  | {
      kind: "choice"
      options: ChoiceDialogOptions
      resolve: (value: string | null) => void
    }

const AppDialogContext = createContext<AppDialogContextValue | null>(null)

const choiceIcons: Record<ChoiceDialogIcon, typeof Bot> = {
  claude: Bot,
  codex: Code2,
  gemini: Sparkles,
  custom: PencilLine,
  danger: ShieldAlert,
  safe: ShieldCheck,
  terminal: Terminal,
}

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
    } else if (dialogState.kind === "choice") {
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
    } else if (dialogState.kind === "choice") {
      dialogState.resolve(dialogState.options.initialValue ?? null)
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

  const choice = useCallback((options: ChoiceDialogOptions) => {
    return new Promise<string | null>((resolve) => {
      setDialogState({ kind: "choice", options, resolve })
    })
  }, [])

  const alert = useCallback((options: AlertDialogOptions) => {
    return new Promise<void>((resolve) => {
      setDialogState({ kind: "alert", options, resolve })
    })
  }, [])

  const value = useMemo<AppDialogContextValue>(() => ({ confirm, prompt, choice, alert }), [alert, choice, confirm, prompt])

  const resolveChoice = useCallback((choiceValue: string) => {
    if (dialogState?.kind !== "choice") return
    dialogState.resolve(choiceValue)
    closeDialog()
  }, [closeDialog, dialogState])

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
                {dialogState.kind === "choice" ? (
                  <div className="space-y-1">
                    {dialogState.options.choices.map((choiceOption) => {
                      const Icon = choiceOption.icon && choiceOption.icon in choiceIcons
                        ? choiceIcons[choiceOption.icon as ChoiceDialogIcon]
                        : null
                      const selected = dialogState.options.initialValue === choiceOption.value
                      return (
                        <button
                          key={choiceOption.value}
                          type="button"
                          className={cn(
                            "flex w-full items-start gap-3 rounded-xl border p-3 text-start transition-colors",
                            selected
                              ? "border-primary/30 bg-primary/10 text-foreground"
                              : "border-border/60 bg-background hover:border-border hover:bg-muted/70",
                          )}
                          onClick={() => resolveChoice(choiceOption.value)}
                        >
                          {Icon ? (
                            <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                              <Icon className="size-4" />
                            </span>
                          ) : null}
                          <span className="min-w-0">
                            <span className="block text-sm font-medium">{choiceOption.label}</span>
                            {choiceOption.description ? (
                              <span className="mt-1 block text-xs leading-5 text-muted-foreground">{choiceOption.description}</span>
                            ) : null}
                          </span>
                        </button>
                      )
                    })}
                  </div>
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
                  className={dialogState.kind === "choice" ? "hidden" : undefined}
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
