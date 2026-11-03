import dayjs from 'dayjs'

import { DAYJS_LOCALE_LOADERS } from './dayjsLocaleLoaders.ts'

// Every dayjs locale name that has a loader.
export const DAYJS_LOCALE_NAMES = Object.keys(DAYJS_LOCALE_LOADERS)

// The name of the dayjs locale a Regional format reads its dates from: the tag itself when dayjs
// has data for it (`en-GB` is `en-gb`), else its language's (`de-DE` is `de`), else English.
export function dayjsLocaleName(regionalFormat: string | undefined): string {
  if (regionalFormat === undefined) {
    return 'en'
  }
  const tag = regionalFormat.toLowerCase()
  const language = tag.replace(/-.*/, '')
  return [tag, language].find((name) => name in DAYJS_LOCALE_LOADERS) ?? 'en'
}

// Loads the locale data of a dayjs locale name, once; English is already there.
export async function loadDayjsLocale(name: string): Promise<void> {
  if (name === 'en' || dayjs.Ls[name] !== undefined) {
    return
  }
  await DAYJS_LOCALE_LOADERS[name]?.()
}

type Weekday = 0 | 1 | 2 | 3 | 4 | 5 | 6

const WEEKDAYS: readonly Weekday[] = [0, 1, 2, 3, 4, 5, 6]

// The first day of the week a locale starts its calendar on, as Mantine counts it (0 is Sunday,
// 1 Monday), or undefined when the browser cannot tell: the pickers then keep their own default.
export function firstDayOfWeek(regionalFormat: string | undefined): Weekday | undefined {
  if (regionalFormat === undefined) {
    return undefined
  }
  const locale = new Intl.Locale(regionalFormat)
  const info: unknown = Reflect.get(locale, 'getWeekInfo')
  const week: unknown =
    typeof info === 'function' ? Reflect.apply(info, locale, []) : Reflect.get(locale, 'weekInfo')
  const first: unknown =
    week !== null && typeof week === 'object' ? Reflect.get(week, 'firstDay') : undefined
  // Intl counts Monday as 1 and Sunday as 7; Mantine counts Sunday as 0.
  return typeof first === 'number' ? WEEKDAYS.find((day) => day === first % 7) : undefined
}
