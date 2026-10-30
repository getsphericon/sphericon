import { Alert, Box, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteInstanceGetReleaseOptions } from '../generated/site/@tanstack/react-query.gen.ts'

// ReleaseAlert says that a newer release exists (the Release check, ADR 0032): the one alert the
// app's banner and the Staff console's Instance page both show.
export function ReleaseAlert({ version }: { version: string }) {
  const { t } = useTranslation()

  return (
    <Alert color="teal" title={t(($) => $.instance.update.title)}>
      <Text size="sm" maw={720}>
        {t(($) => $.instance.update.description, { version })}
      </Text>
    </Alert>
  )
}

// UpdateBanner tells a Workspace Owner that a newer release exists. The server carries a release
// only when it is newer than the running version, and refuses it to a User who owns no live
// Workspace, so nothing renders for them.
export function UpdateBanner() {
  const release = useQuery({ ...siteInstanceGetReleaseOptions(), retry: false })
  const latest = release.data?.latestRelease

  if (!latest) {
    return null
  }
  return (
    <Box mb="md">
      <ReleaseAlert version={latest} />
    </Box>
  )
}
