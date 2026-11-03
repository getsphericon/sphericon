import { formatDate } from './datetime.ts'

// The Regional formats (#1010) the profile offers: common language-region pairs. The server takes
// any well-formed tag, so a value outside this list that an API client stored is still shown and
// kept until the User changes it; the list is a convenience, not a rule.
export const REGIONAL_FORMATS = [
  'en-US',
  'en-GB',
  'en-AU',
  'en-CA',
  'en-IN',
  'de-DE',
  'de-AT',
  'de-CH',
  'fr-FR',
  'fr-CA',
  'fr-CH',
  'es-ES',
  'es-MX',
  'es-AR',
  'pt-BR',
  'pt-PT',
  'it-IT',
  'nl-NL',
  'pl-PL',
  'cs-CZ',
  'sv-SE',
  'da-DK',
  'nb-NO',
  'fi-FI',
  'ru-RU',
  'uk-UA',
  'tr-TR',
  'ja-JP',
  'ko-KR',
  'zh-CN',
  'zh-TW',
]

// The day an option's label shows its date in: the last of a year, so that a day-first and a
// month-first format differ at a glance.
const SAMPLE_DAY = '2026-12-31T12:00:00Z'

// A Regional format's name in the language of the UI, followed by a date written in it:
// "German (Germany) · 31.12.2026".
function regionalFormatLabel(tag: string, uiLocale: string): string {
  const name = new Intl.DisplayNames(uiLocale, { type: 'language', languageDisplay: 'standard' })
  return `${name.of(tag) ?? tag} · ${formatDate(SAMPLE_DAY, tag, 'UTC')}`
}

// The options of the Regional format select, named in the language of the UI and sorted by name;
// the User's current tag is added when it is not in the list.
export function regionalFormatOptions(
  uiLocale: string,
  current: string | null,
): { value: string; label: string }[] {
  const tags =
    current !== null && !REGIONAL_FORMATS.includes(current)
      ? [...REGIONAL_FORMATS, current]
      : REGIONAL_FORMATS
  return tags
    .map((tag) => ({ value: tag, label: regionalFormatLabel(tag, uiLocale) }))
    .toSorted((a, b) => a.label.localeCompare(b.label, uiLocale))
}
