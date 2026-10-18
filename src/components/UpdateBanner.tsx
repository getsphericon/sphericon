import { Alert, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { useInstanceState } from '../hooks/useInstanceState.ts'

// UpdateBanner tells an Owner of the oldest Workspace that a newer release exists (the
// Release check, ADR 0032). The state carries a release only when it is newer than the
// running version, and is refused to everyone else, so nothing renders for them.
export function UpdateBanner() {
  const { t } = useTranslation()
  const state = useInstanceState()
  const latest = state.data?.latestRelease

  if (!latest) {
    return null
  }
  return (
    <Alert color="teal" title={t(($) => $.instance.update.title)} mb="md">
      <Text size="sm" maw={720}>
        {t(($) => $.instance.update.description, { version: latest })}
      </Text>
    </Alert>
  )
}
