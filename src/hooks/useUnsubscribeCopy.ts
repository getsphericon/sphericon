import { useTranslation } from 'react-i18next'

import type { SiteIntegrationChannel } from '../generated/site/types.gen.ts'

// What the unsubscribe pages say, in the words of the Channel the link came on: a text's link
// speaks of text messages and a Telegram message's of Telegram messages, any other of emails.
export interface UnsubscribeCopy {
  ask: string
  done: string
  all: string
}

export function useUnsubscribeCopy(channel: SiteIntegrationChannel | undefined): UnsubscribeCopy {
  const { t } = useTranslation()
  const copy: Record<SiteIntegrationChannel, UnsubscribeCopy> = {
    email: {
      ask: t(($) => $.unsubscribe.body),
      done: t(($) => $.unsubscribed.body),
      all: t(($) => $.unsubscribed.unsubscribeAll),
    },
    sms: {
      ask: t(($) => $.unsubscribe.bodySms),
      done: t(($) => $.unsubscribed.bodySms),
      all: t(($) => $.unsubscribed.unsubscribeAllSms),
    },
    telegram: {
      ask: t(($) => $.unsubscribe.bodyTelegram),
      done: t(($) => $.unsubscribed.bodyTelegram),
      all: t(($) => $.unsubscribed.unsubscribeAllTelegram),
    },
  }
  return copy[channel ?? 'email']
}
