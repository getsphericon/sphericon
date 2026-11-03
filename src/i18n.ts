import { createInstance } from 'i18next'
import { initReactI18next } from 'react-i18next'
import * as z from 'zod'

import en from '../locales/en/translation.json' with { type: 'json' }

// Locale is a property of the instance (a SaaS deployment runs per country),
// fixed at startup — there is no in-app language switcher and no browser
// detection. The backend substitutes window.__APP_LOCALE__ into index.html at
// serve time (see internal/server/spa/spa.go); Vite does the same in dev. We read it
// synchronously here, so no extra request is needed at boot.
const SUPPORTED_LOCALES = ['en'] as const
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

const REGIONAL_FORMAT_CHANGED = 'regionalFormatChanged'

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
  // A string re-renders when the User's Regional format changes, as when the language does.
  react: { bindI18n: `languageChanged ${REGIONAL_FORMAT_CHANGED}` },
})

// The Regional format (#1010) the numbers inside strings are written in, or undefined for the
// Instance's language. The UI language, and with it the plural rules, never changes: only the
// digit grouping and the decimal separator of `{{count, number}}` do (#1022).
let regionalFormat: string | undefined

// i18next 26 has no `interpolation.format`: a formatter is added to the service after init, and
// it replaces the built-in `number` of the same name. The options after the name
// (`{{value, number(minimumFractionDigits: 2)}}`) reach `Intl.NumberFormat` as they did.
const formatter = i18n.services.formatter
if (!formatter) {
  throw new Error('i18next has no formatter after init: the number format would not apply')
}
formatter.add('number', (value: number, lng: string | undefined, options: object) =>
  new Intl.NumberFormat(regionalFormat ?? lng, options).format(value),
)

// setRegionalFormat tells the strings which Regional format their numbers follow, and re-renders
// the ones already on screen. Called by <RegionalNumbers>, the one place that reads the profile.
export function setRegionalFormat(tag: string | undefined): void {
  if (tag === regionalFormat) {
    return
  }
  regionalFormat = tag
  i18n.emit(REGIONAL_FORMAT_CHANGED)
}

// Zod's default message for a missing required value ("Invalid input: expected string,
// received undefined") is useless in a form. Whatever the rule that failed, a value is missing
// when it is absent (resource forms drop blank fields), null (a Select with nothing chosen) or
// an empty string (a trimmed text input left blank) or an empty required collection.
z.config({
  customError: (issue) =>
    issue.input === undefined ||
    issue.input === null ||
    issue.input === '' ||
    (Array.isArray(issue.input) && issue.input.length === 0)
      ? i18n.t(($) => $.validation.required)
      : undefined,
})

export { i18n }
