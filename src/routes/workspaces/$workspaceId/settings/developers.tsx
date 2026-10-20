import { createFileRoute } from '@tanstack/react-router'

import { DevelopersPage } from '../../../../pages/workspace/settings/developers.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/developers')({
  component: DevelopersPage,
})
