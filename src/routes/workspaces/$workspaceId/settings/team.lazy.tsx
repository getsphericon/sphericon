import { createLazyFileRoute } from '@tanstack/react-router'

import { TeamPage } from '../../../../pages/workspace/settings/team.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/team')({
  component: TeamPage,
})
