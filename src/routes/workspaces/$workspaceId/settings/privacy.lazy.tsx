import { createLazyFileRoute } from '@tanstack/react-router'

import { PrivacyPage } from '../../../../pages/workspace/settings/privacy.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/privacy')({
  component: PrivacyPage,
})
