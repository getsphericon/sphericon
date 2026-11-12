import type { OptionsFilter } from '@mantine/core'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SystemEventAction } from '../generated/site/types.gen.ts'
import { actionFilter } from '../utils/actionFilter.ts'
import { AUDIT_ENTRY_EVENT, PAGE_VIEW_ACTION } from '../utils/systemEvents.ts'
import type { WordedAction } from '../utils/systemEvents.ts'

// useEventActionLabel names an Event action for a person: an action the system records (`email.*`,
// `marketing.*`, `contact.*`, `automation.*`, `telegram.*`, `sms.*`, and the bus's `audit.entry` a webhook subscribes to)
// and the first-party tracker's `page.view` by its words, any other action as it was tracked. The
// action itself stays what is stored, filtered and sent; only the screen shows the words.
export function useEventActionLabel() {
  const { t } = useTranslation()

  return useMemo(() => {
    const words: Record<WordedAction, string> = {
      [SystemEventAction.CONTACT_CREATED]: t(($) => $.eventActions.contactCreated),
      [SystemEventAction.CONTACT_ERASED]: t(($) => $.eventActions.contactErased),
      [SystemEventAction.CONTACT_TAGGED]: t(($) => $.eventActions.contactTagged),
      [SystemEventAction.CONTACT_UNTAGGED]: t(($) => $.eventActions.contactUntagged),
      [SystemEventAction.CONTACT_FIELD_CHANGED]: t(($) => $.eventActions.contactFieldChanged),
      [SystemEventAction.AUTOMATION_ENROLLED]: t(($) => $.eventActions.automationEnrolled),
      [SystemEventAction.AUTOMATION_COMPLETED]: t(($) => $.eventActions.automationCompleted),
      [SystemEventAction.AUTOMATION_GOAL_REACHED]: t(($) => $.eventActions.automationGoalReached),
      [SystemEventAction.AUTOMATION_EXITED]: t(($) => $.eventActions.automationExited),
      [SystemEventAction.TELEGRAM_BOUND]: t(($) => $.eventActions.telegramBound),
      [SystemEventAction.TELEGRAM_CLICKED]: t(($) => $.eventActions.telegramClicked),
      [SystemEventAction.TELEGRAM_RECEIVED]: t(($) => $.eventActions.telegramReceived),
      [SystemEventAction.TELEGRAM_UNBOUND]: t(($) => $.eventActions.telegramUnbound),
      [SystemEventAction.TELEGRAM_UNSUBSCRIBED]: t(($) => $.eventActions.telegramUnsubscribed),
      [SystemEventAction.TELEGRAM_SENT]: t(($) => $.eventActions.telegramSent),
      [SystemEventAction.SMS_CLICKED]: t(($) => $.eventActions.smsClicked),
      [SystemEventAction.SMS_CONFIRMED]: t(($) => $.eventActions.smsConfirmed),
      [SystemEventAction.SMS_RECEIVED]: t(($) => $.eventActions.smsReceived),
      [SystemEventAction.SMS_SENT]: t(($) => $.eventActions.smsSent),
      [SystemEventAction.SMS_DELIVERED]: t(($) => $.eventActions.smsDelivered),
      [SystemEventAction.SMS_UNDELIVERED]: t(($) => $.eventActions.smsUndelivered),
      [SystemEventAction.SMS_UNSUBSCRIBED]: t(($) => $.eventActions.smsUnsubscribed),
      [PAGE_VIEW_ACTION]: t(($) => $.eventActions.pageView),
      [SystemEventAction.EMAIL_SENT]: t(($) => $.eventActions.emailSent),
      [SystemEventAction.EMAIL_DELIVERED]: t(($) => $.eventActions.emailDelivered),
      [SystemEventAction.EMAIL_DELAYED]: t(($) => $.eventActions.emailDelayed),
      [SystemEventAction.EMAIL_DROPPED]: t(($) => $.eventActions.emailDropped),
      [SystemEventAction.EMAIL_BOUNCED]: t(($) => $.eventActions.emailBounced),
      [SystemEventAction.EMAIL_COMPLAINED]: t(($) => $.eventActions.emailComplained),
      [SystemEventAction.EMAIL_OPENED]: t(($) => $.eventActions.emailOpened),
      [SystemEventAction.EMAIL_CLICKED]: t(($) => $.eventActions.emailClicked),
      [SystemEventAction.EMAIL_UNSUBSCRIBED]: t(($) => $.eventActions.emailUnsubscribed),
      [SystemEventAction.MARKETING_CONFIRMED]: t(($) => $.eventActions.marketingConfirmed),
      [AUDIT_ENTRY_EVENT]: t(($) => $.eventActions.auditEntry),
    }
    const worded = (action: string): action is WordedAction => Object.hasOwn(words, action)
    return (action: string): string => (worded(action) ? words[action] : action)
  }, [t])
}

// useEventActionFilter is the search of a picker of Event actions: an option matches by its words or
// by its code.
export function useEventActionFilter(): OptionsFilter {
  const labelOf = useEventActionLabel()
  return useMemo(() => actionFilter(({ value }) => labelOf(value)), [labelOf])
}
