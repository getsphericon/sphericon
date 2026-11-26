import { useTranslation } from 'react-i18next'

import type { FrequencyCapChannel } from '../../../generated/site/types.gen.ts'

// The names of the Channels a sending rule is set on.
export function useChannelNames(): Record<FrequencyCapChannel, string> {
  const { t } = useTranslation()
  return {
    email: t(($) => $.settings.sendingRules.channels.email),
    telegram: t(($) => $.settings.sendingRules.channels.telegram),
    sms: t(($) => $.settings.sendingRules.channels.sms),
  }
}
