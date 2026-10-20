import { createFileRoute } from '@tanstack/react-router'

import { SegmentEditPage } from '../../../../../pages/segments/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/segments/$segmentId/edit')({
  component: SegmentEditPage,
})
