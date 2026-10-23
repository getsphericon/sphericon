import { useTranslation } from 'react-i18next'

import { canManage } from '../../../hooks/useCurrentRole.ts'
import { Route as webhooksRoute } from '../../../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { WebhooksSection } from '../WebhooksSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// Webhook secrets are read only by owners and admins, so a member sees the title and no section.
export function WebhooksPage() {
  const { t } = useTranslation()
  const { workspaceId } = webhooksRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.webhooks)}>
      {(workspace) =>
        canManage(workspace.role) ? <WebhooksSection workspaceId={workspace.id} /> : null
      }
    </SettingsFrame>
  )
}
