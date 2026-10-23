import { createLazyFileRoute } from '@tanstack/react-router'

import { SegmentCreatePage } from '../../../../pages/segments/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/segments/new')({
  component: SegmentCreatePage,
})
