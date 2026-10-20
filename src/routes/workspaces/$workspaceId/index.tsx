import { createFileRoute } from '@tanstack/react-router'

import { OverviewPage } from '../../../pages/workspace/overview.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/')({
  component: OverviewPage,
})
