import { createLazyFileRoute } from '@tanstack/react-router'

import { SegmentsListPage } from '../../../../pages/segments/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/segments/')({
  component: SegmentsListPage,
})
