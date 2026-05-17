import { useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react"
import {
  Check,
  Columns3,
  Minus,
  Palette,
  Plus,
  Rows3,
  Settings2,
  Type,
  type LucideIcon,
} from "lucide-react"
import { Button } from "../ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover"
import { cn } from "../../lib/utils"
import { useI18n } from "../../i18n/context"

const READER_SETTINGS_KEY = "ai-agent-manager.abolqasem-reader-settings.v1"
const READER_SETTINGS_CHANGE_EVENT = "ai-agent-manager:reader-settings-change"

export type ReaderFont = "vazirmatn" | "naskh" | "kufi" | "body"
export type ReaderWidth = "focus" | "comfortable" | "wide"
export type ReaderTheme = "paper" | "sepia" | "night" | "charcoal" | "teal"

export interface ReaderSettings {
  font: ReaderFont
  fontSize: number
  lineHeight: number
  width: ReaderWidth
  theme: ReaderTheme
}

export const defaultReaderSettings: ReaderSettings = {
  font: "vazirmatn",
  fontSize: 18,
  lineHeight: 1.9,
  width: "comfortable",
  theme: "night",
}

const fontOptions: Array<{ value: ReaderFont; label: string; sample: string }> = [
  { value: "vazirmatn", label: "Vazirmatn", sample: "خواندن روان" },
  { value: "naskh", label: "Naskh", sample: "متن کلاسیک" },
  { value: "kufi", label: "Kufi", sample: "تیتر خوانا" },
  { value: "body", label: "Abolqasem", sample: "UI font" },
]

const widthOptions: Array<{ value: ReaderWidth; labelFa: string; labelEn: string }> = [
  { value: "focus", labelFa: "متمرکز", labelEn: "Focus" },
  { value: "comfortable", labelFa: "راحت", labelEn: "Comfort" },
  { value: "wide", labelFa: "عریض", labelEn: "Wide" },
]

const lineHeightOptions = [1.65, 1.9, 2.15]

const themeOptions: Array<{ value: ReaderTheme; labelFa: string; labelEn: string; swatch: string }> = [
  { value: "paper", labelFa: "کاغذ", labelEn: "Paper", swatch: "bg-[linear-gradient(135deg,#fbf8f1_0_50%,#805f34_50%)]" },
  { value: "sepia", labelFa: "سپیا", labelEn: "Sepia", swatch: "bg-[linear-gradient(135deg,#2a211d_0_50%,#f0c28b_50%)]" },
  { value: "night", labelFa: "شب", labelEn: "Night", swatch: "bg-[linear-gradient(135deg,#07101d_0_50%,#9bc8ff_50%)]" },
  { value: "charcoal", labelFa: "ذغالی", labelEn: "Charcoal", swatch: "bg-[linear-gradient(135deg,#0f1115_0_50%,#b7c2d8_50%)]" },
  { value: "teal", labelFa: "کله‌غازی", labelEn: "Teal", swatch: "bg-[linear-gradient(135deg,#041816_0_50%,#6ee7d8_50%)]" },
]

type ReaderSettingsUpdater = ReaderSettings | ((current: ReaderSettings) => ReaderSettings)

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  const numberValue = typeof value === "number" ? value : Number(value)
  if (!Number.isFinite(numberValue)) return fallback
  return Math.min(max, Math.max(min, numberValue))
}

function normalizeFont(value: unknown): ReaderFont {
  switch (value) {
    case "bzar":
    case "naskh":
      return "naskh"
    case "kufi":
      return "kufi"
    case "ui":
    case "body":
      return "body"
    case "vazir":
    case "iranian":
    case "vazirmatn":
    default:
      return "vazirmatn"
  }
}

function normalizeWidth(value: unknown): ReaderWidth {
  switch (value) {
    case "contained":
      return "comfortable"
    case "focus":
      return "focus"
    case "wide":
      return "wide"
    case "comfortable":
    default:
      return "comfortable"
  }
}

function normalizeTheme(value: unknown): ReaderTheme {
  switch (value) {
    case "paper":
      return "paper"
    case "sepia":
      return "sepia"
    case "night":
      return "night"
    case "charcoal":
      return "charcoal"
    case "teal":
      return "teal"
    case "abolqasem":
    case "dark":
      return "night"
    case "light":
      return "paper"
    default:
      return "night"
  }
}

function normalizeReaderSettings(value: unknown): ReaderSettings {
  const input = value && typeof value === "object" ? value as Record<string, unknown> : {}
  return {
    font: normalizeFont(input.font),
    fontSize: clampNumber(input.fontSize, 14, 30, defaultReaderSettings.fontSize),
    lineHeight: clampNumber(input.lineHeight, 1.35, 2.5, defaultReaderSettings.lineHeight),
    width: normalizeWidth(input.width),
    theme: normalizeTheme(input.theme),
  }
}

function loadReaderSettings(): ReaderSettings {
  if (typeof window === "undefined") return defaultReaderSettings
  try {
    const raw = window.localStorage.getItem(READER_SETTINGS_KEY)
    return raw ? normalizeReaderSettings(JSON.parse(raw)) : defaultReaderSettings
  } catch {
    return defaultReaderSettings
  }
}

function persistReaderSettings(settings: ReaderSettings) {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(READER_SETTINGS_KEY, JSON.stringify(settings))
  } catch {
    // Keep the in-memory state usable when localStorage is blocked.
  }
  window.dispatchEvent(new CustomEvent(READER_SETTINGS_CHANGE_EVENT, { detail: settings }))
}

export function readerFontFamily(font: ReaderFont) {
  switch (font) {
    case "naskh":
      return '"Noto Naskh Arabic Variable", "Vazirmatn Variable", Georgia, serif'
    case "kufi":
      return '"Noto Kufi Arabic Variable", "Vazirmatn Variable", Tahoma, sans-serif'
    case "body":
      return '"Body", "Vazirmatn Variable", sans-serif'
    case "vazirmatn":
    default:
      return '"Vazirmatn Variable", "Body", Tahoma, sans-serif'
  }
}

function readerFontWeight(font: ReaderFont) {
  return font === "naskh" ? 450 : 400
}

export function useReaderAppearanceSettings() {
  const [settings, setSettingsState] = useState(loadReaderSettings)

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== READER_SETTINGS_KEY) return
      try {
        setSettingsState(event.newValue ? normalizeReaderSettings(JSON.parse(event.newValue)) : defaultReaderSettings)
      } catch {
        setSettingsState(defaultReaderSettings)
      }
    }

    const handleChange = (event: Event) => {
      const detail = (event as CustomEvent<ReaderSettings>).detail
      setSettingsState(normalizeReaderSettings(detail))
    }

    window.addEventListener("storage", handleStorage)
    window.addEventListener(READER_SETTINGS_CHANGE_EVENT, handleChange)
    return () => {
      window.removeEventListener("storage", handleStorage)
      window.removeEventListener(READER_SETTINGS_CHANGE_EVENT, handleChange)
    }
  }, [])

  const setSettings = useCallback((updater: ReaderSettingsUpdater) => {
    setSettingsState((current) => {
      const next = normalizeReaderSettings(typeof updater === "function" ? updater(current) : updater)
      persistReaderSettings(next)
      return next
    })
  }, [])

  return [settings, setSettings] as const
}

export function getAppearanceTextStyle(settings: ReaderSettings): CSSProperties {
  return {
    fontFamily: readerFontFamily(settings.font),
    fontSize: `${settings.fontSize}px`,
    fontWeight: readerFontWeight(settings.font),
    lineHeight: settings.lineHeight,
  }
}

export function getAppearanceThemeClassName(settings: ReaderSettings) {
  const darkTheme = isDarkAppearanceTheme(settings.theme)
  return cn(
    "appearance-theme",
    settings.theme === "paper" && "appearance-light",
    settings.theme === "paper" && "appearance-theme-paper",
    settings.theme === "sepia" && "appearance-theme-sepia",
    settings.theme === "night" && "appearance-theme-night",
    settings.theme === "charcoal" && "appearance-theme-charcoal",
    settings.theme === "teal" && "appearance-theme-teal",
    darkTheme && "dark",
  )
}

const appearanceDocumentClassNames = [
  "appearance-theme",
  "appearance-light",
  "appearance-theme-paper",
  "appearance-theme-sepia",
  "appearance-theme-night",
  "appearance-theme-charcoal",
  "appearance-theme-teal",
  "dark",
]

export function applyAppearanceThemeClassName(target: HTMLElement, settings: ReaderSettings) {
  target.classList.remove(...appearanceDocumentClassNames)
  target.classList.add(...getAppearanceThemeClassName(settings).split(/\s+/).filter(Boolean))
}

export function useDocumentAppearanceTheme(settings: ReaderSettings) {
  useEffect(() => {
    if (typeof document === "undefined") return
    applyAppearanceThemeClassName(document.documentElement, settings)
    applyAppearanceThemeClassName(document.body, settings)
    return () => {
      document.documentElement.classList.remove(...appearanceDocumentClassNames)
      document.body.classList.remove(...appearanceDocumentClassNames)
    }
  }, [settings])
}

export function isDarkAppearanceTheme(theme: ReaderTheme) {
  return theme !== "paper"
}

export function getAppearanceRootClassName(settings: ReaderSettings) {
  return cn(
    "flex min-h-0 flex-1 flex-col overflow-hidden bg-background text-foreground",
    getAppearanceThemeClassName(settings),
  )
}

export function getAppearanceHeaderClassName() {
  return "sticky top-0 z-20 flex h-14 shrink-0 items-center justify-between gap-4 border-b border-border bg-background/88 px-4 backdrop-blur-xl"
}

export function getAppearanceArticleClassName(settings: ReaderSettings) {
  return cn(
    "reader-article prose max-w-none px-5 py-10 md:px-8 md:py-14",
    settings.width === "focus" && "max-w-[680px]",
    settings.width === "comfortable" && "max-w-[820px]",
    settings.width === "wide" && "max-w-[1040px]",
    isDarkAppearanceTheme(settings.theme) && "prose-invert",
  )
}

function labels(isPersian: boolean) {
  return {
    reader: isPersian ? "حالت مطالعه" : "Reader",
    settings: isPersian ? "تنظیمات نمایش" : "Appearance settings",
    font: isPersian ? "فونت" : "Font",
    size: isPersian ? "اندازه متن" : "Text size",
    lineHeight: isPersian ? "فاصله خط" : "Line height",
    width: isPersian ? "عرض متن" : "Text width",
    theme: isPersian ? "رنگ‌بندی" : "Color",
    decrease: isPersian ? "کوچک‌تر" : "Smaller",
    increase: isPersian ? "بزرگ‌تر" : "Larger",
  }
}

function SettingGroup({
  icon: Icon,
  label,
  children,
}: {
  icon: LucideIcon
  label: string
  children: ReactNode
}) {
  return (
    <section className="space-y-2 rounded-2xl border border-border/70 bg-muted/20 p-3">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        <span>{label}</span>
      </div>
      {children}
    </section>
  )
}

export function ReaderAppearancePopover({
  title,
  trigger,
  align,
  sideOffset = 10,
}: {
  title?: string
  trigger?: ReactNode
  align?: "start" | "center" | "end"
  sideOffset?: number
}) {
  const { locale, direction } = useI18n()
  const documentDirection = typeof document !== "undefined" ? document.documentElement.dir : ""
  const isPersian = locale === "fa" || direction === "rtl" || documentDirection === "rtl"
  const text = labels(isPersian)
  const [settings, setSettings] = useReaderAppearanceSettings()
  const resolvedAlign = align ?? (isPersian ? "start" : "end")

  const defaultTrigger = useMemo(() => (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-9 w-9 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
      aria-label={text.settings}
      title={text.settings}
    >
      <Settings2 className="h-4 w-4" />
    </Button>
  ), [text.settings])

  return (
    <Popover>
      <PopoverTrigger asChild>
        {trigger ?? defaultTrigger}
      </PopoverTrigger>
      <PopoverContent
        align={resolvedAlign}
        sideOffset={sideOffset}
        className={cn("w-[min(360px,calc(100vw-24px))] rounded-3xl p-2", getAppearanceThemeClassName(settings))}
      >
        <div className="space-y-2">
          <div className="px-2 pb-1 pt-2">
            <div className="text-sm font-medium text-foreground">{text.settings}</div>
            {title ? <div className="mt-1 text-xs text-muted-foreground">{title}</div> : null}
          </div>

          <SettingGroup icon={Palette} label={text.theme}>
            <div className="grid grid-cols-5 gap-1.5">
              {themeOptions.map((theme) => (
                <button
                  key={theme.value}
                  type="button"
                  className={cn(
                    "flex min-h-14 flex-col items-center justify-center gap-1.5 rounded-xl border px-1.5 py-2 text-[11px] leading-none transition-colors",
                    settings.theme === theme.value
                      ? "border-logo/60 bg-logo/10 text-foreground"
                      : "border-border/70 bg-background/55 text-muted-foreground hover:bg-muted/50 hover:text-foreground",
                  )}
                  onClick={() => setSettings((current) => ({ ...current, theme: theme.value }))}
                >
                  <span className={cn("h-4 w-4 rounded-full border border-border shadow-sm", theme.swatch)} aria-hidden="true" />
                  <span className="max-w-full truncate">{isPersian ? theme.labelFa : theme.labelEn}</span>
                </button>
              ))}
            </div>
          </SettingGroup>

          <SettingGroup icon={Type} label={text.font}>
            <div className="grid grid-cols-2 gap-1.5">
              {fontOptions.map((font) => (
                <button
                  key={font.value}
                  type="button"
                  className={cn(
                    "flex min-h-16 flex-col items-start justify-between rounded-xl border px-3 py-2 text-start transition-colors",
                    settings.font === font.value
                      ? "border-logo/60 bg-logo/10 text-foreground"
                      : "border-border/70 bg-background/55 text-muted-foreground hover:bg-muted/50 hover:text-foreground",
                  )}
                  style={{ fontFamily: readerFontFamily(font.value) }}
                  onClick={() => setSettings((current) => ({ ...current, font: font.value }))}
                >
                  <span className="flex w-full items-center justify-between gap-2 text-xs font-medium">
                    {font.label}
                    {settings.font === font.value ? <Check className="h-3.5 w-3.5 text-logo" /> : null}
                  </span>
                  <span className="text-[13px] leading-5">{font.sample}</span>
                </button>
              ))}
            </div>
          </SettingGroup>

          <SettingGroup icon={Type} label={text.size}>
            <div className="grid grid-cols-[auto_1fr_auto] items-center gap-2">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 rounded-xl"
                aria-label={text.decrease}
                onClick={() => setSettings((current) => ({ ...current, fontSize: Math.max(14, current.fontSize - 1) }))}
              >
                <Minus className="h-3.5 w-3.5" />
              </Button>
              <div className="rounded-xl border border-border/70 bg-background/55 px-3 py-1.5 text-center text-sm font-medium">
                {settings.fontSize}px
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 rounded-xl"
                aria-label={text.increase}
                onClick={() => setSettings((current) => ({ ...current, fontSize: Math.min(30, current.fontSize + 1) }))}
              >
                <Plus className="h-3.5 w-3.5" />
              </Button>
            </div>
          </SettingGroup>

          <SettingGroup icon={Rows3} label={text.lineHeight}>
            <div className="grid grid-cols-3 gap-1.5">
              {lineHeightOptions.map((lineHeight) => (
                <button
                  key={lineHeight}
                  type="button"
                  className={cn(
                    "rounded-xl border px-3 py-2 text-sm transition-colors",
                    Math.abs(settings.lineHeight - lineHeight) < 0.01
                      ? "border-logo/60 bg-logo/10 text-foreground"
                      : "border-border/70 bg-background/55 text-muted-foreground hover:bg-muted/50 hover:text-foreground",
                  )}
                  onClick={() => setSettings((current) => ({ ...current, lineHeight }))}
                >
                  {lineHeight.toFixed(2)}
                </button>
              ))}
            </div>
          </SettingGroup>

          <SettingGroup icon={Columns3} label={text.width}>
            <div className="grid grid-cols-3 gap-1.5">
              {widthOptions.map((width) => (
                <button
                  key={width.value}
                  type="button"
                  className={cn(
                    "rounded-xl border px-2 py-2 text-xs transition-colors",
                    settings.width === width.value
                      ? "border-logo/60 bg-logo/10 text-foreground"
                      : "border-border/70 bg-background/55 text-muted-foreground hover:bg-muted/50 hover:text-foreground",
                  )}
                  onClick={() => setSettings((current) => ({ ...current, width: width.value }))}
                >
                  {isPersian ? width.labelFa : width.labelEn}
                </button>
              ))}
            </div>
          </SettingGroup>
        </div>
      </PopoverContent>
    </Popover>
  )
}
