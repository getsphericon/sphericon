import { createFileRoute } from '@tanstack/react-router'

import { SegmentCreatePage } from '../../../../pages/segments/resource.tsx'

export const Route = createFileRoute('/workspaces/$slug/segments/new')({
  component: SegmentCreatePage,
})
