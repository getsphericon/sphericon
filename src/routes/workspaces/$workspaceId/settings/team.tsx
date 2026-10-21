import { createFileRoute } from '@tanstack/react-router'

import { TeamPage } from '../../../../pages/workspace/settings/team.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/team')({
  component: TeamPage,
})
