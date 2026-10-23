import { createLazyFileRoute } from '@tanstack/react-router'

import { ApiKeysPage } from '../../../../pages/workspace/settings/api-keys.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/api-keys')({
  component: ApiKeysPage,
})
