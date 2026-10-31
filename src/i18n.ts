import { createInstance } from 'i18next'
import { initReactI18next } from 'react-i18next'
import * as z from 'zod'

import en from '../locales/en/translation.json' with { type: 'json' }

// Locale is a property of the instance (a SaaS deployment runs per country),
// fixed at startup — there is no in-app language switcher and no browser
// detection. The backend substitutes window.__APP_LOCALE__ into index.html at
// serve time (see internal/server/spa/spa.go); Vite does the same in dev. We read it
// synchronously here, so no extra request is needed at boot.
export const SUPPORTED_LOCALES = ['en'] as const
export type Locale = (typeof SUPPORTED_LOCALES)[number]

function isLocale(value: unknown): value is Locale {
  return SUPPORTED_LOCALES.some((locale) => locale === value)
}

export function resolveLocale(): Locale {
  const injected: unknown = Reflect.get(window, '__APP_LOCALE__')
  return isLocale(injected) ? injected : 'en'
}

// The app runs on a DEDICATED i18next instance rather than the global default.
// @workflowbuilder/sdk calls i18next.init() on the global default at import time,
// which rebuilds the resource store and wipes our `translation` namespace. Owning
// our own instance keeps the app's strings independent of the SDK. Components must
// reach it through the <I18nextProvider> in main.tsx (the SDK subtree is wrapped in
// its own provider bound back to the global instance it initialises).
const i18n = createInstance()

// index.html ships a static lang="en" (so the a11y lint accepts a real code);
// reflect the actual instance locale onto <html lang> once we know it.
document.documentElement.lang = resolveLocale()

void i18n.use(initReactI18next).init({
  lng: resolveLocale(),
  fallbackLng: 'en',
  resources: {
    en: { translation: en },
  },
  interpolation: {
    escapeValue: false,
  },
})

// Zod's default message for a missing required value ("Invalid input: expected string,
// received undefined") is useless in a form. Whatever the rule that failed, a value is missing
// when it is absent (resource forms drop blank fields), null (a Select with nothing chosen) or
// an empty string (a trimmed text input left blank).
z.config({
  customError: (issue) =>
    issue.input === undefined || issue.input === null || issue.input === ''
      ? i18n.t(($) => $.validation.required)
      : undefined,
})

export { i18n }
