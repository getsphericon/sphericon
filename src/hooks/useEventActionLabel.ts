import type { OptionsFilter } from '@mantine/core'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

// useEventActionLabel names an Event action for a person: an action the system records (`email.*`,
// `marketing.*`, `contact.*`, and the bus's `audit.entry` a webhook subscribes to) by its words, any
// other action as it was tracked. The action itself stays what is stored, filtered and sent; only
// the screen shows the words.
export function useEventActionLabel() {
  const { t } = useTranslation()

  return useCallback(
    (action: string): string => {
      switch (action) {
        case 'contact.created':
          return t(($) => $.eventActions.contactCreated)
        case 'contact.erased':
          return t(($) => $.eventActions.contactErased)
        case 'email.sent':
          return t(($) => $.eventActions.emailSent)
        case 'email.delivered':
          return t(($) => $.eventActions.emailDelivered)
        case 'email.delayed':
          return t(($) => $.eventActions.emailDelayed)
        case 'email.dropped':
          return t(($) => $.eventActions.emailDropped)
        case 'email.bounced':
          return t(($) => $.eventActions.emailBounced)
        case 'email.complained':
          return t(($) => $.eventActions.emailComplained)
        case 'email.opened':
          return t(($) => $.eventActions.emailOpened)
        case 'email.clicked':
          return t(($) => $.eventActions.emailClicked)
        case 'email.unsubscribed':
          return t(($) => $.eventActions.emailUnsubscribed)
        case 'marketing.confirmed':
          return t(($) => $.eventActions.marketingConfirmed)
        case 'audit.entry':
          return t(($) => $.eventActions.auditEntry)
        default:
          return action
      }
    },
    [t],
  )
}

// useEventActionFilter is the search of a picker of actions: an option matches by its words or by
// its code, so "spam" finds email.complained and "email.opened" finds Opened an email.
export function useEventActionFilter(): OptionsFilter {
  const labelOf = useEventActionLabel()

  return useCallback(
    ({ options, search }) => {
      const needle = search.trim().toLowerCase()
      if (needle === '') {
        return options
      }
      return options.filter(
        (option) =>
          'value' in option &&
          [option.value, labelOf(option.value)].some((text) => text.toLowerCase().includes(needle)),
      )
    },
    [labelOf],
  )
}
