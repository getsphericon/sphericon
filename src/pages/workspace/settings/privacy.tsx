import { Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as privacyRoute } from '../../../routes/workspaces/$workspaceId/settings/privacy.tsx'
import { can } from '../../../utils/permissions.ts'
import { PrivacyRequestsSection } from '../PrivacyRequestsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function PrivacyPage() {
  const { t } = useTranslation()
  const { workspaceId } = privacyRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.privacy)}>
      {(workspace) =>
        can(workspace, 'contacts:erase') ? (
          <PrivacyRequestsSection workspaceId={workspace.id} />
        ) : (
          <Text c="dimmed">{t(($) => $.settings.privacy.membersOnly)}</Text>
        )
      }
    </SettingsFrame>
  )
}
