import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as developersRoute } from '../../../routes/workspaces/$workspaceId/settings/developers.tsx'
import { ApiKeysSection } from '../ApiKeysSection.tsx'
import { WebhooksSection } from '../WebhooksSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function DevelopersPage() {
  const { t } = useTranslation()
  const { workspaceId } = developersRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.developers)}>
      {(workspace) => (
        <Stack>
          <ApiKeysSection workspaceId={workspace.id} />
          <WebhooksSection workspaceId={workspace.id} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
