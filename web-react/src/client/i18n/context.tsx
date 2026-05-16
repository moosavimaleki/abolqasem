import { createContext, useContext, useMemo, type ReactNode } from "react"
import type { AppLocale } from "../../shared/types"
import { DEFAULT_LOCALE, getDictionary, getLocaleDirection, normalizeLocale, type TranslationDictionary } from "."

interface I18nContextValue {
  locale: AppLocale
  direction: "ltr" | "rtl"
  t: TranslationDictionary
}

const fallbackLocale = DEFAULT_LOCALE
const fallbackContext: I18nContextValue = {
  locale: fallbackLocale,
  direction: getLocaleDirection(fallbackLocale),
  t: getDictionary(fallbackLocale),
}

const I18nContext = createContext<I18nContextValue>(fallbackContext)

export function I18nProvider({
  locale,
  children,
}: {
  locale: string | null | undefined
  children: ReactNode
}) {
  const normalizedLocale = normalizeLocale(locale)
  const value = useMemo<I18nContextValue>(() => ({
    locale: normalizedLocale,
    direction: getLocaleDirection(normalizedLocale),
    t: getDictionary(normalizedLocale),
  }), [normalizedLocale])

  return (
    <I18nContext.Provider value={value}>
      {children}
    </I18nContext.Provider>
  )
}

export function useI18n() {
  return useContext(I18nContext)
}
