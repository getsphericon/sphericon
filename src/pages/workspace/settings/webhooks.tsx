import { useTranslation } from 'react-i18next'

import { Route as webhooksRoute } from '../../../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { can } from '../../../utils/permissions.ts'
import { WebhooksSection } from '../WebhooksSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// Webhook secrets are read only with webhooks:manage, so anyone without it sees the title and no
// section.
export function WebhooksPage() {
  const { t } = useTranslation()
  const { workspaceId } = webhooksRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.webhooks)}>
      {(workspace) =>
        can(workspace, 'webhooks:manage') ? <WebhooksSection workspaceId={workspace.id} /> : null
      }
    </SettingsFrame>
  )
}
