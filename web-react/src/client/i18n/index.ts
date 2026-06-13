import type { AppLocale } from "../../shared/types"
import { en, type TranslationDictionary } from "./en"
import { fa } from "./fa"

export const DEFAULT_LOCALE: AppLocale = "fa"
export const LOCALE_STORAGE_KEY = "abolqasem:locale"

export const LOCALE_OPTIONS: Array<{ value: AppLocale; labelKey: "english" | "persian" }> = [
  { value: "en", labelKey: "english" },
  { value: "fa", labelKey: "persian" },
]

const dictionaries: Record<AppLocale, TranslationDictionary> = {
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

export type { TranslationDictionary }
