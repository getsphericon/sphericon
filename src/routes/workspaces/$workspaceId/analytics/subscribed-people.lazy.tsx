import { createLazyFileRoute } from '@tanstack/react-router'

import { SubscribedPeoplePage } from '../../../../pages/workspace/analytics/subscribedPeople.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/subscribed-people')({
  component: SubscribedPeoplePage,
})
