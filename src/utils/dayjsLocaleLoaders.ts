// The date inputs (Mantine's pickers) draw their month names, weekdays and `L` / `LL` formats from
// dayjs locale data, which is not loaded until asked for. One loader per locale the Regional format
// list offers (src/utils/regionalFormats.ts), each its own chunk; a tag with no data of its own
// falls back to its language's, and then to English, which dayjs has built in. Everything the app
// shows outside a date input is written by Intl, not by this data.
//
// A table of dynamic imports and nothing else: dayjsLocale.test.ts loads every one, and the
// coverage run leaves the file out because v8 counts each `import()` as a branch of its own.
export const DAYJS_LOCALE_LOADERS: Record<string, () => Promise<unknown>> = {
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
