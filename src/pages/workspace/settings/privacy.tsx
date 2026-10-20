import { Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { canManage } from '../../../hooks/useCurrentRole.ts'
import { Route as privacyRoute } from '../../../routes/workspaces/$workspaceId/settings/privacy.tsx'
import { PrivacyRequestsSection } from '../PrivacyRequestsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function PrivacyPage() {
  const { t } = useTranslation()
  const { workspaceId } = privacyRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.privacy)}>
      {(workspace) =>
        canManage(workspace.role) ? (
          <PrivacyRequestsSection workspaceId={workspace.id} />
        ) : (
          <Text c="dimmed">{t(($) => $.settings.privacy.membersOnly)}</Text>
        )
      }
    </SettingsFrame>
  )
}
