import type { OptionsFilter } from '@mantine/core'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

// useEventActionLabel names an Event action for a person: an action the system records (`email.*`,
// `marketing.*`, `contact.*`, `automation.*`, and the bus's `audit.entry` a webhook subscribes to)
// and the first-party tracker's `page.view` by its words, any other action as it was tracked. The
// action itself stays what is stored, filtered and sent; only the screen shows the words.
export function useEventActionLabel() {
  const { t } = useTranslation()

  return useMemo(() => {
    const words: Record<string, string> = {
      'contact.created': t(($) => $.eventActions.contactCreated),
      'contact.erased': t(($) => $.eventActions.contactErased),
      'contact.tagged': t(($) => $.eventActions.contactTagged),
      'contact.untagged': t(($) => $.eventActions.contactUntagged),
      'contact.field_changed': t(($) => $.eventActions.contactFieldChanged),
      'automation.enrolled': t(($) => $.eventActions.automationEnrolled),
      'automation.completed': t(($) => $.eventActions.automationCompleted),
      'automation.goal_reached': t(($) => $.eventActions.automationGoalReached),
      'automation.exited': t(($) => $.eventActions.automationExited),
      'page.view': t(($) => $.eventActions.pageView),
      'email.sent': t(($) => $.eventActions.emailSent),
      'email.delivered': t(($) => $.eventActions.emailDelivered),
      'email.delayed': t(($) => $.eventActions.emailDelayed),
      'email.dropped': t(($) => $.eventActions.emailDropped),
      'email.bounced': t(($) => $.eventActions.emailBounced),
      'email.complained': t(($) => $.eventActions.emailComplained),
      'email.opened': t(($) => $.eventActions.emailOpened),
      'email.clicked': t(($) => $.eventActions.emailClicked),
      'email.unsubscribed': t(($) => $.eventActions.emailUnsubscribed),
      'marketing.confirmed': t(($) => $.eventActions.marketingConfirmed),
      'audit.entry': t(($) => $.eventActions.auditEntry),
    }
    return (action: string): string => words[action] ?? action
  }, [t])
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
