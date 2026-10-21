import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { canManage } from '../../../hooks/useCurrentRole.ts'
import { Route as emailsRoute } from '../../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { IntegrationsSection } from '../IntegrationsSection.tsx'
import { SendingDomainsSection } from '../SendingDomainsSection.tsx'
import { SuppressionsSection } from '../SuppressionsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function EmailsPage() {
  const { t } = useTranslation()
  const { workspaceId } = emailsRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.emails)}>
      {(workspace) => (
        <Stack>
          <IntegrationsSection workspaceId={workspace.id} />
          <SendingDomainsSection workspaceId={workspace.id} canManage={canManage(workspace.role)} />
          <SuppressionsSection workspaceId={workspace.id} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
