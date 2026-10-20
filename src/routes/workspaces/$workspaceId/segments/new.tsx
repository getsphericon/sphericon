import { createFileRoute } from '@tanstack/react-router'

import { SegmentCreatePage } from '../../../../pages/segments/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/segments/new')({
  component: SegmentCreatePage,
})
