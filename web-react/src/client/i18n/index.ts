import type { AppLocale } from "../../shared/types"
import { en } from "./en"
import { fa } from "./fa"

export const DEFAULT_LOCALE: AppLocale = "en"

export const LOCALE_OPTIONS: Array<{ value: AppLocale; labelKey: "english" | "persian" }> = [
  { value: "en", labelKey: "english" },
  { value: "fa", labelKey: "persian" },
]

const dictionaries = {
  en,
  fa,
}

export function normalizeLocale(value: string | null | undefined): AppLocale {
  return value === "fa" || value === "en" ? value : DEFAULT_LOCALE
}

export function getLocaleDirection(locale: AppLocale) {
  return locale === "fa" ? "rtl" : "ltr"
}

export function getDictionary(locale: AppLocale) {
  return dictionaries[locale]
}

