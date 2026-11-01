import type { OptionsFilter } from '@mantine/core'

// actionFilter is the search of a picker of actions whose words labelOf gives: an option matches by
// its words or by its code, so "spam" finds email.complained and "email.opened" finds Opened an
// email.
export function actionFilter(labelOf: (action: string) => string): OptionsFilter {
  return ({ options, search }) => {
    const needle = search.trim().toLowerCase()
    if (needle === '') {
      return options
    }
    return options.filter(
      (option) =>
        'value' in option &&
        [option.value, labelOf(option.value)].some((text) => text.toLowerCase().includes(needle)),
    )
  }
}
