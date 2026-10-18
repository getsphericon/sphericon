import { createFileRoute } from '@tanstack/react-router'

import { SegmentEditPage } from '../../../../../pages/segments/resource.tsx'

export const Route = createFileRoute('/workspaces/$slug/segments/$segmentId/edit')({
  component: SegmentEditPage,
})
