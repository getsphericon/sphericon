import { createLazyFileRoute } from '@tanstack/react-router'

import { SegmentEditPage } from '../../../../../pages/segments/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/segments/$segmentId/edit')({
  component: SegmentEditPage,
})
