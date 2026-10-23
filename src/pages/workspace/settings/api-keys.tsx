import { useTranslation } from 'react-i18next'

import { Route as apiKeysRoute } from '../../../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { ApiKeysSection } from '../ApiKeysSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function ApiKeysPage() {
  const { t } = useTranslation()
  const { workspaceId } = apiKeysRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.apiKeys)}>
      {(workspace) => <ApiKeysSection workspaceId={workspace.id} />}
    </SettingsFrame>
  )
}
