import { createFileRoute } from '@tanstack/react-router'

import { SegmentsListPage } from '../../../../pages/segments/list.tsx'

export const Route = createFileRoute('/workspaces/$slug/segments/')({
  component: SegmentsListPage,
})
