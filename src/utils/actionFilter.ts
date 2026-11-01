import type { ComboboxItem, OptionsFilter } from '@mantine/core'

// actionFilter is the search of a picker of actions whose words wordsOf gives: an option matches by
// its words or by its code, so "spam" finds email.complained and "email.opened" finds Opened an
// email.
export function actionFilter(wordsOf: (option: ComboboxItem) => string): OptionsFilter {
  return ({ options, search }) => {
    const needle = search.trim().toLowerCase()
    if (needle === '') {
      return options
    }
    return options.filter(
      (option) =>
        'value' in option &&
        [option.value, wordsOf(option)].some((text) => text.toLowerCase().includes(needle)),
    )
  }
}

// labelledActionFilter is the search of a picker whose options carry their words as their label.
export const labelledActionFilter = actionFilter(({ label }) => label)
