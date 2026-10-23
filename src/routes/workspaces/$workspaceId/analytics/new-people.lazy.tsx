import { createLazyFileRoute } from '@tanstack/react-router'

import { NewPeoplePage } from '../../../../pages/workspace/analytics/newPeople.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/new-people')({
  component: NewPeoplePage,
})
