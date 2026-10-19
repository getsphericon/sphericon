import { Alert, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { useInstanceState } from '../hooks/useInstanceState.ts'

// LicenseBanner tells an Owner of the oldest Workspace that the Instance's license key
// expired and it runs the open-source core (ADR 0035). The state carries the expiry only
// while the key has lapsed, and is refused to everyone else, so nothing renders for them.
export function LicenseBanner() {
  const { t, i18n } = useTranslation()
  const state = useInstanceState()
  const expiredAt = state.data?.licenseExpiredAt

  if (!expiredAt) {
    return null
  }
  const date = new Intl.DateTimeFormat(i18n.language, { dateStyle: 'long' }).format(
    new Date(expiredAt),
  )
  return (
    <Alert color="yellow" title={t(($) => $.instance.licenseExpired.title)} mb="md">
      <Text size="sm" maw={720}>
        {t(($) => $.instance.licenseExpired.description, { date })}
      </Text>
    </Alert>
  )
}
