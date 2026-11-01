import flagIcons from 'flag-icons/country.json' with { type: 'json' }

const regionNames = new Intl.DisplayNames(['en'], { type: 'region' })

// The name of a country by its ISO 3166-1 alpha-2 code, or the code when it names none.
export function countryName(code: string): string {
  return regionNames.of(code) ?? code
}

// Every ISO 3166-1 country flag-icons draws, as Select options ordered by name. Its regions
// that are no country (the EU, England) are left out.
export const countryOptions = flagIcons
  .filter((country) => country.iso)
  .map((country) => {
    const value = country.code.toUpperCase()
    return { value, label: countryName(value) }
  })
  .toSorted((a, b) => a.label.localeCompare(b.label))
