import { Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { canManage } from '../../../hooks/useCurrentRole.ts'
import { Route as privacyRoute } from '../../../routes/workspaces/$slug/settings/privacy.tsx'
import { PrivacyRequestsSection } from '../PrivacyRequestsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function PrivacyPage() {
  const { t } = useTranslation()
  const { slug } = privacyRoute.useParams()

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.privacy)}>
      {(workspace) =>
        canManage(workspace.role) ? (
          <PrivacyRequestsSection slug={workspace.slug} />
        ) : (
          <Text c="dimmed">{t(($) => $.settings.privacy.membersOnly)}</Text>
        )
      }
    </SettingsFrame>
  )
}
