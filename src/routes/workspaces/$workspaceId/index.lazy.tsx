import { createLazyFileRoute } from '@tanstack/react-router'

import { OverviewPage } from '../../../pages/workspace/overview.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/')({
  component: OverviewPage,
})
