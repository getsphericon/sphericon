import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as apiKeysRoute } from '../../../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { can } from '../../../utils/permissions.ts'
import { ApiKeysSection } from '../ApiKeysSection.tsx'
import { PersonalTokensSection } from '../PersonalTokensSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// ApiKeysPage holds the Workspace's API tokens, for systems, and apart from them the Personal
// tokens, a member's own credentials for their scripts (ADR 0061).
export function ApiKeysPage() {
  const { t } = useTranslation()
  const { workspaceId } = apiKeysRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.apiKeys)}>
      {(workspace) => (
        <Stack gap="xl">
          <ApiKeysSection workspaceId={workspace.id} canManage={can(workspace, 'tokens:manage')} />
          <PersonalTokensSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
