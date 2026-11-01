import { Image } from '@mantine/core'

import { countryName } from '../utils/countries.ts'

// Each flag-icons SVG as a URL keyed by its file, never inlined: the browser fetches only the
// flags shown.
const flagUrls = import.meta.glob<string>('/node_modules/flag-icons/flags/4x3/*.svg', {
  eager: true,
  query: '?url&no-inline',
  import: 'default',
})

// The flag of an ISO 3166-1 alpha-2 country code, titled with the country's name; a code
// flag-icons has no flag for draws nothing.
export function CountryFlag({ code }: { code: string }) {
  const src = flagUrls[`/node_modules/flag-icons/flags/4x3/${code.toLowerCase()}.svg`]
  if (src === undefined) {
    return null
  }
  const name = countryName(code)
  return <Image src={src} alt={name} title={name} h={14} w="auto" radius={2} />
}
