import dayjs from 'dayjs'

// The date inputs (Mantine's pickers) draw their month names, weekdays and `L` / `LL` formats from
// dayjs locale data, which is not loaded until asked for. One loader per locale the Regional format
// list offers (src/utils/regionalFormats.ts), each its own chunk; a tag with no data of its own
// falls back to its language's, and then to English, which dayjs has built in. Everything the app
// shows outside a date input is written by Intl, not by this data.
const LOADERS: Record<string, () => Promise<unknown>> = {
  cs: async () => import('dayjs/locale/cs.js'),
  da: async () => import('dayjs/locale/da.js'),
  de: async () => import('dayjs/locale/de.js'),
  'de-at': async () => import('dayjs/locale/de-at.js'),
  'de-ch': async () => import('dayjs/locale/de-ch.js'),
  'en-au': async () => import('dayjs/locale/en-au.js'),
  'en-ca': async () => import('dayjs/locale/en-ca.js'),
  'en-gb': async () => import('dayjs/locale/en-gb.js'),
  'en-in': async () => import('dayjs/locale/en-in.js'),
  es: async () => import('dayjs/locale/es.js'),
  'es-mx': async () => import('dayjs/locale/es-mx.js'),
  fi: async () => import('dayjs/locale/fi.js'),
  fr: async () => import('dayjs/locale/fr.js'),
  'fr-ca': async () => import('dayjs/locale/fr-ca.js'),
  'fr-ch': async () => import('dayjs/locale/fr-ch.js'),
  it: async () => import('dayjs/locale/it.js'),
  ja: async () => import('dayjs/locale/ja.js'),
  ko: async () => import('dayjs/locale/ko.js'),
  nb: async () => import('dayjs/locale/nb.js'),
  nl: async () => import('dayjs/locale/nl.js'),
  pl: async () => import('dayjs/locale/pl.js'),
  pt: async () => import('dayjs/locale/pt.js'),
  'pt-br': async () => import('dayjs/locale/pt-br.js'),
  ru: async () => import('dayjs/locale/ru.js'),
  sv: async () => import('dayjs/locale/sv.js'),
  tr: async () => import('dayjs/locale/tr.js'),
  uk: async () => import('dayjs/locale/uk.js'),
  'zh-cn': async () => import('dayjs/locale/zh-cn.js'),
  'zh-tw': async () => import('dayjs/locale/zh-tw.js'),
}

// The name of the dayjs locale a Regional format reads its dates from: the tag itself when dayjs
// has data for it (`en-GB` is `en-gb`), else its language's (`de-DE` is `de`), else English.
export function dayjsLocaleName(regionalFormat: string | undefined): string {
  if (regionalFormat === undefined) {
    return 'en'
  }
  const tag = regionalFormat.toLowerCase()
  const language = tag.split('-')[0] ?? tag
  return [tag, language].find((name) => name in LOADERS) ?? 'en'
}

// Loads the locale data of a dayjs locale name, once; English is already there.
export async function loadDayjsLocale(name: string): Promise<void> {
  if (name === 'en' || dayjs.Ls[name] !== undefined) {
    return
  }
  await LOADERS[name]?.()
}

// The first day of the week a locale starts its calendar on, as Mantine counts it (0 is Sunday,
// 1 Monday), or undefined when the browser cannot tell: the pickers then keep their own default.
export function firstDayOfWeek(
  regionalFormat: string | undefined,
): 0 | 1 | 2 | 3 | 4 | 5 | 6 | undefined {
  if (regionalFormat === undefined) {
    return undefined
  }
  const locale = new Intl.Locale(regionalFormat)
  const info: unknown = Reflect.get(locale, 'getWeekInfo')
  const week: unknown =
    typeof info === 'function' ? Reflect.apply(info, locale, []) : Reflect.get(locale, 'weekInfo')
  const first: unknown =
    week !== null && typeof week === 'object' ? Reflect.get(week, 'firstDay') : undefined
  switch (first) {
    case 1:
      return 1
    case 2:
      return 2
    case 3:
      return 3
    case 4:
      return 4
    case 5:
      return 5
    case 6:
      return 6
    case 7:
      return 0
    default:
      return undefined
  }
}
