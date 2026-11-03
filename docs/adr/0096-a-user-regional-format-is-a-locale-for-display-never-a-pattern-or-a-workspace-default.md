---
status: accepted
---

# A User Regional format is a locale for display, never a pattern or a Workspace default

Every date, time and number the app shows was written in the Instance's language: an `en` Instance
showed `12/31/2026` and 12-hour time to everyone, including a team in Europe that reads `31.12.2026`
and 24-hour time. A User could say where they are (ADR 0068) but not how they write a date.

## Decisions

- **Regional format** (GLOSSARY): a BCP 47 tag with a region (`en-GB`, `de-DE`) on the User, empty
  until they choose one on their profile. The server accepts any well-formed tag, as it does for the
  Contact locale, and stores it in its canonical spelling; the profile offers about thirty common
  pairs, as a convenience and not a rule.
- **A locale, not a pattern.** There is no `MM/DD/YYYY` choice and no separate 12/24-hour setting:
  the locale decides the order of the parts, the separators and the clock, and it decides numbers,
  percentages, money and durations too, so a date never sits next to a number written the other way.
- **On the User only,** for the reason of ADR 0068: a User belongs to several Workspaces, and a
  personal display preference does not change with the Workspace. There is no Workspace default and
  no override layer.
- **Empty is the Instance's language,** which is what every User saw before. It is a default, not a
  fallback: nothing changes for a User who never chooses.
- **The UI language stays the Instance's.** An English UI with `de-DE` formatting is expected; this
  is not a per-User language.
- **One path on the frontend.** A signed-in page formats through one hook that supplies the User time
  zone and the Regional format from the cached profile; the formatting helpers take the locale as an
  argument and never read the Instance's language themselves (an `ast-grep` rule keeps it so). The
  formats come from `Intl`, not from dayjs locale data, which only serves the date inputs' month
  names and typed-date parsing and is loaded when a format is first used.
- **A number inside a string follows it too (#1022).** A quantity a string prints is written
  `{{count, number}}`, never passed in already formatted: `count` stays a number, so the plural
  form is still picked by the UI language's rules. i18next 26 has no `interpolation.format`, so the
  built-in `number` formatter is replaced after init by one that formats in the Regional format
  (`setRegionalFormat` in `src/i18n.ts`), and `<RegionalNumbers>`, mounted once, feeds it from the
  cached profile and re-renders the strings on screen when it changes; it is mounted at the root
  and pinned to the Instance's language under `/console`. No `useTranslation` wrapper is needed,
  and `src/locales.test.ts` fails a bare `{{count}}`. An identifier (a version, an
  id, an IP address) is not a quantity and is printed as it is. A figure that is not in a string
  goes through `useFormat().formatNumber`, and a table's range through `usePaginationText()`.
- **Unaffected:** public pages and the Staff console (no User preference to read), mail to the User
  (still the Instance's formatting), parsing of incoming values (ingest stays ISO-only), Report days
  (ADR 0057) and everything addressed to a Contact, which follows the Contact locale.

## Considered options

- **A date pattern chosen from a list** (Drip's "Date Format"). Rejected: it leaves times and numbers
  in the old format, and a list of patterns has to grow with every country.
- **A Workspace default a User overrides** (HubSpot). Rejected: two layers for a setting a person
  holds about themselves; ADR 0068 rejected the same shape for the time zone.
- **The browser's locale when empty.** Rejected: the Instance's language is what every User already
  sees, and a browser set to another language would change the app without anyone choosing it.
