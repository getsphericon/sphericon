import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as developersRoute } from '../../../routes/workspaces/$slug/settings/developers.tsx'
import { ApiKeysSection } from '../ApiKeysSection.tsx'
import { WebhooksSection } from '../WebhooksSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function DevelopersPage() {
  const { t } = useTranslation()
  const { slug } = developersRoute.useParams()

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.developers)}>
      {(workspace) => (
        <Stack>
          <ApiKeysSection slug={workspace.slug} />
          <WebhooksSection slug={workspace.slug} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
