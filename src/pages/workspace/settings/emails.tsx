import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as emailsRoute } from '../../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { BrandKitSection } from '../BrandKitSection.tsx'
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
          <SendingDomainsSection workspaceId={workspace.id} />
          <SuppressionsSection workspaceId={workspace.id} />
          <BrandKitSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
