import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as sendingRoute } from '../../../routes/workspaces/$workspaceId/settings/sending.tsx'
import { IntegrationsSection } from '../IntegrationsSection.tsx'
import { SendingDomainsSection } from '../SendingDomainsSection.tsx'
import { SuppressionsSection } from '../SuppressionsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function SendingPage() {
  const { t } = useTranslation()
  const { workspaceId } = sendingRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.sending)}>
      {(workspace) => (
        <Stack>
          <IntegrationsSection workspaceId={workspace.id} />
          <SendingDomainsSection workspaceId={workspace.id} />
          <SuppressionsSection workspaceId={workspace.id} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
