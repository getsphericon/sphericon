import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as emailsRoute } from '../../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { can } from '../../../utils/permissions.ts'
import { IntegrationsSection } from '../IntegrationsSection.tsx'
import { SendCapNotice } from '../SendCapNotice.tsx'
import { SendingDomainsSection } from '../SendingDomainsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function EmailsPage() {
  const { t } = useTranslation()
  const { workspaceId } = emailsRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.emails)}>
      {(workspace) => (
        <Stack gap="xl">
          {workspace.sendCap && <SendCapNotice sendCap={workspace.sendCap} />}
          <IntegrationsSection workspace={workspace} />
          <SendingDomainsSection
            workspaceId={workspace.id}
            canEdit={can(workspace, 'sending_domains:manage')}
          />
        </Stack>
      )}
    </SettingsFrame>
  )
}
