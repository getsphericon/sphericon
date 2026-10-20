import { createFileRoute } from '@tanstack/react-router'

import { PrivacyPage } from '../../../../pages/workspace/settings/privacy.tsx'

export const Route = createFileRoute('/workspaces/$slug/settings/privacy')({
  component: PrivacyPage,
})
